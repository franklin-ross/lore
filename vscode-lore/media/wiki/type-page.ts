import { el, basename, locLine, aliasSpans, type LspLocation } from "./dom.ts";
import { colour, renderSegments, type ContextSegment } from "./segments.ts";
import type { PageCtx } from "./ctx.ts";

export interface TypeEntityDefinition {
  segments: ContextSegment[];
  location: LspLocation;
  startLine: number;
  endLine: number;
}
export interface TypeEntityEntry {
  name: string;
  colourIndex?: number;
  aliases?: string[];
  tags?: string[];
  location: LspLocation;
  definitions?: TypeEntityDefinition[];
}
export interface TypeDetails {
  found: boolean;
  type?: string;
  entities?: TypeEntityEntry[];
}

// renderTypePage builds the DOM for a type page: header followed by one
// `.type-entry` per entity of the type. Each entry is the entity name
// (clickable to its wiki page) plus a list of definition rows — one row per
// authored description block, formatted like a reference row so the page
// reads as a directory of where the entity is defined.
export function renderTypePage(d: TypeDetails | undefined, ctx: PageCtx): HTMLElement[] {
  if (!d || !d.found || !d.entities) {
    return [el("p", { class: "empty" }, "No entities found for this type in this project.")];
  }
  const entities = d.entities;
  const out: HTMLElement[] = [
    el("h1", null, d.type ?? ""),
    el(
      "div",
      { class: "aliases" },
      entities.length === 1 ? "1 entity" : entities.length + " entities",
    ),
  ];
  for (const ent of entities) {
    out.push(renderTypeEntityEntry(ent, ctx));
  }
  return out;
}

function renderTypeEntityEntry(ent: TypeEntityEntry, ctx: PageCtx): HTMLElement {
  const c = colour(ctx.palette, ent.colourIndex);
  const heading = el(
    "h3",
    {
      style: c ? ({ color: c } as Partial<CSSStyleDeclaration>) : undefined,
      title: "Open entity page",
      onclick: () => ctx.openEntity(ent.name),
    },
    ent.name,
    ...aliasSpans(ent.aliases, c),
  );
  const wrap = el("div", { class: "type-entry" }, heading);
  if (ent.definitions && ent.definitions.length) {
    for (const def of ent.definitions) {
      wrap.appendChild(definitionRow(def, ctx));
    }
  } else {
    wrap.appendChild(el("p", { class: "empty" }, "No definitions."));
  }
  return wrap;
}

// definitionRow mirrors the reference-row layout: clickable, with a file:line
// label on the left and the colourised description preview on the right.
// Inline name mentions inside the preview don't drill into their wiki — the
// row click owns navigation, so segment children get linkToWiki=false.
function definitionRow(def: TypeEntityDefinition, ctx: PageCtx): HTMLElement {
  const line = locLine(def.location);
  const ctxSpan = el("span", { class: "ctx" });
  renderSegments(def.segments, ctxSpan, ctx.palette, ctx.openEntity, false);
  return el(
    "div",
    {
      class: "ref-row",
      onclick: () => ctx.navigate(def.location.uri, line, def.location.range),
    },
    el("span", { class: "loc" }, basename(def.location.uri) + ":" + line),
    ctxSpan,
  );
}
