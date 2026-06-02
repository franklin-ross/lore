// Orchestrator: wires the SVG renderer to the active layout engine.
// Knows nothing about d3-force or paint loops — physics lives in
// layouts/<id>.ts, paint in graph-renderer.ts. This file translates wire
// payloads into renderer/engine inputs and routes drag/click events.

import {
    mountRenderer,
    type GraphRenderer,
    type RenderCentroid,
    type RenderLink,
    type RenderNode,
} from "./graph-renderer.ts";
import {
    defaultFactory,
    layoutEntries,
    loadFactory,
} from "./layouts/registry.ts";
import type {
    LayoutEngine,
    LayoutFactory,
    LayoutHandlers,
    OptionSpec,
} from "./layouts/types.ts";

export type { OptionSpec, RangeOption, ToggleOption } from "./layouts/types.ts";

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

export interface GraphRelationEdge {
    from: string;
    to: string;
    label: string;
    symmetric?: boolean;
}

export interface GraphPayload {
    nodes?: GraphNode[];
    defEdges?: GraphDefEdge[];
    relationEdges?: GraphRelationEdge[];
}

export interface GraphHandlers {
    onFocus(label: string | null): void;
    onOpenEntity(label: string): void;
}

export interface LayoutDescriptor {
    id: string;
    label: string;
    options: OptionSpec[];
}

export interface LayoutChoice {
    id: string;
    label: string;
}

export interface GraphView {
    update(payload: GraphPayload, focus: string | null | undefined): void;
    setFocus(label: string | null | undefined): void;
    // Choose which edge layers are drawn. Relations are the explicit typed
    // edges; mentions are reference-derived. Relations default on, mentions off.
    setEdgeKinds(relations: boolean, mentions: boolean): void;
    setHopLimit(limit: number | null): void;
    setArrowSize(size: number): void;
    setTypeFilter(types: string[] | null): void;
    // Pin the focused node to the host's visual centre on every tick.
    // Pan input is ignored while on; zoom stays anchored. Used by the
    // embedded wiki graph; off in the standalone graph panel.
    setAnchorFocusToCentre(on: boolean): void;
    setOption(key: string, value: number | boolean): void;
    getLayout(): LayoutDescriptor;
    getLayouts(): LayoutChoice[];
    setLayout(
        id: string,
        getOption?: (key: string) => number | boolean | undefined,
    ): Promise<void>;
    dispose(): void;
}

export function mountGraph(
    host: HTMLElement,
    handlers: GraphHandlers,
    getPalette: () => string[],
): GraphView {
    let focus: string | null = null;
    let lastNodes: RenderNode[] = [];
    let lastLinks: RenderLink[] = [];
    let lastCentroids: RenderCentroid[] = [];
    let lastPayload: GraphPayload = {};
    let showRelations = true;
    let showMentions = false;

    let factory: LayoutFactory = defaultFactory;
    let engine: LayoutEngine = factory.create();

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
            // Drag start is no-op here — the immediately-following dragMove
            // event pins the node at cursor (within DRAG_THRESHOLD pixels of
            // its current position). Engine auto-bumps energy on first pin.
            onNodeDragStart() {},
            onNodeDragMove(id, x, y) {
                engine.pinNode(id, x, y);
            },
            onNodeDragEnd(id) {
                engine.releaseNode(id);
            },
        },
        getPalette,
    );

    // Engine handlers extracted so setLayout can re-bind them on the
    // freshly-created engine. Closures capture lastNodes/lastLinks/
    // renderer from the outer scope; both stay valid across swaps.
    const engineHandlers: LayoutHandlers = {
        onTick(positions) {
            renderer.setPositions(positions);
        },
        onCentroidsChanged(centroids) {
            lastCentroids = centroids;
            renderer.setData(lastNodes, lastLinks, lastCentroids);
        },
    };

    engine.setHandlers(engineHandlers);
    engine.start();

    function buildLinks(payload: GraphPayload, ids: Set<string>): RenderLink[] {
        const rLinks: RenderLink[] = [];
        if (showRelations) {
            for (const e of payload.relationEdges ?? []) {
                if (!ids.has(e.from) || !ids.has(e.to)) continue;
                rLinks.push({
                    source: e.from,
                    target: e.to,
                    count: 1,
                    kind: "relation",
                    symmetric: e.symmetric,
                    label: e.label,
                });
            }
        }
        if (showMentions) {
            for (const e of payload.defEdges ?? []) {
                if (!ids.has(e.from) || !ids.has(e.to)) continue;
                rLinks.push({ source: e.from, target: e.to, count: e.count, kind: "mention" });
            }
        }
        return rLinks;
    }

    // applyData rebuilds the renderer/engine inputs from the last payload and
    // current edge-kind toggles. Called on both fresh payloads and toggle
    // changes so switching layers doesn't need a server round-trip.
    function applyData(): void {
        const rNodes: RenderNode[] = (lastPayload.nodes ?? []).map((n) => ({
            id: n.label,
            name: n.name,
            type: n.type,
            colourIndex: n.colourIndex,
        }));
        const ids = new Set(rNodes.map((n) => n.id));
        const rLinks = buildLinks(lastPayload, ids);

        lastNodes = rNodes;
        lastLinks = rLinks;
        // engine.setData fires onCentroidsChanged synchronously, which
        // pushes lastNodes/lastLinks (now fresh) through to the renderer
        // along with the new centroid list. Then the synchronous emitTick
        // fills positions before we return.
        engine.setData(
            rNodes.map((n) => ({ id: n.id, type: n.type })),
            rLinks.map((l) => ({
                source: l.source,
                target: l.target,
                weight: l.count,
            })),
            focus ?? undefined,
        );
        renderer.setFocus(focus);
    }

    function update(
        payload: GraphPayload,
        nextFocus: string | null | undefined,
    ): void {
        focus = nextFocus ?? focus;
        lastPayload = payload;
        applyData();
    }

    function setEdgeKinds(relations: boolean, mentions: boolean): void {
        showRelations = relations;
        showMentions = mentions;
        applyData();
    }

    function setFocus(label: string | null | undefined): void {
        focus = label ?? null;
        renderer.setFocus(focus);
    }

    function setHopLimit(limit: number | null): void {
        renderer.setHopLimit(limit);
    }

    function setTypeFilter(types: string[] | null): void {
        renderer.setTypeFilter(types);
    }

    function setArrowSize(size: number): void {
        renderer.setArrowSize(size);
    }

    function setAnchorFocusToCentre(on: boolean): void {
        renderer.setAnchorFocusToCentre(on);
    }

    function setOption(key: string, value: number | boolean): void {
        engine.setOption(key, value);
    }

    function getLayout(): LayoutDescriptor {
        return {
            id: factory.id,
            label: factory.label,
            options: factory.options,
        };
    }

    function getLayouts(): LayoutChoice[] {
        return layoutEntries.map((e) => ({ id: e.id, label: e.label }));
    }

    // setLayout swaps the active engine. Non-default factories are
    // loaded via dynamic import, so this is async — the chunk lands
    // on first switch then is cached for the session. Positions reset
    // (new engine starts from scratch) but data, focus, and renderer
    // state carry across. Per-option values come from getOption(key)
    // when provided, otherwise the new engine's defaults apply.
    async function setLayout(
        id: string,
        getOption?: (key: string) => number | boolean | undefined,
    ): Promise<void> {
        if (id === factory.id) return;
        const next = await loadFactory(id);
        if (!next || next === factory) return;
        engine.stop();
        engine.dispose();
        factory = next;
        engine = factory.create();
        engine.setHandlers(engineHandlers);
        for (const opt of factory.options) {
            const stored = getOption?.(opt.key);
            const value = stored !== undefined ? stored : opt.default;
            engine.setOption(opt.key, value);
        }
        engine.setData(
            lastNodes.map((n) => ({ id: n.id, type: n.type })),
            lastLinks.map((l) => ({
                source: l.source,
                target: l.target,
                weight: l.count,
            })),
            focus ?? undefined,
        );
        engine.start();
    }

    function dispose(): void {
        engine.dispose();
        renderer.dispose();
    }

    return {
        update,
        setFocus,
        setEdgeKinds,
        setHopLimit,
        setArrowSize,
        setTypeFilter,
        setAnchorFocusToCentre,
        setOption,
        getLayout,
        getLayouts,
        setLayout,
        dispose,
    };
}
