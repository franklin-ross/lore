export type Child = Node | string | null | undefined;
export type Children = Child | Child[];

export interface ElAttrs {
  class?: string;
  style?: Partial<CSSStyleDeclaration>;
  onclick?: ((ev: MouseEvent) => void) | null;
  data?: Record<string, string>;
  [key: string]: unknown;
}

// Tiny createElement helper. Skips null children, supports class/style/onclick
// shortcuts, and handles a `data` map for dataset assignment. All page
// renderers go through this so attribute application stays consistent.
export function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  attrs: ElAttrs | null,
  ...children: Children[]
): HTMLElementTagNameMap[K];
export function el(tag: string, attrs: ElAttrs | null, ...children: Children[]): HTMLElement;
export function el(tag: string, attrs: ElAttrs | null, ...children: Children[]): HTMLElement {
  const node = document.createElement(tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (v == null) continue;
      if (k === "style") Object.assign(node.style, v as Partial<CSSStyleDeclaration>);
      else if (k === "class") node.className = v as string;
      else if (k === "onclick") node.onclick = v as (ev: MouseEvent) => void;
      else if (k === "data") {
        for (const [dk, dv] of Object.entries(v as Record<string, string>)) node.dataset[dk] = dv;
      } else {
        node.setAttribute(k, String(v));
      }
    }
  }
  for (const c of children) {
    appendChild(node, c);
  }
  return node;
}

function appendChild(node: HTMLElement, c: Children): void {
  if (c == null) return;
  if (Array.isArray(c)) {
    for (const cc of c) appendChild(node, cc);
    return;
  }
  if (typeof c === "string") {
    node.appendChild(document.createTextNode(c));
  } else {
    node.appendChild(c);
  }
}

// section renders a collapsible `<section>` with an h2 header. `collapsed` is
// the Set of section titles that should start collapsed; clicks toggle and
// notify `onToggle(title, isCollapsed)` so callers can persist state.
export function section(
  title: string,
  collapsed: Set<string>,
  onToggle: (title: string, isCollapsed: boolean) => void,
  ...body: Children[]
): HTMLElement {
  const isCollapsed = collapsed.has(title);
  const head = el("h2", {
    class: "collapsible" + (isCollapsed ? " collapsed" : ""),
  }, title);
  const wrap = el("div", null);
  for (const c of body) appendChild(wrap, c);
  if (isCollapsed) wrap.style.display = "none";
  head.onclick = () => {
    const next = !head.classList.contains("collapsed");
    head.classList.toggle("collapsed", next);
    wrap.style.display = next ? "none" : "";
    if (next) collapsed.add(title);
    else collapsed.delete(title);
    onToggle(title, next);
  };
  const sec = document.createElement("section");
  sec.appendChild(head);
  sec.appendChild(wrap);
  return sec;
}

export function basename(uri: string): string {
  try {
    const path = new URL(uri).pathname;
    const idx = path.lastIndexOf("/");
    return idx >= 0 ? path.slice(idx + 1) : path;
  } catch {
    return uri;
  }
}

export interface LspLocation {
  uri: string;
  range: { start: { line: number; character: number }; end: { line: number; character: number } };
}

export function locLine(loc: LspLocation | null | undefined): number {
  return (loc?.range?.start?.line ?? 0) + 1;
}

// aliasSpans returns one `<span>` per alias, each prefixed with " · " so it
// sits inline next to the canonical name in a header. Returns an empty array
// when there are no aliases. baseColour, when set, paints the aliases in the
// same palette colour as the entity name so the relationship is visible
// without relying on a separate label like "Also known as".
export function aliasSpans(aliases: string[] | undefined, baseColour: string | undefined): HTMLElement[] {
  if (!aliases || !aliases.length) return [];
  const out: HTMLElement[] = [];
  for (const a of aliases) {
    out.push(el(
      "span",
      { class: "alias-inline", style: baseColour ? ({ color: baseColour } as Partial<CSSStyleDeclaration>) : undefined },
      " · " + a,
    ));
  }
  return out;
}
