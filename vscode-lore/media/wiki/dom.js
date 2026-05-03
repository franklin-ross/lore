// Tiny createElement helper. Skips null children, supports class/style/onclick
// shortcuts, and handles a `data` map for dataset assignment. All page
// renderers go through this so attribute application stays consistent.
export function el(tag, attrs, ...children) {
  const node = document.createElement(tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (v == null) continue;
      if (k === "style") Object.assign(node.style, v);
      else if (k === "class") node.className = v;
      else if (k === "onclick") node.onclick = v;
      else if (k === "data") for (const [dk, dv] of Object.entries(v)) node.dataset[dk] = dv;
      else node.setAttribute(k, v);
    }
  }
  for (const c of children) {
    if (c == null) continue;
    if (Array.isArray(c)) {
      for (const cc of c) {
        if (cc == null) continue;
        node.appendChild(typeof cc === "string" ? document.createTextNode(cc) : cc);
      }
    } else if (typeof c === "string") {
      node.appendChild(document.createTextNode(c));
    } else {
      node.appendChild(c);
    }
  }
  return node;
}

// section renders a collapsible `<section>` with an h2 header. `collapsed` is
// the Set of section titles that should start collapsed; clicks toggle and
// notify `onToggle(title, isCollapsed)` so callers can persist state.
export function section(title, collapsed, onToggle, ...body) {
  const isCollapsed = collapsed.has(title);
  const head = el("h2", {
    class: "collapsible" + (isCollapsed ? " collapsed" : ""),
  }, title);
  const wrap = el("div", null);
  for (const c of body) {
    if (c == null) continue;
    if (Array.isArray(c)) for (const cc of c) { if (cc) wrap.appendChild(cc); }
    else wrap.appendChild(c);
  }
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

export function basename(uri) {
  try {
    const path = new URL(uri).pathname;
    const idx = path.lastIndexOf("/");
    return idx >= 0 ? path.slice(idx + 1) : path;
  } catch {
    return uri;
  }
}

export function locLine(loc) {
  return (loc?.range?.start?.line ?? 0) + 1;
}

// aliasSpans returns one `<span>` per alias, each prefixed with " · " so it
// sits inline next to the canonical name in a header. Returns an empty array
// when there are no aliases. baseColour, when set, paints the aliases in the
// same palette colour as the entity name so the relationship is visible
// without relying on a separate label like "Also known as".
export function aliasSpans(aliases, baseColour) {
  if (!aliases || !aliases.length) return [];
  const out = [];
  for (const a of aliases) {
    out.push(el(
      "span",
      { class: "alias-inline", style: baseColour ? { color: baseColour } : undefined },
      " · " + a
    ));
  }
  return out;
}
