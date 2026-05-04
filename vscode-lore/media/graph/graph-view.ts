import {
  forceCenter,
  forceCollide,
  forceLink,
  forceManyBody,
  forceSimulation,
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
  onFocus(label: string): void;
  onOpenEntity(label: string): void;
}

interface SimNode extends SimulationNodeDatum {
  id: string;
  name: string;
  type?: string;
  colourIndex: number;
  hop: number;
}

interface SimLink extends SimulationLinkDatum<SimNode> {
  count: number;
}

export interface GraphView {
  update(payload: GraphPayload, focus: string | null | undefined): void;
  setFocus(label: string | null | undefined): void;
  setHopLimit(limit: number | null): void;
  setArrowSize(size: number): void;
  dispose(): void;
}

const NODE_RADIUS = 7;
const FOCUS_RADIUS = 10;
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
export function mountGraph(host: HTMLElement, handlers: GraphHandlers, getPalette: () => string[]): GraphView {
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

  const viewport = document.createElementNS("http://www.w3.org/2000/svg", "g");
  viewport.classList.add("graph-viewport");
  svg.appendChild(viewport);

  const linkLayer = document.createElementNS("http://www.w3.org/2000/svg", "g");
  linkLayer.classList.add("graph-links");
  viewport.appendChild(linkLayer);
  const nodeLayer = document.createElementNS("http://www.w3.org/2000/svg", "g");
  nodeLayer.classList.add("graph-nodes");
  viewport.appendChild(nodeLayer);

  let nodes: SimNode[] = [];
  let links: SimLink[] = [];
  let nodeById = new Map<string, SimNode>();
  let nodeEls = new Map<string, { circle: SVGCircleElement; label: SVGTextElement }>();
  let linkEls = new Map<string, {
    line: SVGLineElement;
    gradient: SVGLinearGradientElement;
    stopFrom: SVGStopElement;
    stopTo: SVGStopElement;
  }>();
  let focus: string | null = null;
  let hoveredId: string | null = null;
  let hopLimit: number | null = null;
  let arrowSize = 0;
  let palette: string[] = getPalette();
  let gradientCounter = 0;

  const transform = { x: 0, y: 0, k: 1 };

  function applyTransform(): void {
    viewport.setAttribute("transform", `translate(${transform.x},${transform.y}) scale(${transform.k})`);
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
      const marker = document.createElementNS("http://www.w3.org/2000/svg", "marker");
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
      const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
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

  const simulation: Simulation<SimNode, SimLink> = forceSimulation<SimNode, SimLink>([])
    .force("link", forceLink<SimNode, SimLink>([])
      .id((d) => d.id)
      .distance(90)
      .strength(0.6))
    .force("charge", forceManyBody<SimNode>().strength(-400))
    .force("collide", forceCollide<SimNode>().radius(NODE_RADIUS + 14))
    .force("center", forceCenter(0, 0))
    .alphaDecay(0.04)
    .on("tick", drawTick);

  function linkKey(l: SimLink): string {
    const s = typeof l.source === "object" ? (l.source as SimNode).id : String(l.source);
    const t = typeof l.target === "object" ? (l.target as SimNode).id : String(l.target);
    return `${s}→${t}`;
  }

  function diffNodes(next: GraphNode[]): { kept: SimNode[]; added: SimNode[] } {
    const seen = new Set<string>();
    const kept: SimNode[] = [];
    const added: SimNode[] = [];
    const seedX = focus && nodeById.has(focus) ? nodeById.get(focus)!.x ?? 0 : 0;
    const seedY = focus && nodeById.has(focus) ? nodeById.get(focus)!.y ?? 0 : 0;
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

  function buildLinks(payload: GraphPayload, ids: Map<string, SimNode>): SimLink[] {
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
  // (subject to label gating) its label. The hop limit is a hard cutoff —
  // nodes past it return 0 so they vanish from view but stay in the
  // simulation, anchoring positions for in-scope neighbours.
  function nodeVisibilityOpacity(n: SimNode): number {
    if (focus === n.id) return 1;
    if (hopLimit !== null && n.hop > hopLimit) return 0;
    return HOP_FADE[Math.min(n.hop, HOP_FADE.length - 1)] ?? 0.05;
  }

  // labelOpacityFor decides whether to show a node's label at all. Once
  // visible labels paint at full opacity regardless of node fade — fading
  // the names alongside their dots makes hop-distant labels unreadable.
  // Focus distinction goes to font weight (see paintNodes), not opacity.
  function labelOpacityFor(n: SimNode, nodeOpacity: number): number {
    if (nodeOpacity === 0) return 0;
    if (hoveredId === n.id) return 1;
    const isFocused = focus === n.id;
    if (focus === null || isFocused || n.hop <= LABEL_HOP_LIMIT) return 1;
    return 0;
  }

  function paintNodes(): void {
    for (const n of nodes) {
      let els = nodeEls.get(n.id);
      if (!els) {
        const circle = document.createElementNS("http://www.w3.org/2000/svg", "circle");
        circle.classList.add("graph-node");
        const label = document.createElementNS("http://www.w3.org/2000/svg", "text");
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
      els.circle.setAttribute("r", String(isFocused ? FOCUS_RADIUS : NODE_RADIUS));
      const nodeOpacity = nodeVisibilityOpacity(n);
      els.circle.setAttribute("opacity", String(nodeOpacity));
      // Hidden nodes shouldn't catch clicks or steal focus — they're still
      // there for the simulation, but the user can't interact with them.
      els.circle.style.pointerEvents = nodeOpacity === 0 ? "none" : "auto";
      els.label.setAttribute("opacity", String(labelOpacityFor(n, nodeOpacity)));
      els.circle.classList.toggle("focused", isFocused);
      els.label.classList.toggle("focused", isFocused);
    }
  }

  // repaintLabel updates one node's label opacity in place. Used by the
  // hover handlers so we don't repaint every node on a pointerenter.
  function repaintLabel(n: SimNode): void {
    const els = nodeEls.get(n.id);
    if (!els) return;
    els.label.setAttribute("opacity", String(labelOpacityFor(n, nodeVisibilityOpacity(n))));
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
        const gradient = document.createElementNS("http://www.w3.org/2000/svg", "linearGradient");
        gradient.setAttribute("id", gradientId);
        gradient.setAttribute("gradientUnits", "userSpaceOnUse");
        const stopFrom = document.createElementNS("http://www.w3.org/2000/svg", "stop");
        stopFrom.setAttribute("offset", "0");
        const stopTo = document.createElementNS("http://www.w3.org/2000/svg", "stop");
        stopTo.setAttribute("offset", "1");
        gradient.appendChild(stopFrom);
        gradient.appendChild(stopTo);
        defs.appendChild(gradient);

        const line = document.createElementNS("http://www.w3.org/2000/svg", "line");
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
      bundle.stopFrom.setAttribute("stop-opacity", String(opS * sFade * 0.7));
      bundle.stopTo.setAttribute("stop-opacity", String(opT * tFade * 0.7));

      const visible = opS > 0 || opT > 0;
      bundle.line.style.display = visible ? "" : "none";

      // Markers are opt-in (arrowSize === 0 disables them entirely) and
      // hidden when the target end is faded out — the marker is solid
      // colour and would otherwise float past the cutoff.
      if (arrowSize > 0 && opT > 0) {
        bundle.line.setAttribute("marker-end", `url(#arrow-${s.colourIndex})`);
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
  }

  function attachNodeHandlers(el: SVGElement, node: SimNode): void {
    let dragging = false;
    let movedDuringDrag = false;
    let pointerId = -1;
    el.addEventListener("pointerdown", (ev) => {
      ev.stopPropagation();
      dragging = true;
      movedDuringDrag = false;
      pointerId = ev.pointerId;
      el.setPointerCapture(pointerId);
      simulation.alphaTarget(0.3).restart();
      node.fx = node.x;
      node.fy = node.y;
    });
    el.addEventListener("pointermove", (ev) => {
      if (!dragging) return;
      movedDuringDrag = true;
      const pt = clientToWorld(ev.clientX, ev.clientY);
      node.fx = pt.x;
      node.fy = pt.y;
    });
    el.addEventListener("pointerup", () => {
      if (!dragging) return;
      dragging = false;
      el.releasePointerCapture(pointerId);
      simulation.alphaTarget(0);
      node.fx = null;
      node.fy = null;
      if (!movedDuringDrag) {
        // Treat as click — fire focus.
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

  // Pan with empty-space drag.
  let panActive = false;
  let panLast = { x: 0, y: 0 };
  svg.addEventListener("pointerdown", (ev) => {
    if (ev.target !== svg && ev.target !== viewport && ev.target !== linkLayer && ev.target !== nodeLayer) return;
    panActive = true;
    panLast = { x: ev.clientX, y: ev.clientY };
    svg.setPointerCapture(ev.pointerId);
  });
  svg.addEventListener("pointermove", (ev) => {
    if (!panActive) return;
    transform.x += ev.clientX - panLast.x;
    transform.y += ev.clientY - panLast.y;
    panLast = { x: ev.clientX, y: ev.clientY };
    applyTransform();
  });
  svg.addEventListener("pointerup", (ev) => {
    if (!panActive) return;
    panActive = false;
    svg.releasePointerCapture(ev.pointerId);
  });

  // Zoom with wheel — zoom around cursor so the spot under the pointer
  // stays put.
  svg.addEventListener("wheel", (ev) => {
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
  }, { passive: false });

  // Centre the simulation in the viewport once layout is known.
  const resizeObserver = new ResizeObserver(() => {
    const rect = host.getBoundingClientRect();
    svg.setAttribute("width", String(rect.width));
    svg.setAttribute("height", String(rect.height));
    svg.setAttribute("viewBox", `${-rect.width / 2} ${-rect.height / 2} ${rect.width} ${rect.height}`);
  });
  resizeObserver.observe(host);

  function update(payload: GraphPayload, nextFocus: string | null | undefined): void {
    palette = getPalette();
    ensureMarkers();
    focus = nextFocus ?? focus;

    const { kept, added } = diffNodes(payload.nodes ?? []);
    nodes = [...kept, ...added];
    nodeById = new Map(nodes.map((n) => [n.id, n]));
    links = buildLinks(payload, nodeById);

    simulation.nodes(nodes);
    const linkForce = simulation.force<ReturnType<typeof forceLink<SimNode, SimLink>>>("link");
    if (linkForce) linkForce.links(links);

    recomputeHops();
    paintNodes();
    paintLinks();

    simulation.alpha(Math.max(simulation.alpha(), 0.15)).restart();
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

  function dispose(): void {
    simulation.stop();
    resizeObserver.disconnect();
    host.innerHTML = "";
  }

  return { update, setFocus, setHopLimit, setArrowSize, dispose };
}
