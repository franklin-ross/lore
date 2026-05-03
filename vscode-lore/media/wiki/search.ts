import { el } from "./dom.ts";

export interface CatalogEntity {
  name: string;
  type: string;
}
export interface CatalogType {
  name: string;
  count: number;
}
export interface Catalog {
  entities: CatalogEntity[];
  types: CatalogType[];
}

export interface Suggestion {
  kind: "entity" | "type";
  label: string;
  qual: string;
  score: number;
}

export interface SearchActions {
  openEntity(entity: string): void;
  openType(type: string): void;
}

export interface SearchController {
  setCatalog(c: Catalog | undefined): void;
  attach(input: HTMLInputElement, results: HTMLElement): void;
  detach(input: HTMLInputElement): void;
  pruneDetached(): void;
  // exposed for tests
  _query(): string;
  _suggestions(): Suggestion[];
  _setQueryForTest(v: string): void;
  _commitForTest(s: Suggestion): void;
}

// SearchController owns the autocomplete state for one wiki session: the
// catalog, the active query, the suggestion list, and which result is
// currently highlighted. Multiple input/results pairs can be attached and
// they all stay in sync — the toolbar always shows one, and the home page
// adds a second prominent one when it's mounted.
export function createSearchController(actions: SearchActions): SearchController {
  let catalog: Catalog = { entities: [], types: [] };
  let query = "";
  let suggestions: Suggestion[] = [];
  let active = -1;
  const attached: { input: HTMLInputElement; results: HTMLElement }[] = [];
  // The dropdown to render into; only one shows at a time so typing in
  // one input doesn't open a duplicate menu under any other attached input.
  let activeResults: HTMLElement | null = null;

  function compute(): void {
    const q = query.trim().toLowerCase();
    if (!q) {
      suggestions = [];
      active = -1;
      return;
    }
    const matches: Suggestion[] = [];
    for (const e of catalog.entities) {
      if (e.name.toLowerCase().includes(q)) {
        matches.push({
          kind: "entity",
          label: e.name,
          qual: e.type,
          score: rank(e.name, q),
        });
      }
    }
    for (const t of catalog.types) {
      const name = t.name;
      if (name.toLowerCase().includes(q)) {
        matches.push({
          kind: "type",
          label: name,
          qual: t.count + (t.count === 1 ? " entity" : " entities"),
          score: rank(name, q),
        });
      }
    }
    matches.sort((a, b) => a.score - b.score || a.label.localeCompare(b.label));
    suggestions = matches.slice(0, 12);
    active = suggestions.length ? 0 : -1;
  }

  function paint(): void {
    // Only the active dropdown renders results; close every other so the
    // user only sees one menu at a time.
    for (const { results } of attached) {
      if (results !== activeResults) results.classList.remove("open");
    }
    if (activeResults) paintInto(activeResults);
  }

  function paintInto(results: HTMLElement): void {
    results.innerHTML = "";
    if (!suggestions.length) {
      results.classList.remove("open");
      return;
    }
    suggestions.forEach((s, i) => {
      const row = el(
        "div",
        {
          class: "search-row" + (i === active ? " active" : ""),
          onclick: () => commit(s),
        },
        el("span", { class: "kind" }, s.kind),
        el("span", null, s.label),
        s.qual ? el("span", { class: "qual" }, s.qual) : null,
      );
      results.appendChild(row);
    });
    results.classList.add("open");
  }

  function commit(s: Suggestion | undefined): void {
    if (!s) return;
    if (s.kind === "entity") actions.openEntity(s.label);
    else if (s.kind === "type") actions.openType(s.label);
    setQuery("");
    for (const { input } of attached) input.value = "";
  }

  function setQuery(v: string): void {
    query = v;
    compute();
    paint();
  }

  function keyDown(e: KeyboardEvent): void {
    if (e.key === "ArrowDown") {
      if (suggestions.length) {
        active = (active + 1) % suggestions.length;
        paint();
        e.preventDefault();
      }
    } else if (e.key === "ArrowUp") {
      if (suggestions.length) {
        active = (active - 1 + suggestions.length) % suggestions.length;
        paint();
        e.preventDefault();
      }
    } else if (e.key === "Enter") {
      if (active >= 0 && suggestions[active]) {
        commit(suggestions[active]);
        e.preventDefault();
      }
    } else if (e.key === "Escape") {
      setQuery("");
      for (const { input } of attached) input.value = "";
    }
  }

  return {
    setCatalog(c) {
      catalog = c || { entities: [], types: [] };
      compute();
      paint();
    },
    attach(input, results) {
      attached.push({ input, results });
      input.value = query;
      input.addEventListener("input", (e) => {
        const target = e.target as HTMLInputElement;
        activeResults = results;
        setQuery(target.value);
        for (const a of attached) {
          if (a.input !== target) a.input.value = target.value;
        }
      });
      input.addEventListener("focus", () => {
        activeResults = results;
        if (query) paint();
      });
      input.addEventListener("keydown", keyDown);
      input.addEventListener("blur", () => {
        // Defer so a click on a result row commits before the dropdown vanishes.
        setTimeout(() => {
          results.classList.remove("open");
          if (activeResults === results) activeResults = null;
        }, 120);
      });
    },
    detach(input) {
      const idx = attached.findIndex((a) => a.input === input);
      if (idx >= 0) attached.splice(idx, 1);
    },
    // Drop any inputs whose DOM node is no longer connected. Called after
    // re-render to stop dead nodes from receiving paint() updates.
    pruneDetached() {
      for (let i = attached.length - 1; i >= 0; i--) {
        if (!attached[i]!.input.isConnected) attached.splice(i, 1);
      }
    },
    _query: () => query,
    _suggestions: () => suggestions.slice(),
    _setQueryForTest: setQuery,
    _commitForTest: commit,
  };
}

function rank(name: string, q: string): number {
  const lower = name.toLowerCase();
  if (lower === q) return 0;
  if (lower.startsWith(q)) return 1;
  return 2;
}
