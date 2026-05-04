// Hierarchical force layout — two coupled simulations:
//
//   1. centroidSim — one node per type, weighted edges between types
//      computed from cross-type entity references. Small graph, settles
//      fast under d3-force charge + link.
//
//   2. memberSim — entity nodes only. Each member is anchored to its
//      type's centroid via a half-spring; member-charge spreads cluster
//      members apart locally. Members never feel cross-cluster forces
//      directly — those resolve at the centroid layer.
//
// Splitting the layers means cluster topology converges in ~30 ticks
// (small N) before members start chasing centroids around. The single-
// sim version had members and centroids fighting each other for
// hundreds of ticks while everything slid into place.

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
    weight?: number;
}

const NODE_RADIUS = 7;

const DEFAULTS = {
    linkDistance: 200,
    linkStrength: 0.1,
    // rest distance for the member→centroid half-spring (cluster radius)
    memberDistance: 50,
    // member→centroid half-spring strength (how tightly each cluster
    // holds its members)
    memberAttract: 1.0,
    // rest distance for centroid↔centroid weighted links (how far apart
    // cross-referenced clusters settle)
    clusterDistance: 750,
    // centroid↔centroid weighted link strength (how strongly
    // cross-referenced clusters pull together)
    clusterStrength: 0.5,
    chargeStrength: -750,
    centroidGravity: 0.3,
    showCentroids: false,
};

// Centroid charge is a fixed strong repulsion — clusters need plenty of
// space between them; tuning this from the panel adds little user value.
const CENTROID_CHARGE = -3000;

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
        key: "memberDistance",
        label: "member dist",
        type: "range",
        min: 20,
        max: 200,
        step: 5,
        default: DEFAULTS.memberDistance,
    },
    {
        key: "memberAttract",
        label: "member attract",
        type: "range",
        min: 0,
        max: 2,
        step: 0.05,
        decimals: 2,
        default: DEFAULTS.memberAttract,
    },
    {
        key: "clusterDistance",
        label: "cluster dist",
        type: "range",
        min: 100,
        max: 1000,
        step: 25,
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
        "Charge + entity links with ghost-centroid clustering by entity type.",
    options: OPTIONS,
    create() {
        return createForceHierarchical();
    },
};

function createForceHierarchical(): LayoutEngine {
    let memberNodes: SimNode[] = [];
    let memberLinks: SimLink[] = [];
    let memberById = new Map<string, SimNode>();

    let centroids: SimNode[] = [];
    let centroidByType = new Map<string, SimNode>();
    // Cross-cluster centroid links — weight is normalised cross-cluster
    // reference count. Drives the centroidSim's link force.
    let interCentroidLinks: SimLink[] = [];
    // member→centroid attract pairs — read by memberAttractForce on
    // memberSim. Centroid positions come from centroidSim.
    let memberAttractLinks: SimLink[] = [];

    let linkDistance = DEFAULTS.linkDistance;
    let linkStrength = DEFAULTS.linkStrength;
    let memberDistance = DEFAULTS.memberDistance;
    let memberAttract = DEFAULTS.memberAttract;
    let clusterDistance = DEFAULTS.clusterDistance;
    let clusterStrength = DEFAULTS.clusterStrength;
    let chargeStrength = DEFAULTS.chargeStrength;
    let centroidGravity = DEFAULTS.centroidGravity;
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

    function memberAttractStrengthFor(l: SimLink): number {
        const target = l.target as SimNode;
        const t = target.type ?? "(untyped)";
        const count = memberCountByType.get(t) ?? 1;
        return memberAttract / Math.sqrt(Math.max(1, count));
    }

    function memberAttractForce(alpha: number): void {
        if (memberAttractLinks.length === 0) return;
        const denom = Math.max(memberDistance, 1);
        // Cap overshoot/denom so the quadratic doesn't fling nodes when
        // a member is briefly far from its centroid. Past 4× rest the
        // force plateaus, preventing slingshot oscillation.
        const RATIO_CAP = 4;
        for (const l of memberAttractLinks) {
            const m = l.source as SimNode;
            const c = l.target as SimNode;
            const dx = (c.x ?? 0) - (m.x ?? 0);
            const dy = (c.y ?? 0) - (m.y ?? 0);
            const distSq = dx * dx + dy * dy;
            if (distSq <= memberDistance * memberDistance) continue;
            const dist = Math.sqrt(distSq);
            const overshoot = dist - memberDistance;
            const ratio = Math.min(overshoot / denom, RATIO_CAP);
            const adjust =
                ((ratio * ratio * denom) / dist) *
                alpha *
                memberAttractStrengthFor(l);
            // Centroid is owned by centroidSim — only move members here.
            // Centroid would be over-written by centroidSim's next pass
            // anyway, but skipping the write is cleaner.
            if (m.vx !== undefined) m.vx += dx * adjust;
            if (m.vy !== undefined) m.vy += dy * adjust;
        }
    }

    function centroidGravityFor(d: SimNode): number {
        if (centroidGravity === 0) return 0;
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

    function centroidLinkStrengthFor(l: SimLink): number {
        const w = (l as { weight?: number }).weight ?? 1;
        return clusterStrength * w;
    }

    function centroidId(typeName: string): string {
        return `__centroid:${typeName}`;
    }

    function rebuildCentroids(): void {
        memberCountByType.clear();
        for (const n of memberNodes) {
            const t = n.type ?? "(untyped)";
            memberCountByType.set(t, (memberCountByType.get(t) ?? 0) + 1);
        }
        maxMemberCount = 1;
        for (const v of memberCountByType.values()) {
            if (v > maxMemberCount) maxMemberCount = v;
        }
        const presentTypes = new Set<string>();
        for (const n of memberNodes) presentTypes.add(n.type ?? "(untyped)");

        // Preserve existing centroids' positions when their type still has
        // members; new types spawn near origin with a small jitter.
        const existing = new Map<string, SimNode>();
        for (const c of centroids) existing.set(c.type ?? "(untyped)", c);
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
        centroidByType.clear();
        for (const c of centroids) centroidByType.set(c.type ?? "(untyped)", c);

        // member→centroid attract pairs
        memberAttractLinks = [];
        for (const m of memberNodes) {
            const c = centroidByType.get(m.type ?? "(untyped)");
            if (!c) continue;
            memberAttractLinks.push({ source: m, target: c });
        }

        // inter-centroid links — counts cross-cluster member references,
        // normalised so the busiest pair has weight 1.
        const crossCount = new Map<string, number>();
        for (const l of memberLinks) {
            const sNode = l.source as SimNode;
            const tNode = l.target as SimNode;
            const sType = sNode.type ?? "(untyped)";
            const tType = tNode.type ?? "(untyped)";
            if (sType === tType) continue;
            const key =
                sType < tType ? `${sType}\0${tType}` : `${tType}\0${sType}`;
            crossCount.set(key, (crossCount.get(key) ?? 0) + (l.weight ?? 1));
        }
        let maxCross = 1;
        for (const v of crossCount.values()) {
            if (v > maxCross) maxCross = v;
        }
        interCentroidLinks = [];
        for (const [key, count] of crossCount) {
            const [a, b] = key.split("\0");
            const ca = centroidByType.get(a);
            const cb = centroidByType.get(b);
            if (!ca || !cb) continue;
            interCentroidLinks.push({
                source: ca,
                target: cb,
                weight: count / maxCross,
            });
        }
    }

    function applySimulationContents(): void {
        centroidSim.nodes(centroids);
        const cLink =
            centroidSim.force<ReturnType<typeof forceLink<SimNode, SimLink>>>(
                "link",
            );
        if (cLink) cLink.links(interCentroidLinks);

        memberSim.nodes(memberNodes);
        const mLink =
            memberSim.force<ReturnType<typeof forceLink<SimNode, SimLink>>>(
                "link",
            );
        if (mLink) mLink.links(memberLinks);
    }

    // Centroid sim — small graph (one node per type), weighted links
    // from cross-cluster references. Settles fast (~30 ticks) so the
    // member sim has stable anchors to follow.
    const centroidSim: Simulation<SimNode, SimLink> = forceSimulation<
        SimNode,
        SimLink
    >([])
        .force(
            "link",
            forceLink<SimNode, SimLink>([])
                .id((d) => d.id)
                .distance(() => clusterDistance)
                .strength(centroidLinkStrengthFor),
        )
        .force("charge", forceManyBody<SimNode>().strength(CENTROID_CHARGE))
        .force("center", forceCenter(0, 0))
        .force("gravityX", forceX<SimNode>().x(0).strength(centroidGravityFor))
        .force("gravityY", forceY<SimNode>().y(0).strength(centroidGravityFor))
        .alphaDecay(0.05)
        .on("tick", emitTick);

    // Member sim — entity nodes only. Each member feels its own
    // cluster's charge plus an anchor to its centroid; the centroid
    // is read from centroidSim's state each tick.
    const memberSim: Simulation<SimNode, SimLink> = forceSimulation<
        SimNode,
        SimLink
    >([])
        .force(
            "link",
            forceLink<SimNode, SimLink>([])
                .id((d) => d.id)
                .distance(linkDistance)
                .strength(linkStrength),
        )
        .force("charge", forceManyBody<SimNode>().strength(chargeStrength))
        .force("collide", forceCollide<SimNode>().radius(NODE_RADIUS + 14))
        .force("memberAttract", memberAttractForce)
        .alphaDecay(0.015)
        .on("tick", emitTick);

    function emitTick(): void {
        const map = new Map<string, LayoutPosition>();
        for (const n of memberNodes) {
            map.set(n.id, { x: n.x ?? 0, y: n.y ?? 0 });
        }
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
            focusHint && memberById.has(focusHint)
                ? (memberById.get(focusHint)!.x ?? 0)
                : 0;
        const seedY =
            focusHint && memberById.has(focusHint)
                ? (memberById.get(focusHint)!.y ?? 0)
                : 0;
        const next: SimNode[] = [];
        for (const n of nextNodes) {
            const existing = memberById.get(n.id);
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
        memberNodes = next;
        memberById = new Map(memberNodes.map((n) => [n.id, n]));

        memberLinks = [];
        for (const l of nextLinks) {
            const s = memberById.get(l.source);
            const t = memberById.get(l.target);
            if (!s || !t) continue;
            memberLinks.push({ source: s, target: t, weight: l.weight });
        }

        rebuildCentroids();
        applySimulationContents();
        emitCentroids();
        emitTick();
        // Both sims kicked — centroidSim's higher alphaDecay means it
        // settles in ~30 ticks regardless of membership graph size.
        centroidSim.alpha(Math.max(centroidSim.alpha(), 0.6)).restart();
        memberSim.alpha(Math.max(memberSim.alpha(), 0.4)).restart();
    }

    function setOption(key: string, value: number | boolean): void {
        switch (key) {
            case "linkDistance": {
                if (typeof value !== "number" || value === linkDistance) return;
                linkDistance = value;
                memberSim
                    .force<
                        ReturnType<typeof forceLink<SimNode, SimLink>>
                    >("link")
                    ?.distance(value);
                break;
            }
            case "linkStrength": {
                if (typeof value !== "number" || value === linkStrength) return;
                linkStrength = value;
                memberSim
                    .force<
                        ReturnType<typeof forceLink<SimNode, SimLink>>
                    >("link")
                    ?.strength(value);
                break;
            }
            case "memberAttract": {
                if (typeof value !== "number" || value === memberAttract)
                    return;
                memberAttract = value;
                // memberAttractForce reads memberAttract via closure.
                break;
            }
            case "clusterStrength": {
                if (typeof value !== "number" || value === clusterStrength)
                    return;
                clusterStrength = value;
                // Re-evaluate centroid link strengths — d3 caches
                // per-link strength on init.
                centroidSim
                    .force<
                        ReturnType<typeof forceLink<SimNode, SimLink>>
                    >("link")
                    ?.strength(centroidLinkStrengthFor);
                break;
            }
            case "memberDistance": {
                if (typeof value !== "number" || value === memberDistance)
                    return;
                // memberAttractForce reads memberDistance via closure.
                memberDistance = value;
                break;
            }
            case "clusterDistance": {
                if (typeof value !== "number" || value === clusterDistance)
                    return;
                clusterDistance = value;
                // centroidSim's forceLink.distance() reads clusterDistance
                // via closure each link init — re-call .distance() to
                // refresh cached per-link rest values.
                centroidSim
                    .force<
                        ReturnType<typeof forceLink<SimNode, SimLink>>
                    >("link")
                    ?.distance(() => clusterDistance);
                break;
            }
            case "chargeStrength": {
                if (typeof value !== "number" || value === chargeStrength)
                    return;
                chargeStrength = value;
                memberSim
                    .force<ReturnType<typeof forceManyBody<SimNode>>>("charge")
                    ?.strength(value);
                break;
            }
            case "centroidGravity": {
                if (typeof value !== "number" || value === centroidGravity)
                    return;
                centroidGravity = value;
                centroidSim
                    .force<ReturnType<typeof forceX<SimNode>>>("gravityX")
                    ?.strength(centroidGravityFor);
                centroidSim
                    .force<ReturnType<typeof forceY<SimNode>>>("gravityY")
                    ?.strength(centroidGravityFor);
                break;
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
        centroidSim.alpha(Math.max(centroidSim.alpha(), 0.4)).restart();
        memberSim.alpha(Math.max(memberSim.alpha(), 0.4)).restart();
    }

    function setHandlers(h: LayoutHandlers): void {
        handlers = h;
    }

    function pinNode(id: string, x: number, y: number): void {
        const n = memberById.get(id);
        if (!n) return;
        const wasUnpinned = n.fx === null || n.fx === undefined;
        if (wasUnpinned) {
            pinnedCount += 1;
            if (pinnedCount === 1) memberSim.alphaTarget(0.3).restart();
        }
        n.fx = x;
        n.fy = y;
    }

    function releaseNode(id: string): void {
        const n = memberById.get(id);
        if (!n) return;
        const wasPinned = n.fx !== null && n.fx !== undefined;
        n.fx = null;
        n.fy = null;
        if (wasPinned) {
            pinnedCount = Math.max(0, pinnedCount - 1);
            if (pinnedCount === 0) memberSim.alphaTarget(0);
        }
    }

    function bumpEnergy(level: number = 0.4): void {
        memberSim.alpha(Math.max(memberSim.alpha(), level)).restart();
    }

    function start(): void {
        centroidSim.restart();
        memberSim.restart();
    }

    function stop(): void {
        centroidSim.stop();
        memberSim.stop();
    }

    function dispose(): void {
        centroidSim.stop();
        memberSim.stop();
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
