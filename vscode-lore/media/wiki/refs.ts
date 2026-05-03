import { el, basename, locLine, section, aliasSpans, type LspLocation } from "./dom.ts";
import { renderSegments, colour, type ContextSegment } from "./segments.ts";
import type { PageCtx } from "./ctx.ts";

export interface RefItem {
  segments: ContextSegment[];
  location: LspLocation;
}

export interface RefGroup {
  source: string;
  aliases?: string[];
  colourIndex?: number;
  refs: RefItem[];
}

// renderRefGroups builds the inbound/outbound reference list used on the
// entity page. Each group's heading is the source/target entity name; click
// opens that entity's wiki. freeTextLabel is the heading shown for refs
// outside any entity definition (pass "" to drop them).
export function renderRefGroups(
  title: string,
  groups: RefGroup[] | undefined,
  freeTextLabel: string,
  ctx: PageCtx,
): HTMLElement[] {
  if (!groups || !groups.length) return [];
  const items: HTMLElement[] = [];
  for (const g of groups) {
    const label = g.source || freeTextLabel;
    if (!label) continue;
    items.push(refGroup(label, g, ctx));
  }
  if (!items.length) return [];
  return [section(title, ctx.collapsed, ctx.onToggle, ...items)];
}

function refGroup(label: string, g: RefGroup, ctx: PageCtx): HTMLElement {
  const c = colour(ctx.palette, g.colourIndex);
  const headingAttrs: Record<string, unknown> = {};
  const headingStyle: Partial<CSSStyleDeclaration> = c ? { color: c } : {};
  if (g.source) {
    headingStyle.cursor = "pointer";
    const target = g.source;
    headingAttrs.onclick = () => ctx.openEntity(target);
  }
  if (Object.keys(headingStyle).length) headingAttrs.style = headingStyle;
  const heading = el("h3", headingAttrs, label, ...aliasSpans(g.aliases, c));
  const group = el("div", { class: "ref-group" }, heading);
  for (const r of g.refs) {
    group.appendChild(refRow(r, ctx));
  }
  return group;
}

export function refRow(r: RefItem, ctx: PageCtx): HTMLElement {
  const line = locLine(r.location);
  const ctxSpan = el("span", { class: "ctx" });
  renderSegments(r.segments, ctxSpan, ctx.palette, ctx.openEntity, false);
  return el(
    "div",
    {
      class: "ref-row",
      onclick: () => ctx.navigate(r.location.uri, line, r.location.range),
    },
    el("span", { class: "loc" }, basename(r.location.uri) + ":" + line),
    ctxSpan,
  );
}
