import { el } from "./dom.ts";
import { renderEntityPage, type EntityDetails } from "./entity-page.ts";
import { renderTypePage, type TypeDetails } from "./type-page.ts";
import { renderHomePage } from "./home-page.ts";
import { mountToolbar, type BreadcrumbPage } from "./toolbar.ts";
import { createSearchController, type Catalog } from "./search.ts";
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
}
interface ErrorMessage {
  type: "error";
  message?: string;
}
type WebviewMessage = PageMessage | ErrorMessage;

interface PersistedState {
  collapsed?: string[];
  activeTab?: "state" | "history";
  scrollByPage?: Record<string, number>;
  lastPageKey?: string | null;
}

const vscode = acquireVsCodeApi();
const root = document.getElementById("root")!;
const toolbarHost = document.getElementById("toolbar")!;

const persisted = (vscode.getState<PersistedState>() ?? {}) as PersistedState;
const collapsed = new Set<string>(persisted.collapsed ?? []);
let activeTab: "state" | "history" = persisted.activeTab ?? "state";
const scrollByPage: Record<string, number> = persisted.scrollByPage ?? {};
let lastPageKey: string | null = persisted.lastPageKey ?? null;
let palette: string[] = [];

function saveState(): void {
  vscode.setState<PersistedState>({
    collapsed: [...collapsed],
    activeTab,
    scrollByPage,
    lastPageKey,
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
  navigate(uri, line) {
    vscode.postMessage({ type: "navigate", uri, line });
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
  toolbar.setBreadcrumbs(msg.breadcrumbs ?? [], msg.cursor ?? -1);
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

  lastPageKey = key;
  saveState();
  // Restore scroll for this page only when we're not landing on it for
  // the first time. requestAnimationFrame so the new DOM has settled.
  const targetScroll = sameAsLast ? (scrollByPage[key] ?? 0) : 0;
  if (!sameAsLast) scrollByPage[key] = 0;
  requestAnimationFrame(() => {
    window.scrollTo(0, targetScroll);
  });
}

window.addEventListener("message", (e: MessageEvent<WebviewMessage>) => {
  const msg = e.data;
  if (msg.type === "page") {
    render(msg);
  } else if (msg.type === "error") {
    root.innerHTML = "";
    root.appendChild(el("p", { class: "err" }, msg.message ?? "Unknown error"));
  }
});

vscode.postMessage({ type: "ready" });
