// Flat force layout: charge + entity links + collide. No clustering,
// no centroids. Strict subset of force-hierarchical, validates that
// two layouts can coexist behind the LayoutEngine interface.

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
}

type SimLink = SimulationLinkDatum<SimNode>;

const NODE_RADIUS = 7;

const DEFAULTS = {
    linkDistance: 50,
    linkStrength: 0.1,
    chargeStrength: -150,
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
        key: "chargeStrength",
        label: "charge",
        type: "range",
        min: -3000,
        max: 0,
        step: 25,
        default: DEFAULTS.chargeStrength,
    },
];

export const forceFlatFactory: LayoutFactory = {
    id: "force-flat",
    label: "Force (flat)",
    description: "Charge + entity links. No clustering.",
    options: OPTIONS,
    create() {
        return createForceFlat();
    },
};

function createForceFlat(): LayoutEngine {
    let nodes: SimNode[] = [];
    let links: SimLink[] = [];
    let nodeById = new Map<string, SimNode>();

    let linkDistance = DEFAULTS.linkDistance;
    let linkStrength = DEFAULTS.linkStrength;
    let chargeStrength = DEFAULTS.chargeStrength;
    let pinnedCount = 0;

    let handlers: LayoutHandlers = {
        onTick: () => {},
        onCentroidsChanged: () => {},
    };

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
        .alphaDecay(0.015)
        .on("tick", emitTick);

    function emitTick(): void {
        const map = new Map<string, LayoutPosition>();
        for (const n of nodes) map.set(n.id, { x: n.x ?? 0, y: n.y ?? 0 });
        handlers.onTick(map);
    }

    function applySimulationContents(): void {
        sim.nodes(nodes);
        const entityLinks =
            sim.force<ReturnType<typeof forceLink<SimNode, SimLink>>>("link");
        if (entityLinks) entityLinks.links(links);
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

        applySimulationContents();
        // No centroids in this layout — emit empty so renderer clears any
        // prior debug rings carried over from a previous layout.
        handlers.onCentroidsChanged([]);
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
            case "chargeStrength": {
                if (typeof value !== "number" || value === chargeStrength)
                    return;
                chargeStrength = value;
                sim.force<ReturnType<typeof forceManyBody<SimNode>>>(
                    "charge",
                )?.strength(value);
                break;
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
