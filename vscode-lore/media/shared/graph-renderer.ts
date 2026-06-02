// Renderer-only layer for the knowledge graph webview. Owns the SVG,
// paint loops, hover/drag/pan/zoom, hop fade, label gating, edge
// gradient + trim, marker pool, and type filter. Knows nothing about
// physics — positions arrive through setPositions(); a layout engine
// drives the simulation and reports each tick.

export interface RenderNode {
    id: string;
    name: string;
    type?: string;
    colourIndex: number;
}

export interface RenderLink {
    source: string;
    target: string;
    count: number;
    // "relation" edges are explicit typed relationships; "mention" edges are
    // reference-derived. Defaults to mention when unset. Relations render
    // bolder; symmetric relations (spouse, sibling) omit the arrowhead.
    kind?: "mention" | "relation";
    symmetric?: boolean;
}

export interface RenderCentroid {
    id: string;
    type: string;
}

export interface RendererHandlers {
    onNodeClick(id: string): void;
    onNodeOpen(id: string): void;
    onBackgroundClick(): void;
    onNodeDragStart(id: string): void;
    onNodeDragMove(id: string, worldX: number, worldY: number): void;
    onNodeDragEnd(id: string): void;
}

export interface GraphRenderer {
    setData(
        nodes: RenderNode[],
        links: RenderLink[],
        centroids?: RenderCentroid[],
    ): void;
    setPositions(positions: Map<string, { x: number; y: number }>): void;
    setFocus(label: string | null): void;
    setHopLimit(limit: number | null): void;
    setArrowSize(size: number): void;
    setTypeFilter(types: string[] | null): void;
    // When true, the viewport is translated on every tick so the focused
    // node sits at the SVG origin (which the viewBox places at host centre).
    // Pan input is ignored while anchored — zoom is allowed and re-pins the
    // anchor on each step. Used by the embedded wiki graph; the standalone
    // graph panel leaves it off so the user can compose freely.
    setAnchorFocusToCentre(on: boolean): void;
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

export function mountRenderer(
    host: HTMLElement,
    handlers: RendererHandlers,
    getPalette: () => string[],
): GraphRenderer {
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

    let nodes: RenderNode[] = [];
    let links: RenderLink[] = [];
    let centroids: RenderCentroid[] = [];
    let nodeById = new Map<string, RenderNode>();
    let positions = new Map<string, { x: number; y: number }>();
    let hops = new Map<string, number>();
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
    let palette: string[] = getPalette();
    let gradientCounter = 0;

    const transform = { x: 0, y: 0, k: 1 };
    let anchorFocusToCentre = false;

    // recentreOnFocus translates the viewport so the focused node lands at
    // SVG (0,0) — the viewBox places that point at the host's visual centre,
    // so this is "pan to focus" without the work of measuring host bounds.
    // Re-run on every position tick when anchorFocusToCentre is on so the
    // focus stays centred while the layout settles.
    function recentreOnFocus(): void {
        if (!focus) return;
        const p = positions.get(focus);
        if (!p) return;
        transform.x = -p.x * transform.k;
        transform.y = -p.y * transform.k;
        applyTransform();
    }

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
            // and the tick trim shrinks line endpoints to the node surface,
            // so the arrow tip lands on the circle's edge regardless of size.
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

    function resizeMarkers(): void {
        for (const m of defs.querySelectorAll("marker")) {
            m.setAttribute("markerWidth", String(arrowSize));
            m.setAttribute("markerHeight", String(arrowSize));
        }
    }

    function colourFor(idx: number): string {
        return palette[idx] ?? "#888";
    }

    function linkKey(l: RenderLink): string {
        // Kind is part of the key so a relation and a mention between the same
        // pair coexist as separate lines.
        return `${l.kind ?? "mention"}:${l.source}→${l.target}`;
    }

    function recomputeHops(): void {
        hops = new Map();
        if (!focus || !nodeById.has(focus)) {
            for (const n of nodes) hops.set(n.id, 0);
            return;
        }
        for (const n of nodes) hops.set(n.id, Number.POSITIVE_INFINITY);
        hops.set(focus, 0);
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
            ensure(l.source).add(l.target);
            ensure(l.target).add(l.source);
        }
        const queue: string[] = [focus];
        while (queue.length > 0) {
            const cur = queue.shift()!;
            const curHop = hops.get(cur) ?? 0;
            for (const next of adj.get(cur) ?? []) {
                if (!nodeById.has(next)) continue;
                const prev = hops.get(next);
                if (prev !== Number.POSITIVE_INFINITY) continue;
                hops.set(next, curHop + 1);
                queue.push(next);
            }
        }
    }

    // nodeVisibilityOpacity returns the opacity to paint a node's circle and
    // (subject to label gating) its label. The focused node is always
    // visible (otherwise filters would orphan the user's selection). Two
    // hard cutoffs return 0 for everything else: hop-limit (too far from
    // focus) and type-filter (whitelist set, type not in it). Either way
    // the node stays in the layout so positions stay sane when the cutoff
    // relaxes.
    function nodeVisibilityOpacity(n: RenderNode): number {
        if (focus === n.id) return 1;
        if (typeFilter && !typeFilter.has(n.type ?? "(untyped)")) return 0;
        const hop = hops.get(n.id) ?? Number.POSITIVE_INFINITY;
        if (hopLimit !== null && hop > hopLimit) return 0;
        return HOP_FADE[Math.min(hop, HOP_FADE.length - 1)] ?? 0.05;
    }

    // labelOpacityFor decides whether to show a node's label at all. Once
    // visible labels paint at full opacity regardless of node fade — fading
    // the names alongside their dots makes hop-distant labels unreadable.
    // Focus distinction goes to font weight (see paintNodes), not opacity.
    // When a type filter is active the user has narrowed the view
    // intentionally; show every remaining label so they can read what they
    // kept rather than re-clicking each node.
    function labelOpacityFor(n: RenderNode, nodeOpacity: number): number {
        if (nodeOpacity === 0) return 0;
        if (hoveredId === n.id) return 1;
        if (typeFilter !== null) return 1;
        const isFocused = focus === n.id;
        const hop = hops.get(n.id) ?? Number.POSITIVE_INFINITY;
        if (focus === null || isFocused || hop <= LABEL_HOP_LIMIT) return 1;
        return 0;
    }

    function paintNodes(): void {
        const seen = new Set<string>();
        for (const n of nodes) {
            seen.add(n.id);
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
        for (const [id, els] of nodeEls) {
            if (seen.has(id)) continue;
            els.circle.remove();
            els.label.remove();
            nodeEls.delete(id);
        }
    }

    function repaintLabel(n: RenderNode): void {
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
            const s = nodeById.get(l.source);
            const t = nodeById.get(l.target);
            if (!s || !t) continue;
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
                line.classList.add(
                    l.kind === "relation" ? "graph-edge-relation" : "graph-edge-def",
                );
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
            // Relations are the explicit, curated layer — draw them at full
            // strength; mentions sit back as fainter context.
            const base = l.kind === "relation" ? 1 : 0.5;
            bundle.stopFrom.setAttribute(
                "stop-opacity",
                String(opS * sFade * base),
            );
            bundle.stopTo.setAttribute(
                "stop-opacity",
                String(opT * tFade * base),
            );

            const visible = opS > 0 || opT > 0;
            bundle.line.style.display = visible ? "" : "none";

            // Directed edges get an arrowhead; symmetric relations (spouse,
            // sibling) read as undirected, so skip it.
            if (arrowSize > 0 && opT > 0 && !l.symmetric) {
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
                label.textContent = c.type;
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

    function applyPositions(): void {
        for (const n of nodes) {
            const els = nodeEls.get(n.id);
            if (!els) continue;
            const p = positions.get(n.id);
            if (!p) continue;
            els.circle.setAttribute("cx", String(p.x));
            els.circle.setAttribute("cy", String(p.y));
            els.label.setAttribute("x", String(p.x + LABEL_OFFSET));
            els.label.setAttribute("y", String(p.y + 4));
        }
        for (const l of links) {
            const key = linkKey(l);
            const bundle = linkEls.get(key);
            if (!bundle) continue;
            const sp = positions.get(l.source);
            const tp = positions.get(l.target);
            if (!sp || !tp) continue;
            const sx = sp.x;
            const sy = sp.y;
            const tx = tp.x;
            const ty = tp.y;
            // Trim line endpoints to each node's surface so faded circles
            // don't show line stubs poking through to centre. Use the live
            // radius (focused nodes are bigger) so the trim follows whatever
            // the node currently paints as.
            const dx = tx - sx;
            const dy = ty - sy;
            const dist = Math.hypot(dx, dy);
            const sR = focus === l.source ? FOCUS_RADIUS : NODE_RADIUS;
            const tR = focus === l.target ? FOCUS_RADIUS : NODE_RADIUS;
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
            const p = positions.get(c.id);
            if (!p) continue;
            els.circle.setAttribute("cx", String(p.x));
            els.circle.setAttribute("cy", String(p.y));
            els.label.setAttribute("x", String(p.x));
            els.label.setAttribute("y", String(p.y - CENTROID_RADIUS - 6));
        }
    }

    function attachNodeHandlers(el: SVGElement, node: RenderNode): void {
        // Two-phase drag: pointerdown only arms the watch; we don't notify
        // the engine until the pointer actually moves past DRAG_THRESHOLD.
        // A clean click (no movement) routes to onNodeClick — earlier
        // behaviour bumped sim alpha on every click, keeping the layout
        // perpetually unsettled when the user was just selecting nodes.
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
                handlers.onNodeDragStart(node.id);
            }
            const pt = clientToWorld(ev.clientX, ev.clientY);
            handlers.onNodeDragMove(node.id, pt.x, pt.y);
        });
        el.addEventListener("pointerup", () => {
            if (!watching) return;
            watching = false;
            el.releasePointerCapture(pointerId);
            if (dragStarted) {
                handlers.onNodeDragEnd(node.id);
            } else {
                handlers.onNodeClick(node.id);
            }
        });
        el.addEventListener("dblclick", (ev) => {
            ev.stopPropagation();
            handlers.onNodeOpen(node.id);
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
    const PAN_THRESHOLD = 3;
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
        // Anchor mode keeps the focused node pinned to centre — pan input
        // would fight that on every tick, so we just ignore translation
        // here. Zoom (below) is still honoured and re-anchors after.
        if (!anchorFocusToCentre) {
            transform.x += ev.clientX - panLast.x;
            transform.y += ev.clientY - panLast.y;
            applyTransform();
        }
        panLast = { x: ev.clientX, y: ev.clientY };
    });
    svg.addEventListener("pointerup", (ev) => {
        if (!panActive) return;
        panActive = false;
        svg.releasePointerCapture(ev.pointerId);
        if (!panMoved) handlers.onBackgroundClick();
    });

    svg.addEventListener(
        "wheel",
        (ev) => {
            ev.preventDefault();
            const factor = Math.exp(-ev.deltaY * 0.001);
            // Anchor mode: zoom around the focused node rather than the
            // cursor so the focus stays pinned to centre across zoom steps.
            if (anchorFocusToCentre) {
                transform.k = Math.max(0.2, Math.min(4, transform.k * factor));
                recentreOnFocus();
                return;
            }
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

    function setData(
        nextNodes: RenderNode[],
        nextLinks: RenderLink[],
        nextCentroids?: RenderCentroid[],
    ): void {
        palette = getPalette();
        nodes = nextNodes;
        links = nextLinks;
        centroids = nextCentroids ?? [];
        nodeById = new Map(nodes.map((n) => [n.id, n]));
        // Drop cached positions for nodes/centroids that left so the
        // map doesn't grow unbounded across sessions.
        const live = new Set<string>();
        for (const n of nodes) live.add(n.id);
        for (const c of centroids) live.add(c.id);
        for (const id of [...positions.keys()]) {
            if (!live.has(id)) positions.delete(id);
        }
        ensureMarkers();
        recomputeHops();
        paintNodes();
        paintLinks();
        paintCentroids();
        applyPositions();
    }

    function setPositions(
        next: Map<string, { x: number; y: number }>,
    ): void {
        positions = next;
        if (anchorFocusToCentre) recentreOnFocus();
        applyPositions();
    }

    function setFocus(label: string | null): void {
        focus = label;
        recomputeHops();
        paintNodes();
        paintLinks();
        // Re-trim edge endpoints — the focused node's radius changed,
        // so edges incident to the old/new focus need their x1/y1/x2/y2
        // recomputed against the live radius. Without this we'd wait
        // for the next sim tick.
        applyPositions();
        if (anchorFocusToCentre) recentreOnFocus();
    }

    function setAnchorFocusToCentre(on: boolean): void {
        anchorFocusToCentre = on;
        if (on) recentreOnFocus();
    }

    function setHopLimit(limit: number | null): void {
        if (limit === hopLimit) return;
        hopLimit = limit;
        paintNodes();
        paintLinks();
    }

    function setTypeFilter(types: string[] | null): void {
        typeFilter = types ? new Set(types) : null;
        paintNodes();
        paintLinks();
    }

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

    function dispose(): void {
        resizeObserver.disconnect();
        host.innerHTML = "";
    }

    return {
        setData,
        setPositions,
        setFocus,
        setHopLimit,
        setArrowSize,
        setTypeFilter,
        setAnchorFocusToCentre,
        dispose,
    };
}
