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
import { forceHierarchicalFactory } from "./layouts/force-hierarchical.ts";
import type {
    LayoutEngine,
    LayoutFactory,
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

export interface GraphPayload {
    nodes?: GraphNode[];
    defEdges?: GraphDefEdge[];
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

export interface GraphView {
    update(payload: GraphPayload, focus: string | null | undefined): void;
    setFocus(label: string | null | undefined): void;
    setHopLimit(limit: number | null): void;
    setArrowSize(size: number): void;
    setTypeFilter(types: string[] | null): void;
    setOption(key: string, value: number | boolean): void;
    getLayout(): LayoutDescriptor;
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

    const factory: LayoutFactory = forceHierarchicalFactory;
    const engine: LayoutEngine = factory.create();

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

    engine.setHandlers({
        onTick(positions) {
            renderer.setPositions(positions);
        },
        onCentroidsChanged(centroids) {
            lastCentroids = centroids;
            renderer.setData(lastNodes, lastLinks, lastCentroids);
        },
    });
    engine.start();

    function update(
        payload: GraphPayload,
        nextFocus: string | null | undefined,
    ): void {
        focus = nextFocus ?? focus;

        const rNodes: RenderNode[] = (payload.nodes ?? []).map((n) => ({
            id: n.label,
            name: n.name,
            type: n.type,
            colourIndex: n.colourIndex,
        }));
        const ids = new Set(rNodes.map((n) => n.id));
        const rLinks: RenderLink[] = [];
        for (const e of payload.defEdges ?? []) {
            if (!ids.has(e.from) || !ids.has(e.to)) continue;
            rLinks.push({ source: e.from, target: e.to, count: e.count });
        }

        lastNodes = rNodes;
        lastLinks = rLinks;
        // engine.setData fires onCentroidsChanged synchronously, which
        // pushes lastNodes/lastLinks (now fresh) through to the renderer
        // along with the new centroid list. Then the synchronous emitTick
        // fills positions before we return.
        engine.setData(
            rNodes.map((n) => ({ id: n.id, type: n.type })),
            rLinks.map((l) => ({ source: l.source, target: l.target })),
            focus ?? undefined,
        );
        renderer.setFocus(focus);
    }

    function setFocus(label: string | null | undefined): void {
        focus = label ?? null;
        renderer.setFocus(focus);
        engine.bumpEnergy(0.05);
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

    function dispose(): void {
        engine.dispose();
        renderer.dispose();
    }

    return {
        update,
        setFocus,
        setHopLimit,
        setArrowSize,
        setTypeFilter,
        setOption,
        getLayout,
        dispose,
    };
}
