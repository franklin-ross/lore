// ForceAtlas2 layout via graphology + graphology-layout-forceatlas2.
// Runs synchronous iteration batches per animation frame so the layout
// resolves smoothly without blocking input. Pinned nodes are restored
// after each iterate (FA2 has no native pinning).

import Graph, { UndirectedGraph } from "graphology";
import forceAtlas2, {
    type ForceAtlas2Settings,
} from "graphology-layout-forceatlas2";

import type {
    LayoutEngine,
    LayoutFactory,
    LayoutHandlers,
    LayoutLink,
    LayoutNode,
    LayoutPosition,
    OptionSpec,
} from "./types.ts";

const DEFAULTS = {
    gravity: 1,
    // 10 (the FA2 paper default) collapses TTRPG-sized graphs into a
    // tight blob where labels overlap heavily. 50 matches Gephi's
    // "expanded" preset and reads well at ~100–500 nodes.
    scalingRatio: 50,
    edgeWeightInfluence: 1,
    slowDown: 5,
    // lin-log replaces linear attraction d with log(1+d). It compresses
    // distance so communities separate cleanly — but the same compression
    // dampens the visible effect of edgeWeightInfluence; turn lin-log
    // off if you want the weight slider to bite.
    linLogMode: false,
    strongGravityMode: false,
    // a.k.a. "dissuade hubs" in Gephi. Divides each edge's attraction
    // by the source node's degree, so hubs stay put while their leaves
    // still cluster around them. Without this, every hub gets pulled
    // toward every connected leaf and the centre crushes.
    outboundAttractionDistribution: true,
};

const OPTIONS: OptionSpec[] = [
    {
        key: "gravity",
        label: "gravity",
        type: "range",
        min: 0,
        max: 20,
        step: 0.5,
        decimals: 1,
        default: DEFAULTS.gravity,
    },
    {
        key: "scalingRatio",
        label: "scaling",
        type: "range",
        min: 1,
        max: 100,
        step: 1,
        default: DEFAULTS.scalingRatio,
    },
    {
        key: "edgeWeightInfluence",
        label: "edge influence",
        type: "range",
        min: 0,
        max: 4,
        step: 0.1,
        decimals: 2,
        default: DEFAULTS.edgeWeightInfluence,
    },
    {
        key: "slowDown",
        label: "slow down",
        type: "range",
        min: 0.1,
        max: 10,
        step: 0.1,
        decimals: 2,
        default: DEFAULTS.slowDown,
    },
    {
        key: "linLogMode",
        label: "lin-log",
        type: "toggle",
        default: DEFAULTS.linLogMode,
    },
    {
        key: "strongGravityMode",
        label: "strong gravity",
        type: "toggle",
        default: DEFAULTS.strongGravityMode,
    },
    {
        key: "outboundAttractionDistribution",
        label: "dissuade hubs",
        type: "toggle",
        default: DEFAULTS.outboundAttractionDistribution,
    },
];

export const forceAtlas2Factory: LayoutFactory = {
    id: "force-atlas2",
    label: "ForceAtlas2",
    description:
        "ForceAtlas2 layout: lin-log/Barnes-Hut force model from Gephi.",
    options: OPTIONS,
    create() {
        return createForceAtlas2();
    },
};

function createForceAtlas2(): LayoutEngine {
    let graph: Graph = new UndirectedGraph();
    const pinned = new Map<string, { x: number; y: number }>();
    let iterationsRemaining = 0;
    let rafId: number | null = null;

    const settings: ForceAtlas2Settings = {
        gravity: DEFAULTS.gravity,
        scalingRatio: DEFAULTS.scalingRatio,
        edgeWeightInfluence: DEFAULTS.edgeWeightInfluence,
        slowDown: DEFAULTS.slowDown,
        linLogMode: DEFAULTS.linLogMode,
        strongGravityMode: DEFAULTS.strongGravityMode,
        outboundAttractionDistribution:
            DEFAULTS.outboundAttractionDistribution,
        // Barnes-Hut keeps cost ~O(N log N) at hub-and-spoke scale.
        barnesHutOptimize: true,
        barnesHutTheta: 0.5,
    };

    let handlers: LayoutHandlers = {
        onTick: () => {},
        onCentroidsChanged: () => {},
    };

    function emitTick(): void {
        const map = new Map<string, LayoutPosition>();
        graph.forEachNode((id, attrs) => {
            map.set(id, {
                x: (attrs as { x: number }).x,
                y: (attrs as { y: number }).y,
            });
        });
        handlers.onTick(map);
    }

    function scheduleTick(): void {
        if (rafId !== null) return;
        rafId = requestAnimationFrame(runTick);
    }

    function runTick(): void {
        rafId = null;
        if (graph.order === 0) return;
        if (iterationsRemaining <= 0 && pinned.size === 0) return;
        forceAtlas2.assign(graph, { iterations: 1, settings });
        // Restore pins after iterate — FA2 has no per-node fixing.
        for (const [id, p] of pinned) {
            if (graph.hasNode(id)) {
                graph.setNodeAttribute(id, "x", p.x);
                graph.setNodeAttribute(id, "y", p.y);
            }
        }
        if (iterationsRemaining > 0) iterationsRemaining -= 1;
        emitTick();
        if (iterationsRemaining > 0 || pinned.size > 0) scheduleTick();
    }

    function setData(
        nextNodes: LayoutNode[],
        nextLinks: LayoutLink[],
        focusHint?: string,
    ): void {
        const seedX =
            focusHint && graph.hasNode(focusHint)
                ? (graph.getNodeAttribute(focusHint, "x") as number)
                : 0;
        const seedY =
            focusHint && graph.hasNode(focusHint)
                ? (graph.getNodeAttribute(focusHint, "y") as number)
                : 0;

        const seen = new Set<string>();
        for (const n of nextNodes) {
            seen.add(n.id);
            if (graph.hasNode(n.id)) {
                graph.setNodeAttribute(n.id, "type", n.type);
            } else {
                graph.addNode(n.id, {
                    type: n.type,
                    x: seedX + (Math.random() - 0.5) * 40,
                    y: seedY + (Math.random() - 0.5) * 40,
                });
            }
        }
        for (const id of [...graph.nodes()]) {
            if (!seen.has(id)) graph.dropNode(id);
        }

        // Edges are simpler to rebuild than diff — this stays cheap at
        // ~500 nodes and avoids stale parallel-edge tracking.
        graph.clearEdges();
        for (const l of nextLinks) {
            if (l.source === l.target) continue;
            if (!graph.hasNode(l.source) || !graph.hasNode(l.target)) continue;
            if (graph.hasEdge(l.source, l.target)) continue;
            // Pass the reference count as the edge weight so FA2's
            // edgeWeightInfluence slider has something to scale.
            graph.addEdge(l.source, l.target, { weight: l.weight ?? 1 });
        }

        for (const id of [...pinned.keys()]) {
            if (!graph.hasNode(id)) pinned.delete(id);
        }

        handlers.onCentroidsChanged([]);
        iterationsRemaining = 500;
        emitTick();
        scheduleTick();
    }

    function setOption(key: string, value: number | boolean): void {
        switch (key) {
            case "gravity":
                if (typeof value !== "number") return;
                settings.gravity = value;
                break;
            case "scalingRatio":
                if (typeof value !== "number") return;
                settings.scalingRatio = value;
                break;
            case "edgeWeightInfluence":
                if (typeof value !== "number") return;
                settings.edgeWeightInfluence = value;
                break;
            case "slowDown":
                if (typeof value !== "number") return;
                settings.slowDown = value;
                break;
            case "linLogMode":
                if (typeof value !== "boolean") return;
                settings.linLogMode = value;
                break;
            case "strongGravityMode":
                if (typeof value !== "boolean") return;
                settings.strongGravityMode = value;
                break;
            case "outboundAttractionDistribution":
                if (typeof value !== "boolean") return;
                settings.outboundAttractionDistribution = value;
                break;
            default:
                return;
        }
        iterationsRemaining = Math.max(iterationsRemaining, 200);
        scheduleTick();
    }

    function setHandlers(h: LayoutHandlers): void {
        handlers = h;
    }

    function pinNode(id: string, x: number, y: number): void {
        if (!graph.hasNode(id)) return;
        pinned.set(id, { x, y });
        graph.setNodeAttribute(id, "x", x);
        graph.setNodeAttribute(id, "y", y);
        iterationsRemaining = Math.max(iterationsRemaining, 100);
        scheduleTick();
    }

    function releaseNode(id: string): void {
        pinned.delete(id);
    }

    function bumpEnergy(level: number = 0.4): void {
        iterationsRemaining = Math.max(
            iterationsRemaining,
            Math.floor(level * 500),
        );
        scheduleTick();
    }

    function start(): void {
        iterationsRemaining = Math.max(iterationsRemaining, 100);
        scheduleTick();
    }

    function stop(): void {
        if (rafId !== null) {
            cancelAnimationFrame(rafId);
            rafId = null;
        }
    }

    function dispose(): void {
        stop();
        graph.clear();
        pinned.clear();
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
