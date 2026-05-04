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
}
interface FocusMessage {
  type: "focus";
  entity: string;
}
interface ErrorMessage {
  type: "error";
  message?: string;
}
type WebviewMessage = GraphMessage | FocusMessage | ErrorMessage;

interface PersistedState {
  focus?: string;
  source?: string;
  hopLimit?: number | null;
}

const vscode = acquireVsCodeApi();
const root = document.getElementById("root")!;
const toolbarHost = document.getElementById("toolbar")!;

const persisted = (vscode.getState<PersistedState>() ?? {}) as PersistedState;
let palette: string[] = [];
let focus: string | null = persisted.focus ?? null;
let hopLimit: number | null = persisted.hopLimit === undefined ? 2 : persisted.hopLimit;

function saveState(): void {
  vscode.setState<PersistedState>({
    focus: focus ?? undefined,
    source: persisted.source,
    hopLimit,
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

const view = mountGraph(
  root,
  {
    onFocus(label) {
      focus = label;
      saveState();
      setFocusIndicator(label);
      vscode.postMessage({ type: "focusEntity", entity: label });
      view.setFocus(label);
    },
    onOpenEntity(label) {
      vscode.postMessage({ type: "openEntity", entity: label });
    },
  },
  () => palette,
);

view.setHopLimit(hopLimit);
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
    view.update(msg.payload ?? {}, focus);
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
