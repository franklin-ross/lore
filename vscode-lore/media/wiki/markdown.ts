import { el } from "./dom.ts";
import { renderSegments, type ContextSegment } from "./segments.ts";

// MarkdownNode mirrors the structured tree the server emits for a
// description body. Block kinds (paragraph, blockquote, heading, etc.)
// nest via `children`; "text" leaves carry the entity-aware Segments
// stream built server-side; "code-block" / "code-inline" leaves carry
// raw text (no entity scanning, because identifiers inside code
// shouldn't trigger refs).
export interface MarkdownNode {
  kind:
    | "paragraph"
    | "blockquote"
    | "hr"
    | "heading"
    | "list"
    | "list-item"
    | "code-block"
    | "text"
    | "emphasis"
    | "strong"
    | "code-inline"
    | "link";
  children?: MarkdownNode[];
  segments?: ContextSegment[];
  text?: string;
  level?: number;
  ordered?: boolean;
  lang?: string;
  href?: string;
}

// renderMarkdownTree paints a server-emitted MarkdownNode tree into
// `parent` by building DOM via createElement and text nodes — never
// innerHTML. User prose containing `<script>` therefore lands as
// literal characters, not an executable tag.
//
// Inline content within each block is rendered through renderSegments
// for text leaves, so entity-name colouring + click behaviour survive
// the markdown layer unchanged.
export function renderMarkdownTree(
  content: MarkdownNode[] | undefined,
  parent: HTMLElement,
  palette: string[],
  openEntity: (entity: string) => void,
  linkToWiki: boolean,
): HTMLElement {
  if (!content) return parent;
  for (const node of content) {
    const built = renderNode(node, palette, openEntity, linkToWiki);
    if (built) parent.appendChild(built);
  }
  return parent;
}

// flattenSegments walks the tree and returns every ContextSegment
// attached to a "text" leaf, in source order. Used where we want a
// compact one-line preview (type-page definition rows) rather than the
// full block layout — the rendered output is the same flat stream the
// pre-tree wire format used.
export function flattenSegments(
  content: MarkdownNode[] | undefined,
): ContextSegment[] {
  if (!content) return [];
  const out: ContextSegment[] = [];
  collect(content, out);
  return out;
}

function collect(nodes: MarkdownNode[], out: ContextSegment[]): void {
  for (const n of nodes) {
    if (n.kind === "text" && n.segments) {
      for (const s of n.segments) out.push(s);
    } else if (n.kind === "code-inline" || n.kind === "code-block") {
      if (n.text) out.push({ text: n.text });
    }
    if (n.children) collect(n.children, out);
  }
}

function renderNode(
  node: MarkdownNode,
  palette: string[],
  openEntity: (entity: string) => void,
  linkToWiki: boolean,
): HTMLElement | Node | null {
  switch (node.kind) {
    case "paragraph":
      return renderInlineContainer("p", null, node, palette, openEntity, linkToWiki);
    case "blockquote":
      return renderBlockContainer("blockquote", null, node, palette, openEntity, linkToWiki);
    case "hr":
      return el("hr", null);
    case "heading": {
      // h1 is reserved for the entity's own name on the page, so
      // markdown headings start at h2 and clamp at h6.
      const lvl = Math.min(6, Math.max(2, (node.level ?? 1) + 1));
      const tag = ("h" + lvl) as "h2" | "h3" | "h4" | "h5" | "h6";
      return renderInlineContainer(tag, null, node, palette, openEntity, linkToWiki);
    }
    case "list":
      return renderBlockContainer(node.ordered ? "ol" : "ul", null, node, palette, openEntity, linkToWiki);
    case "list-item":
      return renderBlockContainer("li", null, node, palette, openEntity, linkToWiki);
    case "code-block": {
      const pre = el("pre", null);
      const code = el("code", node.lang ? { class: "lang-" + node.lang } : null);
      code.appendChild(document.createTextNode(node.text ?? ""));
      pre.appendChild(code);
      return pre;
    }
    case "text":
      return renderTextLeaf(node, palette, openEntity, linkToWiki);
    case "emphasis":
      return renderInlineContainer("em", null, node, palette, openEntity, linkToWiki);
    case "strong":
      return renderInlineContainer("strong", null, node, palette, openEntity, linkToWiki);
    case "code-inline": {
      const code = el("code", null);
      code.appendChild(document.createTextNode(node.text ?? ""));
      return code;
    }
    case "link": {
      // Build the anchor manually because el() doesn't expose href as
      // a known attribute and we want to keep the click contained to
      // the link itself (parent rows often own a navigate handler).
      const a = document.createElement("a");
      a.setAttribute("href", node.href ?? "#");
      a.setAttribute("target", "_blank");
      a.setAttribute("rel", "noopener");
      renderInlineChildrenInto(a, node, palette, openEntity, linkToWiki);
      return a;
    }
  }
  return null;
}

function renderInlineContainer(
  tag: "p" | "em" | "strong" | "h2" | "h3" | "h4" | "h5" | "h6",
  attrs: Parameters<typeof el>[1],
  node: MarkdownNode,
  palette: string[],
  openEntity: (entity: string) => void,
  linkToWiki: boolean,
): HTMLElement {
  const host = el(tag, attrs);
  renderInlineChildrenInto(host, node, palette, openEntity, linkToWiki);
  return host;
}

function renderBlockContainer(
  tag: "blockquote" | "ol" | "ul" | "li",
  attrs: Parameters<typeof el>[1],
  node: MarkdownNode,
  palette: string[],
  openEntity: (entity: string) => void,
  linkToWiki: boolean,
): HTMLElement {
  const host = el(tag, attrs);
  if (node.children) {
    for (const c of node.children) {
      const built = renderNode(c, palette, openEntity, linkToWiki);
      if (built) host.appendChild(built);
    }
  }
  return host;
}

function renderInlineChildrenInto(
  host: HTMLElement,
  node: MarkdownNode,
  palette: string[],
  openEntity: (entity: string) => void,
  linkToWiki: boolean,
): void {
  if (!node.children) return;
  for (const c of node.children) {
    const built = renderNode(c, palette, openEntity, linkToWiki);
    if (built) host.appendChild(built);
  }
}

function renderTextLeaf(
  node: MarkdownNode,
  palette: string[],
  openEntity: (entity: string) => void,
  linkToWiki: boolean,
): DocumentFragment {
  const frag = document.createDocumentFragment();
  if (!node.segments || !node.segments.length) return frag;
  // renderSegments needs a real HTMLElement host; render into a
  // temporary span, then move its children into the fragment so the
  // caller appends a single inline sequence rather than a wrapper.
  const tmp = document.createElement("span");
  renderSegments(node.segments, tmp, palette, openEntity, linkToWiki);
  while (tmp.firstChild) frag.appendChild(tmp.firstChild);
  return frag;
}
