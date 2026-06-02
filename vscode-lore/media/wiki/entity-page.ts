import { el, basename, section, aliasSpans, type LspLocation } from "./dom.ts";
import { colour } from "./segments.ts";
import { renderMarkdownTree, type MarkdownNode } from "./markdown.ts";
import { renderRefGroups, type RefGroup } from "./refs.ts";
import { renderRelationsBody, type RelationGroup } from "./relations.ts";
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
  op: "add" | "remove" | "set" | "increment" | "edgeAdd" | "edgeRemove" | "unknown";
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
  history?: StateEvent[];
  relations?: RelationGroup[];
}

// renderEntityPage builds the full DOM for one entity's wiki page. ctx
// supplies palette + state callbacks (openEntity, openType, openHome,
// navigate, collapsed, onToggle, search).
export function renderEntityPage(d: EntityDetails | undefined, ctx: PageCtx): HTMLElement[] {
  if (!d || !d.found) {
    return [el("p", { class: "empty" }, "Entity not found in this project.")];
  }
  return [
    ...renderHeader(d, ctx),
    ...renderDetailsTabs(d, ctx),
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
    case "edgeAdd": return ev.target + " → " + (ev.value ?? "");
    case "edgeRemove": return ev.target + " -/> " + (ev.value ?? "");
  }
  return ev.target;
}

function renderHistoryBody(d: EntityDetails, ctx: PageCtx): HTMLElement[] {
  if (!d.history || !d.history.length) {
    return [el("p", { class: "empty" }, "No history.")];
  }
  const out: HTMLElement[] = [];
  for (const ev of d.history) {
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

// renderDetailsTabs shows Relations, State, and History in one tab strip —
// no collapsible wrapper, so it stays compact and keeps the descriptions high
// on the page. Only the tabs with content appear; the first present tab is
// active by default (Relations, then State, then History — history is rarely
// the thing you open to). Switching tabs is local DOM state, so it resets to
// the default when navigating to another entity.
function renderDetailsTabs(d: EntityDetails, ctx: PageCtx): HTMLElement[] {
  const tabs: { label: string; body: HTMLElement[] }[] = [];
  if (d.relations && d.relations.length) {
    tabs.push({ label: "Relations", body: renderRelationsBody(d.relations, ctx) });
  }
  if (d.fields && d.fields.length) {
    tabs.push({ label: "State", body: renderStateBody(d) });
  }
  if (d.history && d.history.length) {
    tabs.push({ label: "History", body: renderHistoryBody(d, ctx) });
  }
  if (!tabs.length) return [];

  const tabBar = el("div", { class: "tabs" });
  const buttons: HTMLElement[] = [];
  const panels: HTMLElement[] = [];
  tabs.forEach((t, i) => {
    const active = i === 0;
    const btn = el("button", { class: "tab" + (active ? " active" : "") }, t.label);
    const panel = el("div", { class: "tab-panel" + (active ? " active" : "") }, ...t.body);
    btn.onclick = () => {
      buttons.forEach((b, j) => b.classList.toggle("active", j === i));
      panels.forEach((p, j) => p.classList.toggle("active", j === i));
    };
    buttons.push(btn);
    panels.push(panel);
    tabBar.appendChild(btn);
  });
  return [tabBar, ...panels];
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
