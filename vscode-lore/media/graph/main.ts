import {
  mountGraph,
  type GraphPayload,
  type GraphView,
  type OptionSpec,
} from "../shared/graph-view.ts";

interface VsCodeApi {
  postMessage(msg: unknown): void;
  getState<T = unknown>(): T | undefined;
  setState<T>(state: T): void;
}
declare function acquireVsCodeApi(): VsCodeApi;

interface GraphMessage {
  type: "graph";
  payload: GraphPayload | null | undefined;
  palette?: string[];
  focus?: string | null;
  arrowSize?: number;
  filteredTypes?: string[] | null;
}
interface FocusMessage {
  type: "focus";
  entity: string;
}
interface FilteredTypesMessage {
  type: "filteredTypes";
  filteredTypes: string[] | null;
}
interface ErrorMessage {
  type: "error";
  message?: string;
}
interface InfoMessage {
  type: "info";
  message?: string;
}
type WebviewMessage =
  | GraphMessage
  | FocusMessage
  | FilteredTypesMessage
  | ErrorMessage
  | InfoMessage;

type LayoutOptionsMap = Record<string, Record<string, number | boolean>>;

interface PersistedState {
  focus?: string;
  source?: string;
  hopLimit?: number | null;
  activeLayoutId?: string;
  layouts?: LayoutOptionsMap;
  settingsPanelOpen?: boolean;
  // Legacy flat keys — read once on startup, then dropped.
  clustering?: boolean;
  linkDistance?: number;
  linkStrength?: number;
  clusterStrength?: number;
  clusterDistance?: number;
  chargeStrength?: number;
  centroidGravity?: number;
}

const LEGACY_OPTION_KEYS = [
  "linkDistance",
  "linkStrength",
  "clusterStrength",
  "clusterDistance",
  "chargeStrength",
  "centroidGravity",
  "clustering",
] as const;

const vscode = acquireVsCodeApi();
const root = document.getElementById("root")!;
const toolbarHost = document.getElementById("toolbar")!;

const persisted = migrate((vscode.getState<PersistedState>() ?? {}) as PersistedState);

let palette: string[] = [];
let focus: string | null = persisted.focus ?? null;
let hopLimit: number | null = persisted.hopLimit === undefined ? 2 : persisted.hopLimit;
let filteredTypes: string[] | null = null;
let panelOpen = persisted.settingsPanelOpen === true;

// migrate folds legacy flat option keys into the layouts namespace under
// force-hierarchical — the only layout that existed before this change.
// Returns a fresh state object with legacy keys stripped; saves it back
// to vscode storage so subsequent loads skip the migration entirely.
function migrate(raw: PersistedState): PersistedState {
  const next: PersistedState = {
    focus: raw.focus,
    source: raw.source,
    hopLimit: raw.hopLimit,
    activeLayoutId: raw.activeLayoutId,
    layouts: raw.layouts ? { ...raw.layouts } : {},
    settingsPanelOpen: raw.settingsPanelOpen,
  };
  let migrated = false;
  for (const key of LEGACY_OPTION_KEYS) {
    const v = raw[key];
    if (v === undefined) continue;
    migrated = true;
    const bucket = (next.layouts!["force-hierarchical"] ??= {});
    if (bucket[key] === undefined) bucket[key] = v;
  }
  if (migrated) vscode.setState(next);
  return next;
}

function saveState(): void {
  vscode.setState<PersistedState>({
    focus: focus ?? undefined,
    source: persisted.source,
    hopLimit,
    activeLayoutId: persisted.activeLayoutId,
    layouts: persisted.layouts,
    settingsPanelOpen: panelOpen,
  });
}

function readOption(layoutId: string, key: string): number | boolean | undefined {
  return persisted.layouts?.[layoutId]?.[key];
}

function writeOption(layoutId: string, key: string, value: number | boolean): void {
  persisted.layouts ??= {};
  (persisted.layouts[layoutId] ??= {})[key] = value;
  saveState();
}

const HOP_OPTIONS: { label: string; value: number | null }[] = [
  { label: "1", value: 1 },
  { label: "2", value: 2 },
  { label: "all", value: null },
];

const view = mountGraph(
  root,
  {
    onFocus(label) {
      focus = label;
      saveState();
      setFocusIndicator(label);
      // Only propagate selection events to the extension (and onward
      // to the wiki via the focus bus). A null label is a local
      // deselect — leave the wiki on whatever it was last showing.
      if (label) {
        vscode.postMessage({ type: "focusEntity", entity: label });
      }
      view.setFocus(label);
    },
    onOpenEntity(label) {
      vscode.postMessage({ type: "openEntity", entity: label });
    },
  },
  () => palette,
);

// activeLayout is mutable so layout switches can replace it. Initial
// value is the registry default; we may swap to a persisted choice
// asynchronously below.
let activeLayout = view.getLayout();

// Apply persisted (or default) option values for the active default
// layout immediately, so first paint reflects user preference even
// before any persisted-layout switch lands.
for (const opt of activeLayout.options) {
  const stored = readOption(activeLayout.id, opt.key);
  const value = stored !== undefined ? stored : opt.default;
  view.setOption(opt.key, value);
}

view.setHopLimit(hopLimit);

let panelBody: HTMLElement | null = null;
let panelTitleEl: HTMLSpanElement | null = null;
const settingsPanel = buildSettingsPanel();
let settingsToggleBtn: HTMLButtonElement | null = null;
buildToolbar();
applyPanelOpen();

// If user previously had a non-default layout active, switch now —
// non-default factories are lazy-loaded so this is async. The graph
// renders with the default layout first, then re-renders under the
// persisted layout once its chunk arrives.
const wantedLayoutId = persisted.activeLayoutId ?? activeLayout.id;
if (
  wantedLayoutId !== activeLayout.id
  && view.getLayouts().some((l) => l.id === wantedLayoutId)
) {
  void (async () => {
    await view.setLayout(wantedLayoutId, (key) =>
      readOption(wantedLayoutId, key),
    );
    activeLayout = view.getLayout();
    persisted.activeLayoutId = activeLayout.id;
    saveState();
    if (panelTitleEl) panelTitleEl.textContent = activeLayout.label;
    renderPanelBody();
  })();
} else {
  persisted.activeLayoutId = activeLayout.id;
}

function buildToolbar(): void {
  toolbarHost.innerHTML = "";
  toolbarHost.appendChild(buildScopeGroup());
  toolbarHost.appendChild(buildTypesButton());
  toolbarHost.appendChild(buildLayoutDropdown());

  const focusLabel = document.createElement("span");
  focusLabel.className = "focus-indicator";
  focusLabel.textContent = focus ? `Focus: ${focus}` : "Focus: (none)";
  focusLabel.id = "focus-indicator";
  toolbarHost.appendChild(focusLabel);

  // ⚙︎ pinned to far right via .settings-toggle { margin-left: auto }.
  const settingsBtn = document.createElement("button");
  settingsBtn.type = "button";
  settingsBtn.className = "scope-btn settings-toggle";
  settingsBtn.id = "settings-toggle";
  settingsBtn.textContent = "⚙";
  settingsBtn.title = "Layout settings";
  settingsBtn.addEventListener("click", (ev) => {
    ev.stopPropagation();
    setPanelOpen(!panelOpen);
  });
  toolbarHost.appendChild(settingsBtn);
  settingsToggleBtn = settingsBtn;

  toolbarHost.appendChild(settingsPanel);
}

function buildScopeGroup(): HTMLElement {
  const scopeGroup = document.createElement("div");
  scopeGroup.className = "scope-group";
  const scopeLabel = document.createElement("span");
  scopeLabel.className = "scope-label";
  scopeLabel.textContent = "Hops:";
  scopeGroup.appendChild(scopeLabel);
  for (const opt of HOP_OPTIONS) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "scope-btn";
    btn.textContent = opt.label;
    btn.dataset.value = opt.value === null ? "all" : String(opt.value);
    if (opt.value === hopLimit) btn.classList.add("active");
    btn.addEventListener("click", () => {
      hopLimit = opt.value;
      saveState();
      for (const sibling of scopeGroup.querySelectorAll(".scope-btn")) {
        sibling.classList.toggle("active", sibling === btn);
      }
      view.setHopLimit(hopLimit);
    });
    scopeGroup.appendChild(btn);
  }
  return scopeGroup;
}

function buildTypesButton(): HTMLButtonElement {
  const typesBtn = document.createElement("button");
  typesBtn.type = "button";
  typesBtn.className = "scope-btn types-btn";
  typesBtn.id = "types-btn";
  typesBtn.textContent = typesButtonLabel();
  typesBtn.addEventListener("click", () => {
    vscode.postMessage({ type: "openTypeFilter" });
  });
  return typesBtn;
}

function buildLayoutDropdown(): HTMLSelectElement {
  const sel = document.createElement("select");
  sel.className = "scope-btn layout-select";
  sel.id = "layout-select";
  sel.title = "Layout";
  for (const choice of view.getLayouts()) {
    const opt = document.createElement("option");
    opt.value = choice.id;
    opt.textContent = choice.label;
    if (choice.id === activeLayout.id) opt.selected = true;
    sel.appendChild(opt);
  }
  sel.addEventListener("change", () => {
    void switchLayout(sel.value);
  });
  return sel;
}

async function switchLayout(id: string): Promise<void> {
  if (id === activeLayout.id) return;
  await view.setLayout(id, (key) => readOption(id, key));
  activeLayout = view.getLayout();
  persisted.activeLayoutId = id;
  saveState();
  if (panelTitleEl) panelTitleEl.textContent = activeLayout.label;
  renderPanelBody();
}

function setFocusIndicator(label: string | null): void {
  const el = document.getElementById("focus-indicator");
  if (!el) return;
  el.textContent = label ? `Focus: ${label}` : "Focus: (none)";
}

function typesButtonLabel(): string {
  if (!filteredTypes || filteredTypes.length === 0) return "Types: all";
  return `Types: ${filteredTypes.length}`;
}

function refreshTypesButton(): void {
  const el = document.getElementById("types-btn");
  if (!el) return;
  el.textContent = typesButtonLabel();
}

// Settings panel — absolutely positioned child of #toolbar. Renders one
// row per engine.option (range or toggle). Clustering is handled by the
// toolbar button so we skip it here. Open/close is driven by the ⚙︎
// toggle, outside-clicks, and Escape.
function buildSettingsPanel(): HTMLElement {
  const panel = document.createElement("div");
  panel.className = "settings-panel";
  panel.id = "settings-panel";

  const header = document.createElement("header");
  header.className = "settings-header";
  const title = document.createElement("span");
  title.className = "settings-title";
  title.textContent = activeLayout.label;
  panelTitleEl = title;
  header.appendChild(title);
  const close = document.createElement("button");
  close.type = "button";
  close.className = "settings-close";
  close.textContent = "×";
  close.title = "Close";
  close.addEventListener("click", () => setPanelOpen(false));
  header.appendChild(close);
  panel.appendChild(header);

  const body = document.createElement("div");
  body.className = "settings-body";
  panelBody = body;
  renderPanelBody();
  panel.appendChild(body);

  const footer = document.createElement("div");
  footer.className = "settings-footer";
  const restoreBtn = document.createElement("button");
  restoreBtn.type = "button";
  restoreBtn.className = "scope-btn settings-restore";
  restoreBtn.textContent = "Restore defaults";
  restoreBtn.addEventListener("click", restoreDefaults);
  footer.appendChild(restoreBtn);
  panel.appendChild(footer);

  // Outside-click closes the panel. Click handlers inside the panel and
  // on the ⚙︎ toggle stop propagation so they don't trip this.
  document.addEventListener("click", (ev) => {
    if (!panelOpen) return;
    const target = ev.target as Node;
    if (panel.contains(target)) return;
    if (settingsToggleBtn?.contains(target)) return;
    setPanelOpen(false);
  });
  document.addEventListener("keydown", (ev) => {
    if (ev.key === "Escape" && panelOpen) setPanelOpen(false);
  });
  panel.addEventListener("click", (ev) => ev.stopPropagation());

  return panel;
}

function renderPanelBody(): void {
  if (!panelBody) return;
  panelBody.innerHTML = "";
  for (const opt of activeLayout.options) {
    panelBody.appendChild(makeOptionRow(opt));
  }
}

function restoreDefaults(): void {
  for (const opt of activeLayout.options) {
    writeOption(activeLayout.id, opt.key, opt.default);
    view.setOption(opt.key, opt.default);
  }
  renderPanelBody();
}

function makeOptionRow(opt: OptionSpec): HTMLElement {
  if (opt.type === "toggle") return makeToggleRow(opt);
  return makeRangeRow(opt);
}

function makeRangeRow(opt: Extract<OptionSpec, { type: "range" }>): HTMLElement {
  const stored = readOption(activeLayout.id, opt.key);
  const initial = typeof stored === "number" ? stored : opt.default;
  const wrap = document.createElement("label");
  wrap.className = "slider";
  const labelEl = document.createElement("span");
  labelEl.className = "slider-label";
  labelEl.textContent = opt.label;
  wrap.appendChild(labelEl);
  const range = document.createElement("input");
  range.type = "range";
  range.min = String(opt.min);
  range.max = String(opt.max);
  range.step = String(opt.step);
  range.value = String(initial);
  wrap.appendChild(range);
  const value = document.createElement("span");
  value.className = "slider-value";
  const fmt = (n: number) => opt.decimals !== undefined
    ? n.toFixed(opt.decimals)
    : String(Math.round(n));
  value.textContent = fmt(initial);
  wrap.appendChild(value);
  range.addEventListener("input", () => {
    const v = Number(range.value);
    value.textContent = fmt(v);
    writeOption(activeLayout.id, opt.key, v);
    view.setOption(opt.key, v);
  });
  return wrap;
}

function makeToggleRow(opt: Extract<OptionSpec, { type: "toggle" }>): HTMLElement {
  const stored = readOption(activeLayout.id, opt.key);
  const initial = typeof stored === "boolean" ? stored : opt.default;
  const wrap = document.createElement("label");
  wrap.className = "settings-toggle-row";
  const labelEl = document.createElement("span");
  labelEl.className = "slider-label";
  labelEl.textContent = opt.label;
  wrap.appendChild(labelEl);
  const cb = document.createElement("input");
  cb.type = "checkbox";
  cb.checked = initial;
  wrap.appendChild(cb);
  cb.addEventListener("change", () => {
    writeOption(activeLayout.id, opt.key, cb.checked);
    view.setOption(opt.key, cb.checked);
  });
  return wrap;
}

function setPanelOpen(open: boolean): void {
  panelOpen = open;
  applyPanelOpen();
  saveState();
}

function applyPanelOpen(): void {
  settingsPanel.classList.toggle("open", panelOpen);
  settingsToggleBtn?.classList.toggle("active", panelOpen);
}

window.addEventListener("message", (e: MessageEvent<WebviewMessage>) => {
  const msg = e.data;
  if (msg.type === "graph") {
    palette = msg.palette ?? palette;
    if (msg.focus !== undefined) {
      focus = msg.focus ?? null;
      saveState();
      setFocusIndicator(focus);
    }
    if (typeof msg.arrowSize === "number") {
      view.setArrowSize(msg.arrowSize);
    }
    if (msg.filteredTypes !== undefined) {
      filteredTypes = msg.filteredTypes ?? null;
      view.setTypeFilter(filteredTypes);
      refreshTypesButton();
    }
    setOverlay(null);
    view.update(msg.payload ?? {}, focus);
  } else if (msg.type === "filteredTypes") {
    filteredTypes = msg.filteredTypes ?? null;
    view.setTypeFilter(filteredTypes);
    refreshTypesButton();
  } else if (msg.type === "focus") {
    focus = msg.entity;
    saveState();
    setFocusIndicator(focus);
    view.setFocus(focus);
  } else if (msg.type === "error") {
    setOverlay({ className: "err", text: msg.message ?? "Unknown error" });
  } else if (msg.type === "info") {
    setOverlay({ className: "no-project", text: msg.message ?? "" });
  }
});

// setOverlay drives the message layer that sits over the graph canvas.
// Info/error states used to wipe `root.innerHTML`, which also tore out
// the mounted graph view's DOM — so the next `graph` message had nothing
// to update and the stale message stuck. Keep view mounted; toggle an
// overlay sibling instead.
function setOverlay(
  state: { className: string; text: string } | null,
): void {
  const overlay = document.getElementById("message-overlay");
  if (!overlay) return;
  if (!state) {
    overlay.hidden = true;
    overlay.textContent = "";
    overlay.className = "";
    return;
  }
  overlay.hidden = false;
  overlay.className = state.className;
  overlay.textContent = state.text;
}

vscode.postMessage({ type: "ready" });
