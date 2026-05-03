import { el } from "./dom.js";

// renderSegments paints a sequence of text/entity-name chunks emitted by the
// server's buildContextSegments. linkToWiki=true makes coloured entity spans
// clickable: clicking opens that entity's wiki page via openEntity().
// Reference-context previews pass false because their parent row owns the
// click. Ambiguous segments draw a wavy underline + tooltip.
export function renderSegments(segments, parent, palette, openEntity, linkToWiki) {
  if (!segments || !segments.length) return parent;
  for (const seg of segments) {
    const c = colour(palette, seg.colourIndex);
    if (c) {
      const style = { color: c };
      const attrs = { style };
      if (seg.ambiguous) {
        style.textDecoration = "underline wavy var(--vscode-editorWarning-foreground)";
        attrs.title = "Ambiguous reference — " + (seg.entity || seg.text);
      }
      if (linkToWiki && seg.entity) {
        style.cursor = "pointer";
        attrs.onclick = (ev) => {
          ev.stopPropagation();
          openEntity(seg.entity);
        };
      }
      parent.appendChild(el("span", attrs, seg.text));
    } else {
      parent.appendChild(document.createTextNode(seg.text));
    }
  }
  return parent;
}

export function colour(palette, idx) {
  if (idx == null || idx < 0 || idx >= palette.length) return undefined;
  return palette[idx];
}
