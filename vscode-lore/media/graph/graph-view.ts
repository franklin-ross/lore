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
    hop: number;
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
const FOCUS_RADIUS = 10;
const CENTROID_RADIUS = 18;
const LABEL_OFFSET = 11;
const HOP_FADE = [1, 0.6, 0.25, 0.1, 0.05];
// Labels are only painted for nodes within this many hops of the focus.
// Focus + immediate neighbours show names; everything beyond stays a dot
// so the layout reads as a structure first, names second. No focus =
// every label visible (initial overview when nothing's selected).
const LABEL_HOP_LIMIT = 1;

/**
 * mountGraph paints a force-directed graph into `host` and keeps it
 * incrementally in sync as payloads arrive. Node positions persist
 * across updates: kept nodes retain their (x, y) so edits don't reflow
 * the whole layout, new nodes spawn at the focused node (or centroid),
 * and removed nodes simply disappear. The simulation runs at low alpha
 * so it self-settles without yanking the layout around.
 */
export function mountGraph(
    host: HTMLElement,
    handlers: GraphHandlers,
    getPalette: () => string[],
): GraphView {
    host.innerHTML = "";

    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.classList.add("graph-svg");
    svg.setAttribute("xmlns", "http://www.w3.org/2000/svg");
    host.appendChild(svg);

    // Defs holds palette-coloured arrow markers — one per palette index, so
    // a directed edge can pick up the source entity's hue without an inline
    // marker per edge.
    const defs = document.createElementNS("http://www.w3.org/2000/svg", "defs");
    svg.appendChild(defs);

    const viewport = document.createElementNS(
        "http://www.w3.org/2000/svg",
        "g",
    );
    viewport.classList.add("graph-viewport");
    svg.appendChild(viewport);

    const linkLayer = document.createElementNS(
        "http://www.w3.org/2000/svg",
        "g",
    );
    linkLayer.classList.add("graph-links");
    viewport.appendChild(linkLayer);
    const nodeLayer = document.createElementNS(
        "http://www.w3.org/2000/svg",
        "g",
    );
    nodeLayer.classList.add("graph-nodes");
    viewport.appendChild(nodeLayer);
    // Debug overlay layer for centroid markers — sits above nodes so
    // the dashed ring + type label are visible regardless of node
    // density beneath. Empty when clustering is off.
    const centroidLayer = document.createElementNS(
        "http://www.w3.org/2000/svg",
        "g",
    );
    centroidLayer.classList.add("graph-centroids");
    viewport.appendChild(centroidLayer);

    let nodes: SimNode[] = [];
    let links: SimLink[] = [];
    // Ghost centroid nodes — one per type, present in the simulation only
    // while clustering is on. Members get link-pulled toward their type's
    // centroid; centroids inherit the shared charge force and so push
    // each other apart naturally.
    let centroids: SimNode[] = [];
    let centroidLinks: SimLink[] = [];
    let nodeById = new Map<string, SimNode>();
    let nodeEls = new Map<
        string,
        { circle: SVGCircleElement; label: SVGTextElement }
    >();
    let centroidEls = new Map<
        string,
        { circle: SVGCircleElement; label: SVGTextElement }
    >();
    let linkEls = new Map<
        string,
        {
            line: SVGLineElement;
            gradient: SVGLinearGradientElement;
            stopFrom: SVGStopElement;
            stopTo: SVGStopElement;
        }
    >();
    let focus: string | null = null;
    let hoveredId: string | null = null;
    let hopLimit: number | null = null;
    let arrowSize = 0;
    let typeFilter: Set<string> | null = null;
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
    let palette: string[] = getPalette();
    let gradientCounter = 0;

    const transform = { x: 0, y: 0, k: 1 };

    function applyTransform(): void {
        viewport.setAttribute(
            "transform",
            `translate(${transform.x},${transform.y}) scale(${transform.k})`,
        );
    }
    applyTransform();

    function ensureMarkers(): void {
        // arrowSize===0 disables arrowheads entirely — strip any existing
        // markers so paintLinks can simply omit marker-end without leaving
        // orphan defs around.
        if (arrowSize <= 0) {
            for (const m of defs.querySelectorAll("marker")) m.remove();
            return;
        }
        // Rebuild when palette size changed (different number of markers
        // expected). Existing markers' attributes are kept current via the
        // explicit rebuild path below.
        const existing = defs.querySelectorAll("marker");
        if (existing.length === palette.length) return;
        for (const m of existing) m.remove();
        for (let i = 0; i < palette.length; i++) {
            const marker = document.createElementNS(
                "http://www.w3.org/2000/svg",
                "marker",
            );
            marker.setAttribute("id", `arrow-${i}`);
            marker.setAttribute("viewBox", "0 -5 10 10");
            // viewBox runs 0..10 across the arrow path with the tip at x=10.
            // refX=10 anchors the tip exactly at the line's end coordinate —
            // and drawTick trims line endpoints to the node surface, so the
            // arrow tip lands on the circle's edge regardless of node size.
            marker.setAttribute("refX", "10");
            marker.setAttribute("refY", "0");
            marker.setAttribute("markerUnits", "userSpaceOnUse");
            marker.setAttribute("markerWidth", String(arrowSize));
            marker.setAttribute("markerHeight", String(arrowSize));
            marker.setAttribute("orient", "auto");
            const path = document.createElementNS(
                "http://www.w3.org/2000/svg",
                "path",
            );
            path.setAttribute("d", "M0,-4L10,0L0,4");
            path.setAttribute("fill", palette[i] ?? "#888");
            marker.appendChild(path);
            defs.appendChild(marker);
        }
    }

    // resizeMarkers updates existing markers' size in place — cheaper than
    // rebuilding when only the size changed.
    function resizeMarkers(): void {
        for (const m of defs.querySelectorAll("marker")) {
            m.setAttribute("markerWidth", String(arrowSize));
            m.setAttribute("markerHeight", String(arrowSize));
        }
    }

    function colourFor(idx: number): string {
        return palette[idx] ?? "#888";
    }

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
        // Hierarchical cluster pull: each member links to its type's
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
        .on("tick", drawTick);

    // centroidLinkStrengthFor scales the member→centroid spring by the
    // inverse sqrt of cluster size. Small clusters (count → 1) get the
    // full clusterStrength; big clusters (count → 30) get clusterStrength
    // / sqrt(30) ≈ 0.18× — members spread further before the centroid
    // pull dominates.
    function centroidLinkStrengthFor(l: SimLink): number {
        const target = l.target as SimNode;
        const t = target.type ?? "(untyped)";
        const count = memberCountByType.get(t) ?? 1;
        return clusterStrength / Math.sqrt(Math.max(1, count));
    }

    // memberAttractForce is a half-spring: only attracts when member is
    // beyond clusterDistance from its centroid. Inside that radius the
    // force is zero, so charge alone shapes cluster interior. Most of
    // the displacement lands on the member; centroids drift slightly
    // toward members that overshoot so they track their cluster's
    // mass.
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
            // Quadratic in overshoot, capped: at ratio=1 the force
            // matches the previous linear model; beyond that, escape
            // gets harder until ratio hits the cap, then constant.
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
    // current node list. Called whenever clustering toggles on or the
    // type set changes. Existing centroids are kept (positions
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
                    hop: Number.POSITIVE_INFINITY,
                    isCentroid: true,
                    // Spawn near origin with a small random offset; physics
                    // (charge + cluster gravity + member edges) decides the
                    // long-term position.
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

    // applyClusterStrength is a no-op now that the member-attract force
    // reads clusterStrength via closure on every tick. Kept as a hook
    // so callers can signal "force config changed" without knowing the
    // implementation.
    function applyClusterStrength(): void {}

    function linkKey(l: SimLink): string {
        const s =
            typeof l.source === "object"
                ? (l.source as SimNode).id
                : String(l.source);
        const t =
            typeof l.target === "object"
                ? (l.target as SimNode).id
                : String(l.target);
        return `${s}→${t}`;
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
                    hop: Number.POSITIVE_INFINITY,
                    x: seedX + (Math.random() - 0.5) * 40,
                    y: seedY + (Math.random() - 0.5) * 40,
                });
            }
        }
        // Remove DOM elements for nodes that vanished.
        for (const id of nodeById.keys()) {
            if (seen.has(id)) continue;
            const els = nodeEls.get(id);
            if (els) {
                els.circle.remove();
                els.label.remove();
                nodeEls.delete(id);
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

    function recomputeHops(): void {
        for (const n of nodes) n.hop = Number.POSITIVE_INFINITY;
        if (!focus) {
            for (const n of nodes) n.hop = 0;
            return;
        }
        const start = nodeById.get(focus);
        if (!start) return;
        start.hop = 0;
        const adj = new Map<string, Set<string>>();
        const ensure = (id: string) => {
            let s = adj.get(id);
            if (!s) {
                s = new Set();
                adj.set(id, s);
            }
            return s;
        };
        for (const l of links) {
            const s = (l.source as SimNode).id;
            const t = (l.target as SimNode).id;
            ensure(s).add(t);
            ensure(t).add(s);
        }
        const queue: SimNode[] = [start];
        while (queue.length > 0) {
            const cur = queue.shift()!;
            for (const next of adj.get(cur.id) ?? []) {
                const node = nodeById.get(next);
                if (!node) continue;
                if (node.hop !== Number.POSITIVE_INFINITY) continue;
                node.hop = cur.hop + 1;
                queue.push(node);
            }
        }
    }

    // nodeVisibilityOpacity returns the opacity to paint a node's circle and
    // (subject to label gating) its label. The focused node is always
    // visible (otherwise filters would orphan the user's selection). Two
    // hard cutoffs return 0 for everything else: hop-limit (too far from
    // focus) and type-filter (whitelist set, type not in it). Either way
    // the node stays in the sim so positions stay sane when the cutoff
    // relaxes.
    function nodeVisibilityOpacity(n: SimNode): number {
        if (focus === n.id) return 1;
        if (typeFilter && !typeFilter.has(n.type ?? "(untyped)")) return 0;
        if (hopLimit !== null && n.hop > hopLimit) return 0;
        return HOP_FADE[Math.min(n.hop, HOP_FADE.length - 1)] ?? 0.05;
    }

    // labelOpacityFor decides whether to show a node's label at all. Once
    // visible labels paint at full opacity regardless of node fade — fading
    // the names alongside their dots makes hop-distant labels unreadable.
    // Focus distinction goes to font weight (see paintNodes), not opacity.
    // When a type filter is active the user has narrowed the view
    // intentionally; show every remaining label so they can read what they
    // kept rather than re-clicking each node.
    function labelOpacityFor(n: SimNode, nodeOpacity: number): number {
        if (nodeOpacity === 0) return 0;
        if (hoveredId === n.id) return 1;
        if (typeFilter !== null) return 1;
        const isFocused = focus === n.id;
        if (focus === null || isFocused || n.hop <= LABEL_HOP_LIMIT) return 1;
        return 0;
    }

    function paintNodes(): void {
        for (const n of nodes) {
            let els = nodeEls.get(n.id);
            if (!els) {
                const circle = document.createElementNS(
                    "http://www.w3.org/2000/svg",
                    "circle",
                );
                circle.classList.add("graph-node");
                const label = document.createElementNS(
                    "http://www.w3.org/2000/svg",
                    "text",
                );
                label.classList.add("graph-label");
                // Use the server's disambiguation-aware label — bare name when
                // unique, "Name (type)" when the bare name is shared. Same
                // string the wiki uses, so the two views read consistently.
                label.textContent = n.id;
                nodeLayer.appendChild(circle);
                nodeLayer.appendChild(label);
                attachNodeHandlers(circle, n);
                attachNodeHandlers(label, n);
                els = { circle, label };
                nodeEls.set(n.id, els);
            }
            const isFocused = focus === n.id;
            const fill = colourFor(n.colourIndex);
            els.circle.setAttribute("fill", fill);
            els.circle.setAttribute(
                "r",
                String(isFocused ? FOCUS_RADIUS : NODE_RADIUS),
            );
            const nodeOpacity = nodeVisibilityOpacity(n);
            els.circle.setAttribute("opacity", String(nodeOpacity));
            // Hidden nodes shouldn't catch clicks or steal focus — they're still
            // there for the simulation, but the user can't interact with them.
            els.circle.style.pointerEvents =
                nodeOpacity === 0 ? "none" : "auto";
            els.label.setAttribute(
                "opacity",
                String(labelOpacityFor(n, nodeOpacity)),
            );
            els.circle.classList.toggle("focused", isFocused);
            els.label.classList.toggle("focused", isFocused);
        }
    }

    // repaintLabel updates one node's label opacity in place. Used by the
    // hover handlers so we don't repaint every node on a pointerenter.
    function repaintLabel(n: SimNode): void {
        const els = nodeEls.get(n.id);
        if (!els) return;
        els.label.setAttribute(
            "opacity",
            String(labelOpacityFor(n, nodeVisibilityOpacity(n))),
        );
    }

    // paintLinks creates one linearGradient per edge and uses it as the
    // line's stroke. Each stop carries its endpoint's per-node opacity so
    // an edge that crosses the hop limit fades from solid at its in-scope
    // end to fully transparent at the hidden end. Both ends hidden ⇒ the
    // line is collapsed (display:none) so we don't pay for invisible draws.
    function paintLinks(): void {
        const seen = new Set<string>();
        for (const l of links) {
            const key = linkKey(l);
            seen.add(key);
            const s = l.source as SimNode;
            const t = l.target as SimNode;
            let bundle = linkEls.get(key);
            if (!bundle) {
                const gradientId = `lg-${gradientCounter++}`;
                const gradient = document.createElementNS(
                    "http://www.w3.org/2000/svg",
                    "linearGradient",
                );
                gradient.setAttribute("id", gradientId);
                gradient.setAttribute("gradientUnits", "userSpaceOnUse");
                const stopFrom = document.createElementNS(
                    "http://www.w3.org/2000/svg",
                    "stop",
                );
                stopFrom.setAttribute("offset", "0");
                const stopTo = document.createElementNS(
                    "http://www.w3.org/2000/svg",
                    "stop",
                );
                stopTo.setAttribute("offset", "1");
                gradient.appendChild(stopFrom);
                gradient.appendChild(stopTo);
                defs.appendChild(gradient);

                const line = document.createElementNS(
                    "http://www.w3.org/2000/svg",
                    "line",
                );
                line.classList.add("graph-edge-def");
                line.setAttribute("stroke", `url(#${gradientId})`);
                linkLayer.appendChild(line);

                bundle = { line, gradient, stopFrom, stopTo };
                linkEls.set(key, bundle);
            }

            // Stroke colour follows the source so hub colours tint their
            // outgoing arrows.
            const stroke = colourFor(s.colourIndex);
            bundle.stopFrom.setAttribute("stop-color", stroke);
            bundle.stopTo.setAttribute("stop-color", stroke);

            const opS = nodeVisibilityOpacity(s);
            const opT = nodeVisibilityOpacity(t);
            // Edges that cross the hop limit are still useful context ("the
            // other end is busy") but shouldn't compete with edges that sit
            // entirely inside the focused subgraph. Knock the in-scope end's
            // opacity down when the opposite end is hidden so the line reads
            // as background.
            const CROSS_BOUNDARY_FADE = 0.35;
            const sFade = opT === 0 && opS > 0 ? CROSS_BOUNDARY_FADE : 1;
            const tFade = opS === 0 && opT > 0 ? CROSS_BOUNDARY_FADE : 1;
            bundle.stopFrom.setAttribute(
                "stop-opacity",
                String(opS * sFade * 0.7),
            );
            bundle.stopTo.setAttribute(
                "stop-opacity",
                String(opT * tFade * 0.7),
            );

            const visible = opS > 0 || opT > 0;
            bundle.line.style.display = visible ? "" : "none";

            // Markers are opt-in (arrowSize === 0 disables them entirely) and
            // hidden when the target end is faded out — the marker is solid
            // colour and would otherwise float past the cutoff.
            if (arrowSize > 0 && opT > 0) {
                bundle.line.setAttribute(
                    "marker-end",
                    `url(#arrow-${s.colourIndex})`,
                );
            } else {
                bundle.line.removeAttribute("marker-end");
            }
        }
        for (const [k, el] of linkEls) {
            if (!seen.has(k)) {
                el.line.remove();
                el.gradient.remove();
                linkEls.delete(k);
            }
        }
    }

    function drawTick(): void {
        for (const n of nodes) {
            const els = nodeEls.get(n.id);
            if (!els) continue;
            els.circle.setAttribute("cx", String(n.x ?? 0));
            els.circle.setAttribute("cy", String(n.y ?? 0));
            els.label.setAttribute("x", String((n.x ?? 0) + LABEL_OFFSET));
            els.label.setAttribute("y", String((n.y ?? 0) + 4));
        }
        for (const l of links) {
            const key = linkKey(l);
            const bundle = linkEls.get(key);
            if (!bundle) continue;
            const s = l.source as SimNode;
            const t = l.target as SimNode;
            const sx = s.x ?? 0;
            const sy = s.y ?? 0;
            const tx = t.x ?? 0;
            const ty = t.y ?? 0;
            // Trim line endpoints to each node's surface so faded circles
            // don't show line stubs poking through to centre. Use the live
            // radius (focused nodes are bigger) so the trim follows whatever
            // the node currently paints as.
            const dx = tx - sx;
            const dy = ty - sy;
            const dist = Math.hypot(dx, dy);
            const sR = focus === s.id ? FOCUS_RADIUS : NODE_RADIUS;
            const tR = focus === t.id ? FOCUS_RADIUS : NODE_RADIUS;
            let x1 = sx;
            let y1 = sy;
            let x2 = tx;
            let y2 = ty;
            if (dist > sR + tR) {
                const ux = dx / dist;
                const uy = dy / dist;
                x1 = sx + ux * sR;
                y1 = sy + uy * sR;
                x2 = tx - ux * tR;
                y2 = ty - uy * tR;
            }
            bundle.line.setAttribute("x1", String(x1));
            bundle.line.setAttribute("y1", String(y1));
            bundle.line.setAttribute("x2", String(x2));
            bundle.line.setAttribute("y2", String(y2));
            // Gradient uses userSpaceOnUse, so its endpoints must track the
            // line every tick or stops anchor to their last known coordinates.
            bundle.gradient.setAttribute("x1", String(x1));
            bundle.gradient.setAttribute("y1", String(y1));
            bundle.gradient.setAttribute("x2", String(x2));
            bundle.gradient.setAttribute("y2", String(y2));
        }
        for (const c of centroids) {
            const els = centroidEls.get(c.id);
            if (!els) continue;
            els.circle.setAttribute("cx", String(c.x ?? 0));
            els.circle.setAttribute("cy", String(c.y ?? 0));
            els.label.setAttribute("x", String(c.x ?? 0));
            els.label.setAttribute("y", String((c.y ?? 0) - CENTROID_RADIUS - 6));
        }
    }

    // paintCentroids creates the dashed-ring + type label overlay for
    // each ghost centroid. Called after rebuildCentroids; keeps DOM in
    // sync with the centroid set so toggling Cluster off/on doesn't
    // leave orphan markers.
    function paintCentroids(): void {
        const seen = new Set<string>();
        for (const c of centroids) {
            seen.add(c.id);
            let els = centroidEls.get(c.id);
            if (!els) {
                const circle = document.createElementNS(
                    "http://www.w3.org/2000/svg",
                    "circle",
                );
                circle.classList.add("graph-centroid");
                circle.setAttribute("r", String(CENTROID_RADIUS));
                const label = document.createElementNS(
                    "http://www.w3.org/2000/svg",
                    "text",
                );
                label.classList.add("graph-centroid-label");
                label.textContent = c.type ?? "(untyped)";
                centroidLayer.appendChild(circle);
                centroidLayer.appendChild(label);
                els = { circle, label };
                centroidEls.set(c.id, els);
            }
        }
        for (const [id, els] of centroidEls) {
            if (seen.has(id)) continue;
            els.circle.remove();
            els.label.remove();
            centroidEls.delete(id);
        }
    }

    function attachNodeHandlers(el: SVGElement, node: SimNode): void {
        // Two-phase drag: pointerdown only arms the watch; we don't restart
        // the simulation or pin the node until the pointer actually moves
        // past DRAG_THRESHOLD. A clean click (no movement) leaves the sim
        // alone — earlier behaviour was bumping alpha on every click,
        // which kept the layout perpetually unsettled when the user was
        // just selecting nodes.
        const DRAG_THRESHOLD = 3;
        let watching = false;
        let dragStarted = false;
        let pointerId = -1;
        let downAt = { x: 0, y: 0 };
        el.addEventListener("pointerdown", (ev) => {
            ev.stopPropagation();
            watching = true;
            dragStarted = false;
            pointerId = ev.pointerId;
            el.setPointerCapture(pointerId);
            downAt = { x: ev.clientX, y: ev.clientY };
        });
        el.addEventListener("pointermove", (ev) => {
            if (!watching) return;
            if (!dragStarted) {
                if (
                    Math.hypot(ev.clientX - downAt.x, ev.clientY - downAt.y) <=
                    DRAG_THRESHOLD
                )
                    return;
                dragStarted = true;
                simulation.alphaTarget(0.3).restart();
                node.fx = node.x;
                node.fy = node.y;
            }
            const pt = clientToWorld(ev.clientX, ev.clientY);
            node.fx = pt.x;
            node.fy = pt.y;
        });
        el.addEventListener("pointerup", () => {
            if (!watching) return;
            watching = false;
            el.releasePointerCapture(pointerId);
            if (dragStarted) {
                simulation.alphaTarget(0);
                node.fx = null;
                node.fy = null;
            } else {
                handlers.onFocus(node.id);
            }
        });
        el.addEventListener("dblclick", (ev) => {
            ev.stopPropagation();
            handlers.onOpenEntity(node.id);
        });
        el.addEventListener("pointerenter", () => {
            hoveredId = node.id;
            repaintLabel(node);
        });
        el.addEventListener("pointerleave", () => {
            if (hoveredId !== node.id) return;
            hoveredId = null;
            repaintLabel(node);
        });
    }

    function clientToWorld(cx: number, cy: number): { x: number; y: number } {
        const rect = svg.getBoundingClientRect();
        const sx = cx - rect.left - rect.width / 2 - transform.x;
        const sy = cy - rect.top - rect.height / 2 - transform.y;
        return { x: sx / transform.k, y: sy / transform.k };
    }

    // Pan with empty-space drag. Tracks whether the pointer actually
    // moved between down and up so a stationary click on the background
    // can be treated as "deselect focus" rather than a no-op pan.
    let panActive = false;
    let panLast = { x: 0, y: 0 };
    let panStart = { x: 0, y: 0 };
    let panMoved = false;
    const PAN_THRESHOLD = 3; // pixels — below this, treat as a click
    svg.addEventListener("pointerdown", (ev) => {
        if (
            ev.target !== svg &&
            ev.target !== viewport &&
            ev.target !== linkLayer &&
            ev.target !== nodeLayer
        )
            return;
        panActive = true;
        panMoved = false;
        panLast = { x: ev.clientX, y: ev.clientY };
        panStart = { x: ev.clientX, y: ev.clientY };
        svg.setPointerCapture(ev.pointerId);
    });
    svg.addEventListener("pointermove", (ev) => {
        if (!panActive) return;
        if (
            !panMoved &&
            Math.hypot(ev.clientX - panStart.x, ev.clientY - panStart.y) >
                PAN_THRESHOLD
        ) {
            panMoved = true;
        }
        transform.x += ev.clientX - panLast.x;
        transform.y += ev.clientY - panLast.y;
        panLast = { x: ev.clientX, y: ev.clientY };
        applyTransform();
    });
    svg.addEventListener("pointerup", (ev) => {
        if (!panActive) return;
        panActive = false;
        svg.releasePointerCapture(ev.pointerId);
        if (!panMoved) handlers.onFocus(null);
    });

    // Zoom with wheel — zoom around cursor so the spot under the pointer
    // stays put.
    svg.addEventListener(
        "wheel",
        (ev) => {
            ev.preventDefault();
            const factor = Math.exp(-ev.deltaY * 0.001);
            const rect = svg.getBoundingClientRect();
            const cx = ev.clientX - rect.left - rect.width / 2;
            const cy = ev.clientY - rect.top - rect.height / 2;
            const wx = (cx - transform.x) / transform.k;
            const wy = (cy - transform.y) / transform.k;
            transform.k = Math.max(0.2, Math.min(4, transform.k * factor));
            transform.x = cx - wx * transform.k;
            transform.y = cy - wy * transform.k;
            applyTransform();
        },
        { passive: false },
    );

    // Centre the simulation in the viewport once layout is known.
    const resizeObserver = new ResizeObserver(() => {
        const rect = host.getBoundingClientRect();
        svg.setAttribute("width", String(rect.width));
        svg.setAttribute("height", String(rect.height));
        svg.setAttribute(
            "viewBox",
            `${-rect.width / 2} ${-rect.height / 2} ${rect.width} ${rect.height}`,
        );
    });
    resizeObserver.observe(host);

    function update(
        payload: GraphPayload,
        nextFocus: string | null | undefined,
    ): void {
        palette = getPalette();
        ensureMarkers();
        focus = nextFocus ?? focus;

        const { kept, added } = diffNodes(payload.nodes ?? []);
        nodes = [...kept, ...added];
        nodeById = new Map(nodes.map((n) => [n.id, n]));
        links = buildLinks(payload, nodeById);
        rebuildCentroids();
        applySimulationContents();

        recomputeHops();
        paintNodes();
        paintLinks();
        paintCentroids();

        simulation.alpha(Math.max(simulation.alpha(), 0.4)).restart();
    }

    // applySimulationContents pushes the current node + link sets into
    // the simulation. Called whenever nodes/links/centroids change so
    // d3-force re-resolves link endpoints and re-binds per-node forces.
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

    function setFocus(label: string | null | undefined): void {
        focus = label ?? null;
        recomputeHops();
        paintNodes();
        paintLinks();
        simulation.alpha(Math.max(simulation.alpha(), 0.05)).restart();
    }

    // setHopLimit gates which nodes are visible: nodes within `limit` hops
    // of the focus paint normally, anything farther fades to 0. Pass null
    // to drop the cap and show every node. The simulation keeps running
    // over the full graph either way so positions stay consistent when
    // the user widens the scope back out.
    function setHopLimit(limit: number | null): void {
        if (limit === hopLimit) return;
        hopLimit = limit;
        paintNodes();
        paintLinks();
    }

    // setTypeFilter limits which entity types are visible. null clears
    // the filter so every type shows. Like the hop limit, filtered-out
    // nodes stay in the simulation but render at opacity 0 so their
    // neighbours' positions don't snap when the user toggles the filter.
    function setTypeFilter(types: string[] | null): void {
        typeFilter = types ? new Set(types) : null;
        paintNodes();
        paintLinks();
    }

    // setClustering toggles hierarchical type clustering. On, ghost
    // centroids are inserted (one per type) and members link to their
    // centroid; centroid-vs-centroid charge repulsion plus per-cluster
    // link tension produces the layout. Off, centroids drop out
    // entirely and only entity edges + charge run.
    function setClustering(on: boolean): void {
        if (on === clustering) return;
        clustering = on;
        rebuildCentroids();
        applySimulationContents();
        applyClusterStrength();
        paintCentroids();
        simulation.alpha(Math.max(simulation.alpha(), 0.2)).restart();
    }

    // setArrowSize swaps the arrowhead size in user-space units. 0 strips
    // markers entirely; positive values rebuild or resize the marker pool
    // and re-attach marker-end to visible edges.
    function setArrowSize(size: number): void {
        if (size === arrowSize) return;
        const wasOff = arrowSize <= 0;
        const goingOff = size <= 0;
        arrowSize = size;
        if (goingOff || wasOff) {
            ensureMarkers();
        } else {
            resizeMarkers();
        }
        paintLinks();
    }

    // Layout-knob setters all bump alpha so the change reflows the
    // existing positions instead of waiting for the next user action.
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
        applyClusterStrength();
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
        resizeObserver.disconnect();
        host.innerHTML = "";
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
