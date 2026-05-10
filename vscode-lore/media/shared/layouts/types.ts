// Layout-engine contract. Engines own simulation/algorithm state; the
// orchestrator (graph-view.ts) wires an engine instance to the renderer.
// Each engine self-describes its tunable options so a settings UI can
// render sliders/toggles without per-layout hardcoding.

import type { RenderCentroid } from "../graph-renderer.ts";

export interface LayoutPosition {
    x: number;
    y: number;
}

export interface LayoutNode {
    id: string;
    type?: string;
}

export interface LayoutLink {
    source: string;
    target: string;
    // Reference count between source and target. Layouts may use it as
    // an edge weight (FA2's edgeWeightInfluence reads this); layouts
    // that don't care can ignore it.
    weight?: number;
}

export interface LayoutHandlers {
    onTick(positions: Map<string, LayoutPosition>): void;
    onCentroidsChanged(centroids: RenderCentroid[]): void;
    onSettled?(): void;
}

export interface LayoutEngine {
    setData(
        nodes: LayoutNode[],
        links: LayoutLink[],
        focusHint?: string,
    ): void;
    setOption(key: string, value: number | boolean): void;
    setHandlers(handlers: LayoutHandlers): void;
    pinNode(id: string, x: number, y: number): void;
    releaseNode(id: string): void;
    bumpEnergy(level?: number): void;
    start(): void;
    stop(): void;
    dispose(): void;
}

export interface RangeOption {
    key: string;
    label: string;
    type: "range";
    min: number;
    max: number;
    step: number;
    decimals?: number;
    default: number;
}

export interface ToggleOption {
    key: string;
    label: string;
    type: "toggle";
    default: boolean;
}

export type OptionSpec = RangeOption | ToggleOption;

export interface LayoutFactory {
    id: string;
    label: string;
    description?: string;
    options: OptionSpec[];
    create(): LayoutEngine;
}
