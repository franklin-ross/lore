import { el } from "./dom.js";

// mountToolbar wires the back/forward buttons, breadcrumbs, and the
// toolbar's search input. The search controller is owned by main.js and
// passed in so the home page can attach its own input to the same state.
export function mountToolbar(host, ctx, search) {
  host.innerHTML = "";

  const back = el("button", {
    class: "nav-btn",
    title: "Back",
    onclick: () => ctx.back(),
  }, "←");
  const forward = el("button", {
    class: "nav-btn",
    title: "Forward",
    onclick: () => ctx.forward(),
  }, "→");
  const home = el("button", {
    class: "home-btn",
    title: "Home",
    onclick: () => ctx.openHome(),
  }, "⌂");

  const crumbs = el("nav", { class: "breadcrumbs" });

  const searchInput = el("input", {
    type: "text",
    class: "search-input",
    placeholder: "Search entities and types…",
  });
  const searchResults = el("div", { class: "search-results" });
  const searchWrap = el("div", { class: "search-wrap" }, searchInput, searchResults);
  search.attach(searchInput, searchResults);

  host.appendChild(back);
  host.appendChild(forward);
  host.appendChild(home);
  host.appendChild(crumbs);
  host.appendChild(searchWrap);

  return {
    setBreadcrumbs(items, cursor) {
      crumbs.innerHTML = "";
      const visible = items.slice(Math.max(0, items.length - 5));
      const offset = items.length - visible.length;
      visible.forEach((p, idx) => {
        if (idx > 0) crumbs.appendChild(el("span", { class: "sep" }, "›"));
        const histIdx = offset + idx;
        const isCurrent = histIdx === cursor;
        crumbs.appendChild(el("span", {
          class: "crumb" + (isCurrent ? " current" : ""),
          onclick: isCurrent ? null : () => ctx.openHistoryIndex(histIdx),
        }, labelFor(p)));
      });
    },
    setNav(canBack, canForward) {
      back.disabled = !canBack;
      forward.disabled = !canForward;
    },
    focusSearch() {
      searchInput.focus();
    },
  };
}

function labelFor(p) {
  if (p.kind === "home") return "Home";
  if (p.kind === "type") return p.value + " (type)";
  return p.value;
}
