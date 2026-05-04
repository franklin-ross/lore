// Hierarchical force layout: d3-force charge + entity links plus a
// per-type ghost centroid that members are attracted to via a
// half-spring (only pulls when past clusterDistance, no force inside).
// Cluster centroids inherit the shared charge force so they push each
// other apart, producing the hub-and-spoke topology this codebase needs.

import {
    forceCenter,
    forceCollide,
    forceLink,
    forceManyBody,
    forceSimulation,
    forceX,
    forceY,
    type Simulation,
    type SimulationLinkDatum,
    type SimulationNodeDatum,
} from "d3-force";

import type { RenderCentroid } from "../graph-renderer.ts";
import type {
    LayoutEngine,
    LayoutFactory,
    LayoutHandlers,
    LayoutLink,
    LayoutNode,
    LayoutPosition,
    OptionSpec,
} from "./types.ts";

interface SimNode extends SimulationNodeDatum {
    id: string;
    type?: string;
    isCentroid?: boolean;
}

interface SimLink extends SimulationLinkDatum<SimNode> {
    isCentroidLink?: boolean;
}

const NODE_RADIUS = 7;

const DEFAULTS = {
    linkDistance: 200,
    linkStrength: 0.1,
    clusterDistance: 50,
    clusterStrength: 1.0,
    chargeStrength: -250,
    centroidGravity: 0.2,
    clustering: true,
    showCentroids: false,
};

const OPTIONS: OptionSpec[] = [
    {
        key: "linkDistance",
        label: "link dist",
        type: "range",
        min: 20,
        max: 300,
        step: 5,
        default: DEFAULTS.linkDistance,
    },
    {
        key: "linkStrength",
        label: "link str",
        type: "range",
        min: 0,
        max: 1,
        step: 0.05,
        decimals: 2,
        default: DEFAULTS.linkStrength,
    },
    {
        key: "clusterDistance",
        label: "cluster dist",
        type: "range",
        min: 20,
        max: 200,
        step: 5,
        default: DEFAULTS.clusterDistance,
    },
    {
        key: "clusterStrength",
        label: "cluster str",
        type: "range",
        min: 0,
        max: 2,
        step: 0.05,
        decimals: 2,
        default: DEFAULTS.clusterStrength,
    },
    {
        key: "chargeStrength",
        label: "charge",
        type: "range",
        min: -3000,
        max: 0,
        step: 25,
        default: DEFAULTS.chargeStrength,
    },
    {
        key: "centroidGravity",
        label: "cluster gravity",
        type: "range",
        min: 0,
        max: 1,
        step: 0.02,
        decimals: 2,
        default: DEFAULTS.centroidGravity,
    },
    {
        key: "clustering",
        label: "clustering",
        type: "toggle",
        default: DEFAULTS.clustering,
    },
    {
        key: "showCentroids",
        label: "show centroids",
        type: "toggle",
        default: DEFAULTS.showCentroids,
    },
];

export const forceHierarchicalFactory: LayoutFactory = {
    id: "force-hierarchical",
    label: "Force (hierarchical)",
    description:
        "Charge + entity links with optional ghost-centroid clustering by entity type.",
    options: OPTIONS,
    create() {
        return createForceHierarchical();
    },
};

function createForceHierarchical(): LayoutEngine {
    let nodes: SimNode[] = [];
    let links: SimLink[] = [];
    let centroids: SimNode[] = [];
    let centroidLinks: SimLink[] = [];
    let nodeById = new Map<string, SimNode>();

    let linkDistance = DEFAULTS.linkDistance;
    let linkStrength = DEFAULTS.linkStrength;
    let clusterDistance = DEFAULTS.clusterDistance;
    let clusterStrength = DEFAULTS.clusterStrength;
    let chargeStrength = DEFAULTS.chargeStrength;
    let centroidGravity = DEFAULTS.centroidGravity;
    let clustering = DEFAULTS.clustering;
    // Debug ghost-centroid markers (dashed rings + type label). Sim runs
    // them either way; toggle just gates whether the engine surfaces
    // them through onCentroidsChanged for the renderer to paint.
    let showCentroids = DEFAULTS.showCentroids;

    let maxMemberCount = 1;
    const memberCountByType = new Map<string, number>();
    let pinnedCount = 0;

    let handlers: LayoutHandlers = {
        onTick: () => {},
        onCentroidsChanged: () => {},
    };

    function centroidLinkStrengthFor(l: SimLink): number {
        const target = l.target as SimNode;
        const t = target.type ?? "(untyped)";
        const count = memberCountByType.get(t) ?? 1;
        return clusterStrength / Math.sqrt(Math.max(1, count));
    }

    function memberAttractForce(alpha: number): void {
        if (centroidLinks.length === 0) return;
        const memberShare = 0.9;
        const centroidShare = 0.1;
        const denom = Math.max(clusterDistance, 1);
        // Cap overshoot/denom so the quadratic doesn't fling nodes when
        // a member is briefly far from its centroid. Past 4× rest the
        // force plateaus, preventing slingshot oscillation that
        // propagates NaN through the charge force and kills the layout.
        const RATIO_CAP = 4;
        for (const l of centroidLinks) {
            const m = l.source as SimNode;
            const c = l.target as SimNode;
            const dx = (c.x ?? 0) - (m.x ?? 0);
            const dy = (c.y ?? 0) - (m.y ?? 0);
            const distSq = dx * dx + dy * dy;
            if (distSq <= clusterDistance * clusterDistance) continue;
            const dist = Math.sqrt(distSq);
            const overshoot = dist - clusterDistance;
            const ratio = Math.min(overshoot / denom, RATIO_CAP);
            const adjust =
                ((ratio * ratio * denom) / dist) *
                alpha *
                centroidLinkStrengthFor(l);
            if (m.vx !== undefined) m.vx += dx * adjust * memberShare;
            if (m.vy !== undefined) m.vy += dy * adjust * memberShare;
            if (c.vx !== undefined) c.vx -= dx * adjust * centroidShare;
            if (c.vy !== undefined) c.vy -= dy * adjust * centroidShare;
        }
    }

    function centroidGravityFor(d: SimNode): number {
        if (!d.isCentroid || centroidGravity === 0) return 0;
        const count = memberCountByType.get(d.type ?? "(untyped)") ?? 0;
        if (count === 0 || maxMemberCount === 0) return 0;
        // log-normalise so a 1-member cluster keeps most of the gravity
        // a 30-member one gets (~3× rather than ~5.5× under sqrt). With
        // linear or sqrt scaling the smallest types still drift far to
        // the rim under charge repulsion; log flattens further.
        return (
            (centroidGravity * Math.log(1 + count)) /
            Math.log(1 + maxMemberCount)
        );
    }

    function centroidId(typeName: string): string {
        return `__centroid:${typeName}`;
    }

    function rebuildCentroids(): void {
        memberCountByType.clear();
        for (const n of nodes) {
            const t = n.type ?? "(untyped)";
            memberCountByType.set(t, (memberCountByType.get(t) ?? 0) + 1);
        }
        maxMemberCount = 1;
        for (const v of memberCountByType.values()) {
            if (v > maxMemberCount) maxMemberCount = v;
        }
        if (!clustering) {
            centroids = [];
            centroidLinks = [];
            return;
        }
        const presentTypes = new Set<string>();
        for (const n of nodes) presentTypes.add(n.type ?? "(untyped)");
        const existing = new Map<string, SimNode>();
        for (const c of centroids) {
            existing.set(c.type ?? "(untyped)", c);
        }
        const next: SimNode[] = [];
        for (const t of presentTypes) {
            const prior = existing.get(t);
            if (prior) {
                next.push(prior);
            } else {
                next.push({
                    id: centroidId(t),
                    type: t,
                    isCentroid: true,
                    x: (Math.random() - 0.5) * 60,
                    y: (Math.random() - 0.5) * 60,
                });
            }
        }
        centroids = next;
        centroidLinks = [];
        const byType = new Map<string, SimNode>();
        for (const c of centroids) byType.set(c.type ?? "(untyped)", c);
        for (const n of nodes) {
            const c = byType.get(n.type ?? "(untyped)");
            if (!c) continue;
            centroidLinks.push({
                source: n,
                target: c,
                isCentroidLink: true,
            });
        }
    }

    function applySimulationContents(): void {
        sim.nodes([...nodes, ...centroids]);
        const entityLinks =
            sim.force<ReturnType<typeof forceLink<SimNode, SimLink>>>("link");
        if (entityLinks) entityLinks.links(links);
    }

    const sim: Simulation<SimNode, SimLink> = forceSimulation<SimNode, SimLink>(
        [],
    )
        .force(
            "link",
            forceLink<SimNode, SimLink>([])
                .id((d) => d.id)
                .distance(linkDistance)
                .strength(linkStrength),
        )
        .force("charge", forceManyBody<SimNode>().strength(chargeStrength))
        .force("collide", forceCollide<SimNode>().radius(NODE_RADIUS + 14))
        .force("center", forceCenter(0, 0))
        .force("memberAttract", memberAttractForce)
        .force(
            "centroidGravityX",
            forceX<SimNode>().x(0).strength(centroidGravityFor),
        )
        .force(
            "centroidGravityY",
            forceY<SimNode>().y(0).strength(centroidGravityFor),
        )
        // Slower alpha decay (default ~0.023) so the layout has time to
        // resolve large rearrangements without the user having to shake
        // a node to push energy back in.
        .alphaDecay(0.015)
        .on("tick", emitTick);

    function emitTick(): void {
        const map = new Map<string, LayoutPosition>();
        for (const n of nodes) map.set(n.id, { x: n.x ?? 0, y: n.y ?? 0 });
        for (const c of centroids) {
            map.set(c.id, { x: c.x ?? 0, y: c.y ?? 0 });
        }
        handlers.onTick(map);
    }

    function emitCentroids(): void {
        const list: RenderCentroid[] = showCentroids
            ? centroids.map((c) => ({
                  id: c.id,
                  type: c.type ?? "(untyped)",
              }))
            : [];
        handlers.onCentroidsChanged(list);
    }

    function setData(
        nextNodes: LayoutNode[],
        nextLinks: LayoutLink[],
        focusHint?: string,
    ): void {
        const seedX =
            focusHint && nodeById.has(focusHint)
                ? (nodeById.get(focusHint)!.x ?? 0)
                : 0;
        const seedY =
            focusHint && nodeById.has(focusHint)
                ? (nodeById.get(focusHint)!.y ?? 0)
                : 0;
        const next: SimNode[] = [];
        for (const n of nextNodes) {
            const existing = nodeById.get(n.id);
            if (existing) {
                existing.type = n.type;
                next.push(existing);
            } else {
                next.push({
                    id: n.id,
                    type: n.type,
                    x: seedX + (Math.random() - 0.5) * 40,
                    y: seedY + (Math.random() - 0.5) * 40,
                });
            }
        }
        nodes = next;
        nodeById = new Map(nodes.map((n) => [n.id, n]));

        links = [];
        for (const l of nextLinks) {
            const s = nodeById.get(l.source);
            const t = nodeById.get(l.target);
            if (!s || !t) continue;
            links.push({ source: s, target: t });
        }

        rebuildCentroids();
        applySimulationContents();
        emitCentroids();
        emitTick();
        sim.alpha(Math.max(sim.alpha(), 0.4)).restart();
    }

    function setOption(key: string, value: number | boolean): void {
        switch (key) {
            case "linkDistance": {
                if (typeof value !== "number" || value === linkDistance) return;
                linkDistance = value;
                sim.force<ReturnType<typeof forceLink<SimNode, SimLink>>>(
                    "link",
                )?.distance(value);
                break;
            }
            case "linkStrength": {
                if (typeof value !== "number" || value === linkStrength) return;
                linkStrength = value;
                sim.force<ReturnType<typeof forceLink<SimNode, SimLink>>>(
                    "link",
                )?.strength(value);
                break;
            }
            case "clusterStrength": {
                if (typeof value !== "number" || value === clusterStrength)
                    return;
                clusterStrength = value;
                break;
            }
            case "clusterDistance": {
                if (typeof value !== "number" || value === clusterDistance)
                    return;
                // memberAttractForce reads clusterDistance via closure; no
                // force rebind needed.
                clusterDistance = value;
                break;
            }
            case "chargeStrength": {
                if (typeof value !== "number" || value === chargeStrength)
                    return;
                chargeStrength = value;
                sim.force<ReturnType<typeof forceManyBody<SimNode>>>(
                    "charge",
                )?.strength(value);
                break;
            }
            case "centroidGravity": {
                if (typeof value !== "number" || value === centroidGravity)
                    return;
                centroidGravity = value;
                sim.force<ReturnType<typeof forceX<SimNode>>>(
                    "centroidGravityX",
                )?.strength(centroidGravityFor);
                sim.force<ReturnType<typeof forceY<SimNode>>>(
                    "centroidGravityY",
                )?.strength(centroidGravityFor);
                break;
            }
            case "clustering": {
                if (typeof value !== "boolean" || value === clustering) return;
                clustering = value;
                rebuildCentroids();
                applySimulationContents();
                emitCentroids();
                sim.alpha(Math.max(sim.alpha(), 0.2)).restart();
                return;
            }
            case "showCentroids": {
                if (typeof value !== "boolean" || value === showCentroids)
                    return;
                showCentroids = value;
                emitCentroids();
                return;
            }
            default:
                return;
        }
        sim.alpha(Math.max(sim.alpha(), 0.4)).restart();
    }

    function setHandlers(h: LayoutHandlers): void {
        handlers = h;
    }

    function pinNode(id: string, x: number, y: number): void {
        const n = nodeById.get(id);
        if (!n) return;
        const wasUnpinned = n.fx === null || n.fx === undefined;
        if (wasUnpinned) {
            pinnedCount += 1;
            if (pinnedCount === 1) sim.alphaTarget(0.3).restart();
        }
        n.fx = x;
        n.fy = y;
    }

    function releaseNode(id: string): void {
        const n = nodeById.get(id);
        if (!n) return;
        const wasPinned = n.fx !== null && n.fx !== undefined;
        n.fx = null;
        n.fy = null;
        if (wasPinned) {
            pinnedCount = Math.max(0, pinnedCount - 1);
            if (pinnedCount === 0) sim.alphaTarget(0);
        }
    }

    function bumpEnergy(level: number = 0.4): void {
        sim.alpha(Math.max(sim.alpha(), level)).restart();
    }

    function start(): void {
        sim.restart();
    }

    function stop(): void {
        sim.stop();
    }

    function dispose(): void {
        sim.stop();
    }

    return {
        setData,
        setOption,
        setHandlers,
        pinNode,
        releaseNode,
        bumpEnergy,
        start,
        stop,
        dispose,
    };
}
