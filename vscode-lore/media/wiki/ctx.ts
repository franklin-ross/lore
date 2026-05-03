import type { SearchController } from "./search.ts";
import type { LspLocation } from "./dom.ts";

// PageCtx is the bag of palette + state callbacks every page renderer needs.
// Centralised so the renderer signatures stay short and the toolbar/search
// controller can mix in via composition without circular imports.
export interface PageCtx {
  readonly palette: string[];
  // navigate jumps to a source location. `line` is always passed as a
  // fallback the extension can fall back to when no precise range is
  // supplied; `range` is an optional sub-line span the extension uses to
  // select exactly the entity name rather than the whole line.
  navigate(uri: string, line: number, range?: LspLocation["range"]): void;
  openEntity(entity: string): void;
  openType(value: string): void;
  openHome(): void;
  collapsed: Set<string>;
  onToggle: (title: string, isCollapsed: boolean) => void;
  readonly activeTab: "state" | "history";
  setActiveTab(t: "state" | "history"): void;
  search: SearchController;
}
