import { el } from "./dom.ts";

export interface ContextSegment {
  text: string;
  entity?: string;
  ambiguous?: boolean;
  colourIndex?: number;
}

// renderSegments paints a sequence of text/entity-name chunks emitted by the
// server's buildContextSegments. linkToWiki=true makes coloured entity spans
// clickable: clicking opens that entity's wiki page via openEntity().
// Reference-context previews pass false because their parent row owns the
// click. Ambiguous segments draw a wavy underline + tooltip.
export function renderSegments(
  segments: ContextSegment[] | undefined,
  parent: HTMLElement,
  palette: string[],
  openEntity: (entity: string) => void,
  linkToWiki: boolean,
): HTMLElement {
  if (!segments || !segments.length) return parent;
  for (const seg of segments) {
    const c = colour(palette, seg.colourIndex);
    if (c) {
      const style: Partial<CSSStyleDeclaration> = { color: c };
      const attrs: Record<string, unknown> = { style };
      if (seg.ambiguous) {
        style.textDecoration = "underline wavy var(--vscode-editorWarning-foreground)";
        attrs.title = "Ambiguous reference — " + (seg.entity || seg.text);
      }
      if (linkToWiki && seg.entity) {
        style.cursor = "pointer";
        const target = seg.entity;
        attrs.onclick = (ev: MouseEvent) => {
          ev.stopPropagation();
          openEntity(target);
        };
      }
      parent.appendChild(el("span", attrs, seg.text));
    } else {
      parent.appendChild(document.createTextNode(seg.text));
    }
  }
  return parent;
}

export function colour(palette: string[], idx: number | undefined): string | undefined {
  if (idx == null || idx < 0 || idx >= palette.length) return undefined;
  return palette[idx];
}
