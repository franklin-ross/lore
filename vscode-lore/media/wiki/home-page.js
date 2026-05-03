import { el } from "./dom.js";

// renderHomePage shows a centred search box plus chips for every type in
// the catalog. The search input attaches to the shared search controller
// so suggestions render directly underneath it (and stay in sync with the
// toolbar input).
export function renderHomePage(catalog, ctx) {
  const wrap = el("div", { class: "home" });
  wrap.appendChild(el("h1", null, "Lore Wiki"));

  const input = el("input", {
    type: "text",
    class: "search-input home-search",
    placeholder: "Search entities and types…",
  });
  const results = el("div", { class: "search-results" });
  const searchWrap = el("div", { class: "search-wrap" }, input, results);
  ctx.search.attach(input, results);
  wrap.appendChild(searchWrap);

  wrap.appendChild(el("div", { class: "hint" },
    "Type a name to jump straight in, or pick a type below."));

  if (catalog && catalog.types && catalog.types.length) {
    const types = el("div", { class: "types" });
    // Catalog already sorts types by entity count descending; render in order.
    for (const t of catalog.types) {
      types.appendChild(el("span", {
        class: "type-pill",
        onclick: () => ctx.openType(t.name),
      },
        t.name,
        el("span", { class: "type-count" }, String(t.count))
      ));
    }
    wrap.appendChild(types);
  }

  // Defer focus so it lands after VSCode finishes painting.
  requestAnimationFrame(() => input.focus());
  return [wrap];
}
