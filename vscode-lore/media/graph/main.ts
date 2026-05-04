import { mountGraph, type GraphPayload, type GraphView } from "./graph-view.ts";

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
type WebviewMessage = GraphMessage | FocusMessage | FilteredTypesMessage | ErrorMessage;

interface PersistedState {
  focus?: string;
  source?: string;
  hopLimit?: number | null;
  clustering?: boolean;
  linkDistance?: number;
  linkStrength?: number;
  clusterStrength?: number;
  clusterDistance?: number;
  chargeStrength?: number;
  centroidGravity?: number;
}

const vscode = acquireVsCodeApi();
const root = document.getElementById("root")!;
const toolbarHost = document.getElementById("toolbar")!;

const persisted = (vscode.getState<PersistedState>() ?? {}) as PersistedState;
let palette: string[] = [];
let focus: string | null = persisted.focus ?? null;
let hopLimit: number | null = persisted.hopLimit === undefined ? 2 : persisted.hopLimit;
let clustering = persisted.clustering === true;
let linkDistance = persisted.linkDistance ?? 90;
let linkStrength = persisted.linkStrength ?? 0.6;
let clusterStrength = persisted.clusterStrength ?? 1.0;
let clusterDistance = persisted.clusterDistance ?? 60;
let chargeStrength = persisted.chargeStrength ?? -1500;
let centroidGravity = persisted.centroidGravity ?? 0;
let filteredTypes: string[] | null = null;

function saveState(): void {
  vscode.setState<PersistedState>({
    focus: focus ?? undefined,
    source: persisted.source,
    hopLimit,
    clustering,
    linkDistance,
    linkStrength,
    clusterStrength,
    clusterDistance,
    chargeStrength,
    centroidGravity,
  });
}

const HOP_OPTIONS: { label: string; value: number | null }[] = [
  { label: "1", value: 1 },
  { label: "2", value: 2 },
  { label: "all", value: null },
];

function buildToolbar(view: GraphView): void {
  toolbarHost.innerHTML = "";

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
  toolbarHost.appendChild(scopeGroup);

  const typesBtn = document.createElement("button");
  typesBtn.type = "button";
  typesBtn.className = "scope-btn types-btn";
  typesBtn.id = "types-btn";
  typesBtn.textContent = typesButtonLabel();
  typesBtn.addEventListener("click", () => {
    vscode.postMessage({ type: "openTypeFilter" });
  });
  toolbarHost.appendChild(typesBtn);

  const clusterBtn = document.createElement("button");
  clusterBtn.type = "button";
  clusterBtn.className = "scope-btn";
  clusterBtn.textContent = "Cluster";
  if (clustering) clusterBtn.classList.add("active");
  clusterBtn.addEventListener("click", () => {
    clustering = !clustering;
    saveState();
    clusterBtn.classList.toggle("active", clustering);
    view.setClustering(clustering);
  });
  toolbarHost.appendChild(clusterBtn);

  toolbarHost.appendChild(makeSlider({
    label: "link dist", min: 20, max: 300, step: 5, value: linkDistance,
    onChange: (v) => { linkDistance = v; saveState(); view.setLinkDistance(v); },
  }));
  toolbarHost.appendChild(makeSlider({
    label: "link str", min: 0, max: 1, step: 0.05, value: linkStrength, decimals: 2,
    onChange: (v) => { linkStrength = v; saveState(); view.setLinkStrength(v); },
  }));
  toolbarHost.appendChild(makeSlider({
    label: "cluster str", min: 0, max: 2, step: 0.05, value: clusterStrength, decimals: 2,
    onChange: (v) => { clusterStrength = v; saveState(); view.setClusterStrength(v); },
  }));
  toolbarHost.appendChild(makeSlider({
    label: "cluster dist", min: 20, max: 200, step: 5, value: clusterDistance,
    onChange: (v) => { clusterDistance = v; saveState(); view.setClusterDistance(v); },
  }));
  toolbarHost.appendChild(makeSlider({
    label: "charge", min: -3000, max: 0, step: 25, value: chargeStrength,
    onChange: (v) => { chargeStrength = v; saveState(); view.setChargeStrength(v); },
  }));
  toolbarHost.appendChild(makeSlider({
    label: "cluster gravity", min: 0, max: 1, step: 0.02, value: centroidGravity, decimals: 2,
    onChange: (v) => { centroidGravity = v; saveState(); view.setCentroidGravity(v); },
  }));

  const focusLabel = document.createElement("span");
  focusLabel.className = "focus-indicator";
  focusLabel.textContent = focus ? `Focus: ${focus}` : "Focus: (none)";
  focusLabel.id = "focus-indicator";
  toolbarHost.appendChild(focusLabel);
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

interface SliderSpec {
  label: string;
  min: number;
  max: number;
  step: number;
  value: number;
  decimals?: number;
  onChange(value: number): void;
}

function makeSlider(spec: SliderSpec): HTMLElement {
  const wrap = document.createElement("label");
  wrap.className = "slider";
  const labelEl = document.createElement("span");
  labelEl.className = "slider-label";
  labelEl.textContent = spec.label;
  wrap.appendChild(labelEl);
  const range = document.createElement("input");
  range.type = "range";
  range.min = String(spec.min);
  range.max = String(spec.max);
  range.step = String(spec.step);
  range.value = String(spec.value);
  wrap.appendChild(range);
  const value = document.createElement("span");
  value.className = "slider-value";
  const fmt = (n: number) => spec.decimals !== undefined
    ? n.toFixed(spec.decimals)
    : String(Math.round(n));
  value.textContent = fmt(spec.value);
  wrap.appendChild(value);
  range.addEventListener("input", () => {
    const v = Number(range.value);
    value.textContent = fmt(v);
    spec.onChange(v);
  });
  return wrap;
}

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

view.setHopLimit(hopLimit);
view.setLinkDistance(linkDistance);
view.setLinkStrength(linkStrength);
view.setClusterStrength(clusterStrength);
view.setClusterDistance(clusterDistance);
view.setChargeStrength(chargeStrength);
view.setCentroidGravity(centroidGravity);
view.setClustering(clustering);
buildToolbar(view);

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
    root.innerHTML = "";
    const p = document.createElement("p");
    p.className = "err";
    p.textContent = msg.message ?? "Unknown error";
    root.appendChild(p);
  }
});

vscode.postMessage({ type: "ready" });
