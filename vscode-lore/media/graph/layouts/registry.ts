// Registry of available layout engines. The default layout
// (force-hierarchical) is bundled eagerly so the graph paints on
// first open without an extra fetch; everything else lazy-loads via
// dynamic import() so esbuild splits it into a separate chunk that's
// only fetched when the user actually switches to that layout.

import { forceHierarchicalFactory } from "./force-hierarchical.ts";
import type { LayoutFactory } from "./types.ts";

interface LayoutEntry {
    id: string;
    label: string;
    load: () => Promise<LayoutFactory>;
}

export const layoutEntries: LayoutEntry[] = [
    {
        id: forceHierarchicalFactory.id,
        label: forceHierarchicalFactory.label,
        load: () => Promise.resolve(forceHierarchicalFactory),
    },
    {
        id: "force-flat",
        label: "Force (flat)",
        load: () =>
            import("./force-flat.ts").then((m) => m.forceFlatFactory),
    },
    {
        id: "force-atlas2",
        label: "ForceAtlas2",
        load: () =>
            import("./force-atlas2.ts").then((m) => m.forceAtlas2Factory),
    },
];

export async function loadFactory(
    id: string,
): Promise<LayoutFactory | undefined> {
    const entry = layoutEntries.find((e) => e.id === id);
    if (!entry) return undefined;
    return entry.load();
}

// Default factory is bundled eagerly so the graph view can paint on
// first open without waiting on a chunk fetch.
export const defaultFactory: LayoutFactory = forceHierarchicalFactory;
