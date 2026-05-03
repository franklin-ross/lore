import { el, basename, locLine, section, aliasSpans } from "./dom.js";
import { renderSegments, colour } from "./segments.js";

// renderRefGroups builds the inbound/outbound reference list used on the
// entity page. Each group's heading is the source/target entity name; click
// opens that entity's wiki. freeTextLabel is the heading shown for refs
// outside any entity definition (pass "" to drop them).
export function renderRefGroups(title, groups, freeTextLabel, ctx) {
  if (!groups || !groups.length) return [];
  const items = [];
  for (const g of groups) {
    const label = g.source || freeTextLabel;
    if (!label) continue;
    items.push(refGroup(label, g, ctx));
  }
  if (!items.length) return [];
  return [section(title, ctx.collapsed, ctx.onToggle, ...items)];
}

function refGroup(label, g, ctx) {
  const c = colour(ctx.palette, g.colourIndex);
  const headingAttrs = {};
  const headingStyle = c ? { color: c } : {};
  if (g.source) {
    headingStyle.cursor = "pointer";
    headingAttrs.onclick = () => ctx.openEntity(g.source);
  }
  if (Object.keys(headingStyle).length) headingAttrs.style = headingStyle;
  const heading = el("h3", headingAttrs, label, ...aliasSpans(g.aliases, c));
  const group = el("div", { class: "ref-group" }, heading);
  for (const r of g.refs) {
    group.appendChild(refRow(r, ctx));
  }
  return group;
}

export function refRow(r, ctx) {
  const line = locLine(r.location);
  const ctxSpan = el("span", { class: "ctx" });
  renderSegments(r.segments, ctxSpan, ctx.palette, ctx.openEntity, false);
  return el("div", {
    class: "ref-row",
    onclick: () => ctx.navigate(r.location.uri, line),
  },
    el("span", { class: "loc" }, basename(r.location.uri) + ":" + line),
    ctxSpan
  );
}
