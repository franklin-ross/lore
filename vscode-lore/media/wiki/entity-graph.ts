// Embedded force-graph at the bottom of the wiki entity page. Reuses the
// shared mountGraph implementation that the standalone graph panel runs,
// but locked to a depth-1 neighbourhood around the current focus and with
// the next hop kept visible-but-faded via setHopLimit so shadow connections
// stay readable.
//
// The host element lives outside the wiki #root that re-renders on every
// page navigation. That's deliberate — the graph instance persists across
// page changes and we just call view.update() with the new focus's
// neighbourhood. Disposing+remounting per render would lose layout state
// and trigger another full physics warm-up.

import { mountGraph, type GraphPayload, type GraphView } from "../shared/graph-view.ts";

interface OpenGraphHandlers {
  openEntity(entity: string): void;
  openGraphHere(): void;
}

export interface FocusHint {
  // The page key the wiki uses for the entity. May be a bare name or a
  // disambiguated "Name (type)" string depending on the navigation source.
  pageValue: string;
  // Canonical bare name from EntityDetails — matches GraphNode.name.
  name: string | undefined;
  // Type (also from EntityDetails) — used to break ties when multiple
  // graph nodes share a bare name.
  type: string | undefined;
}

export interface EntityGraphController {
  show(payload: GraphPayload | null | undefined, focus: FocusHint, palette: string[]): void;
  hide(): void;
  scrollIntoView(): void;
}

// mountEntityGraph builds the persistent header + host + button scaffold
// inside `section`, then mounts the graph view once. Subsequent show()
// calls reuse the same GraphView and just push a new (filtered) payload.
export function mountEntityGraph(
  section: HTMLElement,
  handlers: OpenGraphHandlers,
): EntityGraphController {
  section.innerHTML = "";
  const header = document.createElement("div");
  header.className = "entity-graph-header";
  const title = document.createElement("h2");
  title.textContent = "Graph";
  header.appendChild(title);
  const openBtn = document.createElement("button");
  openBtn.type = "button";
  openBtn.className = "open-graph-btn";
  openBtn.textContent = "Open in graph view";
  openBtn.title = "Open the full graph focused on this entity";
  openBtn.addEventListener("click", () => handlers.openGraphHere());
  header.appendChild(openBtn);
  section.appendChild(header);

  const host = document.createElement("div");
  host.className = "entity-graph-host";
  section.appendChild(host);

  let palette: string[] = [];
  let view: GraphView | null = null;
  let empty: HTMLElement | null = null;

  function ensureView(): GraphView {
    if (view) return view;
    view = mountGraph(host, {
      // Single-click navigates here — different from the standalone graph
      // panel where single-click selects and double-click opens. Embedded
      // in the wiki the focus already follows whatever entity is on screen,
      // so a separate "select" gesture has nothing to do.
      onFocus(label) {
        if (label) handlers.openEntity(label);
      },
      onOpenEntity(label) {
        handlers.openEntity(label);
      },
    }, () => palette);
    // Fade rather than hide so depth-2 shadow connections stay visible.
    view.setHopLimit(1);
    // Lock the focused entity to the centre of the embedded view — the
    // user expects the page they're reading to anchor the graph, with
    // neighbours arranged around it. Disables pan; zoom stays.
    view.setAnchorFocusToCentre(true);
    return view;
  }

  function setEmpty(message: string | null): void {
    if (message) {
      if (!empty) {
        empty = document.createElement("p");
        empty.className = "entity-graph-empty";
        host.appendChild(empty);
      }
      empty.textContent = message;
    } else if (empty) {
      empty.remove();
      empty = null;
    }
  }

  return {
    show(payload, focus, nextPalette) {
      section.hidden = false;
      palette = nextPalette;

      const resolved = resolveFocusLabel(payload ?? null, focus);
      if (!resolved) {
        ensureView().update({ nodes: [], defEdges: [] }, null);
        setEmpty("Entity is not in the graph.");
        return;
      }
      const filtered = filterToNeighbourhood(payload!, resolved);
      if (filtered.nodes!.length <= 1) {
        ensureView().update(filtered, resolved);
        setEmpty("No connections to graph from this entity.");
        return;
      }
      setEmpty(null);
      ensureView().update(filtered, resolved);
    },
    hide() {
      section.hidden = true;
    },
    scrollIntoView() {
      // Centre the graph in the viewport — alignToBottom would push it
      // hard against the bottom edge, which feels worse on short pages.
      section.scrollIntoView({ block: "center", behavior: "auto" });
    },
  };
}

// resolveFocusLabel maps the wiki's idea of the focus (page key, bare
// name, type) to the actual graph node id. Pages can arrive at the wiki
// via multiple paths — search box (bare name), F12-at-cursor (lookup),
// graph-panel click (already-disambiguated label) — so the same entity
// may surface under different strings. We try strict matches first then
// fall back so an unambiguous entity still resolves even if disambiguation
// strings drift between server and webview.
function resolveFocusLabel(
  payload: GraphPayload | null,
  focus: FocusHint,
): string | null {
  if (!payload || !payload.nodes || payload.nodes.length === 0) return null;
  const nodes = payload.nodes;
  const byLabel = nodes.find((n) => n.label === focus.pageValue);
  if (byLabel) return byLabel.label;
  if (focus.name && focus.type) {
    const exact = nodes.find((n) => n.name === focus.name && n.type === focus.type);
    if (exact) return exact.label;
  }
  if (focus.name) {
    const byName = nodes.filter((n) => n.name === focus.name);
    if (byName.length === 1) return byName[0]!.label;
    // Multiple matches by name with no type to disambiguate — bail rather
    // than guess. The "not in graph" placeholder is honest about the miss.
  }
  return null;
}

// filterToNeighbourhood returns a payload pruned to the focus entity, its
// direct neighbours (depth 1), and *their* neighbours (depth 2). The view
// fades depth-2 nodes via setHopLimit(1), so they read as "shadows" of
// connections leaving the visible neighbourhood. Edge endpoints outside
// the kept set are dropped — graph-view does the same on update() but
// this keeps the wire payload smaller.
export function filterToNeighbourhood(
  payload: GraphPayload,
  focus: string,
): GraphPayload {
  const adj = new Map<string, Set<string>>();
  for (const n of payload.nodes ?? []) adj.set(n.label, new Set());
  for (const e of payload.defEdges ?? []) {
    adj.get(e.from)?.add(e.to);
    adj.get(e.to)?.add(e.from);
  }
  // Relation edges connect entities too — without them, an entity linked
  // only by a typed relation (and never by a mention) wouldn't pull its
  // partner into the neighbourhood, and the relation edge itself would
  // never render.
  for (const e of payload.relationEdges ?? []) {
    adj.get(e.from)?.add(e.to);
    adj.get(e.to)?.add(e.from);
  }

  const keep = new Set<string>([focus]);
  const ring1 = adj.get(focus) ?? new Set();
  for (const n of ring1) keep.add(n);
  for (const n of ring1) {
    const ring2 = adj.get(n);
    if (!ring2) continue;
    for (const m of ring2) keep.add(m);
  }

  const nodes = (payload.nodes ?? []).filter((n) => keep.has(n.label));
  const defEdges = (payload.defEdges ?? []).filter((e) => keep.has(e.from) && keep.has(e.to));
  const relationEdges = (payload.relationEdges ?? []).filter((e) => keep.has(e.from) && keep.has(e.to));
  return { nodes, defEdges, relationEdges };
}
