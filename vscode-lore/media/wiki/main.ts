import { el } from "./dom.ts";
import { renderEntityPage, type EntityDetails } from "./entity-page.ts";
import { renderTypePage, type TypeDetails } from "./type-page.ts";
import { renderHomePage } from "./home-page.ts";
import { mountToolbar, type BreadcrumbPage } from "./toolbar.ts";
import { createSearchController, type Catalog } from "./search.ts";
import { mountEntityGraph, type EntityGraphController } from "./entity-graph.ts";
import type { GraphPayload } from "../shared/graph-view.ts";
import type { PageCtx } from "./ctx.ts";

interface VsCodeApi {
  postMessage(msg: unknown): void;
  getState<T = unknown>(): T | undefined;
  setState<T>(state: T): void;
}
declare function acquireVsCodeApi(): VsCodeApi;

interface Page {
  kind: "entity" | "type" | "home";
  value: string;
}

interface PageMessage {
  type: "page";
  page: Page | null | undefined;
  payload: EntityDetails | TypeDetails | null | undefined;
  catalog?: Catalog;
  palette?: string[];
  breadcrumbs?: BreadcrumbPage[];
  cursor?: number;
  canBack?: boolean;
  canForward?: boolean;
  graph?: GraphPayload | null;
  scrollToGraph?: boolean;
}
interface ErrorMessage {
  type: "error";
  message?: string;
}
interface InfoMessage {
  type: "info";
  message?: string;
}
type WebviewMessage = PageMessage | ErrorMessage | InfoMessage;

interface PersistedState {
  collapsed?: string[];
  activeTab?: "state" | "history";
  scrollByPage?: Record<string, number>;
  lastPageKey?: string | null;
  // Mirrors the extension's navigation history so the panel serializer can
  // restore the last page after an editor restart. Each entry includes
  // source so cross-project lookups land in the right project.
  history?: BreadcrumbPage[];
  cursor?: number;
}

const vscode = acquireVsCodeApi();
const root = document.getElementById("root")!;
const toolbarHost = document.getElementById("toolbar")!;
const graphSection = document.getElementById("entity-graph")!;

const persisted = (vscode.getState<PersistedState>() ?? {}) as PersistedState;
const collapsed = new Set<string>(persisted.collapsed ?? []);
let activeTab: "state" | "history" = persisted.activeTab ?? "state";
const scrollByPage: Record<string, number> = persisted.scrollByPage ?? {};
let lastPageKey: string | null = persisted.lastPageKey ?? null;
let palette: string[] = [];
let lastHistory: BreadcrumbPage[] = persisted.history ?? [];
let lastCursor: number = persisted.cursor ?? -1;

function saveState(): void {
  vscode.setState<PersistedState>({
    collapsed: [...collapsed],
    activeTab,
    scrollByPage,
    lastPageKey,
    history: lastHistory,
    cursor: lastCursor,
  });
}

window.addEventListener("scroll", () => {
  if (lastPageKey) {
    scrollByPage[lastPageKey] = window.scrollY;
    saveState();
  }
}, { passive: true });

const search = createSearchController({
  openEntity: (entity) => vscode.postMessage({ type: "openEntity", entity }),
  openType: (value) => vscode.postMessage({ type: "openType", value }),
});

const ctx: PageCtx = {
  get palette() { return palette; },
  navigate(uri, line, range) {
    vscode.postMessage({ type: "navigate", uri, line, range });
  },
  openEntity(entity) {
    vscode.postMessage({ type: "openEntity", entity });
  },
  openType(value) {
    vscode.postMessage({ type: "openType", value });
  },
  openHome() {
    vscode.postMessage({ type: "openHome" });
  },
  collapsed,
  onToggle: () => saveState(),
  get activeTab() { return activeTab; },
  setActiveTab(t) { activeTab = t; saveState(); },
  search,
};

const toolbar = mountToolbar(toolbarHost, {
  back: () => vscode.postMessage({ type: "back" }),
  forward: () => vscode.postMessage({ type: "forward" }),
  openHome: () => vscode.postMessage({ type: "openHome" }),
  openHistoryIndex: (index) => vscode.postMessage({ type: "goto", index }),
}, search);

// Persistent embedded graph at the bottom of the wiki view. Lives outside
// the #root that re-renders on every navigation so the graph instance —
// and its layout state — survives across page changes.
const entityGraph: EntityGraphController = mountEntityGraph(graphSection, {
  // Scroll-to-graph keeps the embedded graph in view across the navigation.
  // The flag travels through the extension's openEntity handler back into
  // the next page message as scrollToGraph: true.
  openEntity: (entity) => vscode.postMessage({ type: "openEntity", entity, scrollToGraph: true }),
  openGraphHere: () => vscode.postMessage({ type: "openGraphHere" }),
});

function pageKey(page: Page | null | undefined): string {
  if (!page) return "";
  return page.kind + ":" + page.value;
}

function render(msg: PageMessage): void {
  palette = msg.palette ?? [];
  const page = msg.page ?? null;
  const key = pageKey(page);
  const sameAsLast = key === lastPageKey;

  search.setCatalog(msg.catalog);
  lastHistory = msg.breadcrumbs ?? [];
  lastCursor = msg.cursor ?? -1;
  toolbar.setBreadcrumbs(lastHistory, lastCursor);
  toolbar.setNav(!!msg.canBack, !!msg.canForward);

  root.innerHTML = "";

  let nodes: HTMLElement[] = [];
  if (!page || page.kind === "home") {
    nodes = renderHomePage(msg.catalog, ctx);
  } else if (page.kind === "type") {
    nodes = renderTypePage(msg.payload as TypeDetails | undefined, ctx);
  } else {
    nodes = renderEntityPage(msg.payload as EntityDetails | undefined, ctx);
  }
  for (const n of nodes) {
    if (n) root.appendChild(n);
  }
  search.pruneDetached();

  // Embedded graph is entity-only — drives off the entity payload's
  // canonical name+type so resolution works even when page.value is a
  // disambiguated label. Update before scroll restoration so the graph
  // host has its real height when we measure scroll targets.
  if (page && page.kind === "entity") {
    const details = msg.payload as EntityDetails | undefined;
    entityGraph.show(msg.graph ?? null, {
      pageValue: page.value,
      name: details?.name,
      type: details?.type,
    }, palette);
  } else {
    entityGraph.hide();
  }

  lastPageKey = key;
  saveState();
  // Scroll behaviour: scrollToGraph (set when navigating from an embedded
  // graph node click) wins over saved-position restore. Otherwise fall
  // through to per-page scroll memory; first-time landings start at top.
  const targetScroll = sameAsLast ? (scrollByPage[key] ?? 0) : 0;
  if (!sameAsLast) scrollByPage[key] = 0;
  requestAnimationFrame(() => {
    if (msg.scrollToGraph) {
      entityGraph.scrollIntoView();
    } else {
      window.scrollTo(0, targetScroll);
    }
  });
}

window.addEventListener("message", (e: MessageEvent<WebviewMessage>) => {
  const msg = e.data;
  if (msg.type === "page") {
    render(msg);
  } else if (msg.type === "error") {
    root.innerHTML = "";
    root.appendChild(el("p", { class: "err" }, msg.message ?? "Unknown error"));
  } else if (msg.type === "info") {
    // No project to scope to — clear toolbar and embedded graph so the
    // page doesn't look functional.
    toolbarHost.innerHTML = "";
    entityGraph.hide();
    root.innerHTML = "";
    root.appendChild(el("div", { class: "no-project" }, msg.message ?? ""));
  }
});

vscode.postMessage({ type: "ready" });
