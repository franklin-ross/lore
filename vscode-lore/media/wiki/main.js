import { el } from "./dom.js";
import { renderEntityPage } from "./entity-page.js";
import { renderTypePage } from "./type-page.js";
import { renderHomePage } from "./home-page.js";
import { mountToolbar } from "./toolbar.js";
import { createSearchController } from "./search.js";

const vscode = acquireVsCodeApi();
const root = document.getElementById("root");
const toolbarHost = document.getElementById("toolbar");

const persisted = vscode.getState() || {};
const collapsed = new Set(persisted.collapsed || []);
let activeTab = persisted.activeTab || "state"; // "state" | "history"
const scrollByPage = persisted.scrollByPage || {}; // pageKey → scrollY
let lastPageKey = persisted.lastPageKey || null;
let palette = [];

function saveState() {
  vscode.setState({
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

const ctx = {
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

function pageKey(page) {
  if (!page) return "";
  return page.kind + ":" + page.value;
}

function render(msg) {
  palette = msg.palette || [];
  const page = msg.page;
  const key = pageKey(page);
  const sameAsLast = key === lastPageKey;

  search.setCatalog(msg.catalog);
  toolbar.setBreadcrumbs(msg.breadcrumbs || [], msg.cursor ?? -1);
  toolbar.setNav(!!msg.canBack, !!msg.canForward);

  root.innerHTML = "";

  let nodes = [];
  if (!page || page.kind === "home") {
    nodes = renderHomePage(msg.catalog, ctx);
  } else if (page.kind === "type") {
    nodes = renderTypePage(msg.payload, ctx);
  } else {
    nodes = renderEntityPage(msg.payload, ctx);
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

window.addEventListener("message", (e) => {
  const msg = e.data;
  if (msg.type === "page") {
    render(msg);
  } else if (msg.type === "error") {
    root.innerHTML = "";
    root.appendChild(el("p", { class: "err" }, msg.message || "Unknown error"));
  }
});

vscode.postMessage({ type: "ready" });
