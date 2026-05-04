// Registry of available layout engines. Add new factories here, so
// they appear in the toolbar's layout dropdown automatically.

import { forceFlatFactory } from "./force-flat.ts";
import { forceHierarchicalFactory } from "./force-hierarchical.ts";
import type { LayoutFactory } from "./types.ts";

export const layoutFactories: LayoutFactory[] = [
    forceHierarchicalFactory,
    forceFlatFactory,
];

export function findLayout(id: string): LayoutFactory | undefined {
    return layoutFactories.find((f) => f.id === id);
}
