import { el, basename, section, aliasSpans, type LspLocation } from "./dom.ts";
import { colour } from "./segments.ts";
import { renderMarkdownTree, type MarkdownNode } from "./markdown.ts";
import { renderRefGroups, type RefGroup } from "./refs.ts";
import type { PageCtx } from "./ctx.ts";

export interface EntityField {
  name: string;
  value: string;
}
export interface DescriptionBlock {
  content: MarkdownNode[];
  location: LspLocation;
  startLine: number;
  endLine: number;
}
export interface StateEvent {
  op: "add" | "remove" | "set" | "increment" | "unknown";
  target: string;
  value?: string;
  location: LspLocation;
}
export interface EntityDetails {
  found: boolean;
  name?: string;
  type?: string;
  colourIndex?: number;
  aliases?: string[];
  tags?: string[];
  fields?: EntityField[];
  descriptions?: DescriptionBlock[];
  inboundRefs?: RefGroup[];
  outboundRefs?: RefGroup[];
  stateHistory?: StateEvent[];
}

// renderEntityPage builds the full DOM for one entity's wiki page. ctx
// supplies palette + state callbacks (openEntity, openType, navigate,
// collapsed, onToggle, activeTab, setActiveTab).
export function renderEntityPage(d: EntityDetails | undefined, ctx: PageCtx): HTMLElement[] {
  if (!d || !d.found) {
    return [el("p", { class: "empty" }, "Entity not found in this project.")];
  }
  return [
    ...renderHeader(d, ctx),
    ...renderStateSection(d, ctx),
    ...renderDescriptions(d, ctx),
    ...renderRefGroups("Mentioned by", d.inboundRefs, "Free text", ctx),
    ...renderRefGroups("Mentions", d.outboundRefs, "", ctx),
  ];
}

function renderHeader(d: EntityDetails, ctx: PageCtx): HTMLElement[] {
  const c = colour(ctx.palette, d.colourIndex);
  const name = el("span", { style: c ? ({ color: c } as Partial<CSSStyleDeclaration>) : undefined }, d.name ?? "");
  const h1 = el("h1", null, name);
  for (const span of aliasSpans(d.aliases, c)) h1.appendChild(span);
  if (d.type) {
    const type = d.type;
    h1.appendChild(el("span", {
      class: "type",
      title: "Open type page",
      onclick: () => ctx.openType(type),
    }, type));
  }
  const out: HTMLElement[] = [h1];
  if (d.tags && d.tags.length) {
    const tagWrap = el("div", { class: "header-tags" });
    for (const t of d.tags) tagWrap.appendChild(el("span", { class: "pill" }, "+" + t));
    out.push(tagWrap);
  }
  return out;
}

function renderStateBody(d: EntityDetails): HTMLElement[] {
  if (!d.fields || !d.fields.length) {
    return [el("p", { class: "empty" }, "No fields.")];
  }
  const tbl = el("table", { class: "fields" });
  for (const f of d.fields) {
    tbl.appendChild(el("tr", null,
      el("td", { class: "k" }, f.name),
      el("td", null, f.value),
    ));
  }
  return [tbl];
}

function directiveText(ev: StateEvent): string {
  switch (ev.op) {
    case "add": return "+" + ev.target;
    case "remove":
      return ev.value ? ev.target + " -= " + ev.value : "-" + ev.target;
    case "set": return ev.target + " = " + (ev.value ?? "");
    case "increment": return ev.target + " += " + (ev.value ?? "");
  }
  return ev.target;
}

function renderHistoryBody(d: EntityDetails, ctx: PageCtx): HTMLElement[] {
  if (!d.stateHistory || !d.stateHistory.length) {
    return [el("p", { class: "empty" }, "No state history.")];
  }
  const out: HTMLElement[] = [];
  for (const ev of d.stateHistory) {
    const line = (ev.location?.range?.start?.line ?? 0) + 1;
    const row = el("div", {
      class: "history-row",
      onclick: () => ctx.navigate(ev.location.uri, line, ev.location.range),
    },
      el("span", null, directiveText(ev)),
      el("span", { class: "loc" }, basename(ev.location.uri) + ":" + line),
    );
    out.push(row);
  }
  return out;
}

function renderStateSection(d: EntityDetails, ctx: PageCtx): HTMLElement[] {
  const hasFields = d.fields && d.fields.length;
  const hasHistory = d.stateHistory && d.stateHistory.length;
  if (!hasFields && !hasHistory) return [];

  const isHistory = ctx.activeTab === "history";
  const stateTab = el("button", { class: "tab" + (isHistory ? "" : " active") }, "State");
  const historyTab = el("button", { class: "tab" + (isHistory ? " active" : "") }, "History");
  const tabs = el("div", { class: "tabs" }, stateTab, historyTab);

  const statePanel = el("div", { class: "tab-panel" + (isHistory ? "" : " active") }, ...renderStateBody(d));
  const historyPanel = el("div", { class: "tab-panel" + (isHistory ? " active" : "") }, ...renderHistoryBody(d, ctx));

  stateTab.onclick = () => {
    stateTab.classList.add("active");
    historyTab.classList.remove("active");
    statePanel.classList.add("active");
    historyPanel.classList.remove("active");
    ctx.setActiveTab("state");
  };
  historyTab.onclick = () => {
    historyTab.classList.add("active");
    stateTab.classList.remove("active");
    historyPanel.classList.add("active");
    statePanel.classList.remove("active");
    ctx.setActiveTab("history");
  };

  return [section("State", ctx.collapsed, ctx.onToggle, tabs, statePanel, historyPanel)];
}

function renderDescriptions(d: EntityDetails, ctx: PageCtx): HTMLElement[] {
  if (!d.descriptions || !d.descriptions.length) return [];
  const blocks: HTMLElement[] = [];
  for (const block of d.descriptions) {
    const tooltip = block.endLine && block.endLine > block.startLine
      ? basename(block.location.uri) + ":" + block.startLine + "–" + block.endLine
      : basename(block.location.uri) + ":" + block.startLine;
    const jump = el("span", {
      class: "desc-jump",
      title: "Jump to " + tooltip,
      onclick: () => ctx.navigate(block.location.uri, block.startLine),
    });
    const text = el("div", { class: "desc-text" });
    renderMarkdownTree(block.content, text, ctx.palette, ctx.openEntity, true);
    blocks.push(el("div", { class: "desc-block" }, jump, text));
  }
  return [section("Descriptions", ctx.collapsed, ctx.onToggle, ...blocks)];
}
