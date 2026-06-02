import { el, type Child } from "./dom.ts";
import { colour } from "./segments.ts";
import type { PageCtx } from "./ctx.ts";

export interface RelationRef {
  label: string;
  annotation?: string;
  colourIndex?: number;
}

export interface RelationGroup {
  header: string;
  incoming?: boolean;
  items: RelationRef[];
}

// renderRelationsBody builds the resolved-relations view: a borderless table
// matching the state display, one row per group. The key cell is the adaptive
// header (e.g. "children", "spouse") with a direction arrow; the value cell is
// the related entities as coloured, clickable pills. Incoming generic edges
// point inward (←) since they have no reciprocal to render forward.
export function renderRelationsBody(
  groups: RelationGroup[] | undefined,
  ctx: PageCtx,
): HTMLElement[] {
  if (!groups || !groups.length) {
    return [el("p", { class: "empty" }, "No relations.")];
  }
  const tbl = el("table", { class: "fields" });
  for (const g of groups) tbl.appendChild(relationRow(g, ctx));
  return [tbl];
}

function relationRow(g: RelationGroup, ctx: PageCtx): HTMLElement {
  const arrow = g.incoming ? "←" : "→";
  const items: Child[] = [];
  g.items.forEach((it, i) => {
    if (i > 0) items.push(", ");
    items.push(relationEntity(it, ctx));
  });
  return el("tr", null,
    // nowrap keeps the label and its arrow together even when the entity
    // list in the value cell wraps across lines.
    el("td", { class: "k relation-head" }, g.header + " " + arrow),
    el("td", null, ...items),
  );
}

function relationEntity(it: RelationRef, ctx: PageCtx): HTMLElement {
  const c = colour(ctx.palette, it.colourIndex);
  const style: Partial<CSSStyleDeclaration> = c ? { color: c } : {};
  const attrs: Record<string, unknown> = { class: "relation-ent" };
  // Only resolved entities (colourIndex >= 0) are navigable.
  if (it.colourIndex !== undefined && it.colourIndex >= 0) {
    style.cursor = "pointer";
    const target = it.label;
    attrs.onclick = () => ctx.openEntity(target);
  }
  if (Object.keys(style).length) attrs.style = style;
  const name = el("span", attrs, it.label);
  if (!it.annotation) return name;
  return el("span", null, name, el("span", { class: "relation-annotation" }, " (" + it.annotation + ")"));
}
