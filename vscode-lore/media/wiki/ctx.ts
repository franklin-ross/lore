import type { SearchController } from "./search.ts";

// PageCtx is the bag of palette + state callbacks every page renderer needs.
// Centralised so the renderer signatures stay short and the toolbar/search
// controller can mix in via composition without circular imports.
export interface PageCtx {
  readonly palette: string[];
  navigate(uri: string, line: number): void;
  openEntity(entity: string): void;
  openType(value: string): void;
  openHome(): void;
  collapsed: Set<string>;
  onToggle: (title: string, isCollapsed: boolean) => void;
  readonly activeTab: "state" | "history";
  setActiveTab(t: "state" | "history"): void;
  search: SearchController;
}
