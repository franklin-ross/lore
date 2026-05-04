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

import {
    mountRenderer,
    type GraphRenderer,
    type RenderCentroid,
    type RenderLink,
    type RenderNode,
} from "./graph-renderer.ts";

export interface GraphNode {
    label: string;
    name: string;
    type?: string;
    colourIndex: number;
}

export interface GraphDefEdge {
    from: string;
    to: string;
    count: number;
}

export interface GraphPayload {
    nodes?: GraphNode[];
    defEdges?: GraphDefEdge[];
}

export interface GraphHandlers {
    // onFocus fires when the user picks a node (label set) or clicks empty
    // background (label null) to clear the selection. Webview state +
    // extension bus emission decided by main.ts based on which case.
    onFocus(label: string | null): void;
    onOpenEntity(label: string): void;
}

interface SimNode extends SimulationNodeDatum {
    id: string;
    name: string;
    type?: string;
    colourIndex: number;
    // Centroid ghost nodes participate in the simulation so members can
    // be link-pulled toward them and other centroids repel them via the
    // shared charge force, but they're skipped by every paint and
    // user-interaction path.
    isCentroid?: boolean;
}

interface SimLink extends SimulationLinkDatum<SimNode> {
    count: number;
    // Member→centroid links exist only when hierarchical clustering is
    // on. Painted edges filter to !isCentroidLink so the user only sees
    // real entity references.
    isCentroidLink?: boolean;
}

export interface GraphView {
    update(payload: GraphPayload, focus: string | null | undefined): void;
    setFocus(label: string | null | undefined): void;
    setHopLimit(limit: number | null): void;
    setArrowSize(size: number): void;
    setTypeFilter(types: string[] | null): void;
    setClustering(on: boolean): void;
    setLinkDistance(d: number): void;
    setLinkStrength(s: number): void;
    setClusterStrength(s: number): void;
    setClusterDistance(d: number): void;
    setChargeStrength(s: number): void;
    setCentroidGravity(s: number): void;
    dispose(): void;
}

const NODE_RADIUS = 7;

/**
 * mountGraph wires a d3-force simulation to the renderer. The
 * simulation owns physics + centroid lifecycle; the renderer owns
 * paint, hover, drag input, pan/zoom. Each tick pushes a positions
 * map to the renderer; drag input flows back via handlers and pins
 * SimNode.fx/fy.
 */
export function mountGraph(
    host: HTMLElement,
    handlers: GraphHandlers,
    getPalette: () => string[],
): GraphView {
    let nodes: SimNode[] = [];
    let links: SimLink[] = [];
    // Ghost centroid nodes — one per type, present in the simulation only
    // while clustering is on. Members get link-pulled toward their type's
    // centroid; centroids inherit the shared charge force and so push
    // each other apart naturally.
    let centroids: SimNode[] = [];
    let centroidLinks: SimLink[] = [];
    let nodeById = new Map<string, SimNode>();
    let focus: string | null = null;
    let clustering = false;
    // Layout knobs — exposed via setters so the toolbar can tune them
    // live. Defaults match the values previously baked into the force
    // setup; user can drag sliders from there.
    let linkDistance = 90;
    let linkStrength = 0.6;
    let clusterDistance = 60;
    let clusterStrength = 0.3;
    let chargeStrength = -400;
    // Size-weighted centroid gravity: bigger clusters sink toward (0,0),
    // smaller clusters drift to the rim under centroid-vs-centroid
    // charge repulsion. Only applies to isCentroid ghost nodes.
    let centroidGravity = 0;
    let maxMemberCount = 1;
    const memberCountByType = new Map<string, number>();

    const renderer: GraphRenderer = mountRenderer(
        host,
        {
            onNodeClick(id) {
                handlers.onFocus(id);
            },
            onNodeOpen(id) {
                handlers.onOpenEntity(id);
            },
            onBackgroundClick() {
                handlers.onFocus(null);
            },
            onNodeDragStart(id) {
                const n = nodeById.get(id);
                if (!n) return;
                simulation.alphaTarget(0.3).restart();
                n.fx = n.x;
                n.fy = n.y;
            },
            onNodeDragMove(id, x, y) {
                const n = nodeById.get(id);
                if (!n) return;
                n.fx = x;
                n.fy = y;
            },
            onNodeDragEnd(id) {
                const n = nodeById.get(id);
                if (!n) return;
                simulation.alphaTarget(0);
                n.fx = null;
                n.fy = null;
            },
        },
        getPalette,
    );

    const simulation: Simulation<SimNode, SimLink> = forceSimulation<
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
        .force("center", forceCenter(0, 0))
        // Custom half-spring attraction from member to centroid: pulls
        // when the member is past clusterDistance, no force when inside
        // (so a small cluster naturally fitting inside its radius
        // doesn't get pushed apart). Replaces forceLink so members
        // never feel a "too close, push out" repulsion from their own
        // cluster — only charge repels members from each other.
        .force("memberAttract", memberAttractForce)
        // Size-weighted centroid gravity: only fires for isCentroid
        // nodes, scales with the cluster's member count.
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
        .on("tick", emitPositions);

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
                (ratio * ratio * denom / dist)
                * alpha
                * centroidLinkStrengthFor(l);
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
            centroidGravity * Math.log(1 + count)
                / Math.log(1 + maxMemberCount)
        );
    }

    function centroidId(typeName: string): string {
        return `__centroid:${typeName}`;
    }

    // rebuildCentroids regenerates the ghost-centroid set from the
    // current node list. Existing centroids are kept (positions
    // preserved) when their type still has members; new types spawn
    // near origin with a small random jitter and physics decides their
    // long-term position.
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
                    name: t,
                    type: t,
                    colourIndex: -1,
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
                count: 1,
                isCentroidLink: true,
            });
        }
    }

    function diffNodes(next: GraphNode[]): {
        kept: SimNode[];
        added: SimNode[];
    } {
        const seen = new Set<string>();
        const kept: SimNode[] = [];
        const added: SimNode[] = [];
        const seedX =
            focus && nodeById.has(focus) ? (nodeById.get(focus)!.x ?? 0) : 0;
        const seedY =
            focus && nodeById.has(focus) ? (nodeById.get(focus)!.y ?? 0) : 0;
        for (const n of next) {
            seen.add(n.label);
            const existing = nodeById.get(n.label);
            if (existing) {
                existing.name = n.name;
                existing.type = n.type;
                existing.colourIndex = n.colourIndex;
                kept.push(existing);
            } else {
                added.push({
                    id: n.label,
                    name: n.name,
                    type: n.type,
                    colourIndex: n.colourIndex,
                    x: seedX + (Math.random() - 0.5) * 40,
                    y: seedY + (Math.random() - 0.5) * 40,
                });
            }
        }
        return { kept, added };
    }

    function buildLinks(
        payload: GraphPayload,
        ids: Map<string, SimNode>,
    ): SimLink[] {
        const out: SimLink[] = [];
        for (const e of payload.defEdges ?? []) {
            const s = ids.get(e.from);
            const t = ids.get(e.to);
            if (!s || !t) continue;
            out.push({ source: s, target: t, count: e.count });
        }
        return out;
    }

    function applySimulationContents(): void {
        simulation.nodes([...nodes, ...centroids]);
        const entityLinks =
            simulation.force<ReturnType<typeof forceLink<SimNode, SimLink>>>(
                "link",
            );
        if (entityLinks) entityLinks.links(links);
        // Centroid links live in our closure; memberAttractForce reads
        // them directly each tick — no force registration needed.
    }

    function emitPositions(): void {
        const map = new Map<string, { x: number; y: number }>();
        for (const n of nodes) map.set(n.id, { x: n.x ?? 0, y: n.y ?? 0 });
        for (const c of centroids) {
            map.set(c.id, { x: c.x ?? 0, y: c.y ?? 0 });
        }
        renderer.setPositions(map);
    }

    function pushDataToRenderer(): void {
        const rNodes: RenderNode[] = nodes.map((n) => ({
            id: n.id,
            name: n.name,
            type: n.type,
            colourIndex: n.colourIndex,
        }));
        const rLinks: RenderLink[] = links.map((l) => ({
            source: (l.source as SimNode).id,
            target: (l.target as SimNode).id,
            count: l.count,
        }));
        const rCentroids: RenderCentroid[] = centroids.map((c) => ({
            id: c.id,
            type: c.type ?? "(untyped)",
        }));
        renderer.setData(rNodes, rLinks, rCentroids);
        emitPositions();
    }

    function update(
        payload: GraphPayload,
        nextFocus: string | null | undefined,
    ): void {
        focus = nextFocus ?? focus;

        const { kept, added } = diffNodes(payload.nodes ?? []);
        nodes = [...kept, ...added];
        nodeById = new Map(nodes.map((n) => [n.id, n]));
        links = buildLinks(payload, nodeById);
        rebuildCentroids();
        applySimulationContents();

        renderer.setFocus(focus);
        pushDataToRenderer();

        simulation.alpha(Math.max(simulation.alpha(), 0.4)).restart();
    }

    function setFocus(label: string | null | undefined): void {
        focus = label ?? null;
        renderer.setFocus(focus);
        simulation.alpha(Math.max(simulation.alpha(), 0.05)).restart();
    }

    function setHopLimit(limit: number | null): void {
        renderer.setHopLimit(limit);
    }

    function setTypeFilter(types: string[] | null): void {
        renderer.setTypeFilter(types);
    }

    function setClustering(on: boolean): void {
        if (on === clustering) return;
        clustering = on;
        rebuildCentroids();
        applySimulationContents();
        pushDataToRenderer();
        simulation.alpha(Math.max(simulation.alpha(), 0.2)).restart();
    }

    function setArrowSize(size: number): void {
        renderer.setArrowSize(size);
    }

    function bumpSim(target = 0.4): void {
        simulation.alpha(Math.max(simulation.alpha(), target)).restart();
    }

    function setLinkDistance(d: number): void {
        if (d === linkDistance) return;
        linkDistance = d;
        const f =
            simulation.force<ReturnType<typeof forceLink<SimNode, SimLink>>>(
                "link",
            );
        if (f) f.distance(d);
        bumpSim();
    }

    function setLinkStrength(s: number): void {
        if (s === linkStrength) return;
        linkStrength = s;
        const f =
            simulation.force<ReturnType<typeof forceLink<SimNode, SimLink>>>(
                "link",
            );
        if (f) f.strength(s);
        bumpSim();
    }

    function setClusterStrength(s: number): void {
        if (s === clusterStrength) return;
        clusterStrength = s;
        bumpSim();
    }

    function setClusterDistance(d: number): void {
        if (d === clusterDistance) return;
        clusterDistance = d;
        // memberAttractForce reads clusterDistance via closure, so the
        // change takes effect on the next tick — no force rebind needed.
        bumpSim();
    }

    function setChargeStrength(s: number): void {
        if (s === chargeStrength) return;
        chargeStrength = s;
        const f =
            simulation.force<ReturnType<typeof forceManyBody<SimNode>>>(
                "charge",
            );
        if (f) f.strength(s);
        bumpSim();
    }

    function setCentroidGravity(s: number): void {
        if (s === centroidGravity) return;
        centroidGravity = s;
        const gx =
            simulation.force<ReturnType<typeof forceX<SimNode>>>(
                "centroidGravityX",
            );
        const gy =
            simulation.force<ReturnType<typeof forceY<SimNode>>>(
                "centroidGravityY",
            );
        if (gx) gx.strength(centroidGravityFor);
        if (gy) gy.strength(centroidGravityFor);
        bumpSim();
    }

    function dispose(): void {
        simulation.stop();
        renderer.dispose();
    }

    return {
        update,
        setFocus,
        setHopLimit,
        setArrowSize,
        setTypeFilter,
        setClustering,
        setLinkDistance,
        setLinkStrength,
        setClusterStrength,
        setClusterDistance,
        setChargeStrength,
        setCentroidGravity,
        dispose,
    };
}
