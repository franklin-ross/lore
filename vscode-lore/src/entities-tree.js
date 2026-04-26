import * as vscode from "vscode";

// LoreEntitiesProvider feeds the "Lore Entities" tree view in the explorer
// sidebar. Top level groups entities by type ("character", "location", ...);
// leaves are entity names that, when clicked, open the source file at the
// definition span. Data comes from the lore/entityList custom request so we
// can render resolved tags and filter against them.
//
// A free-text filter (set via the title-bar Search action) matches entities
// whose name, type, alias, or tag contains the query as a case-insensitive
// substring. Types with zero matches collapse out of the tree entirely.
export class LoreEntitiesProvider {
  constructor(getClient) {
    this._getClient = getClient;
    this._onDidChangeTreeData = new vscode.EventEmitter();
    this.onDidChangeTreeData = this._onDidChangeTreeData.event;
    this._filter = "";
  }

  refresh() {
    this._onDidChangeTreeData.fire(undefined);
  }

  setFilter(query) {
    const next = (query ?? "").trim();
    if (next === this._filter) return;
    this._filter = next;
    this.refresh();
  }

  getFilter() {
    return this._filter;
  }

  getTreeItem(element) {
    return element;
  }

  // resolveTreeItem is called by VSCode just before a tooltip is rendered, so
  // we can fetch the rich hover markdown from the language server lazily —
  // one request per hover instead of one per entity at refresh time. Falls
  // back to the static tag-list tooltip set in getChildren if anything
  // fails or the cancellation token fires.
  async resolveTreeItem(item, element, token) {
    if (!element || !element._loreLocation) return item;
    const client = this._getClient();
    if (!client) return item;

    let hover;
    try {
      hover = await client.sendRequest(
        "textDocument/hover",
        {
          textDocument: { uri: element._loreLocation.uri },
          position: element._loreLocation.position,
        },
        token
      );
    } catch {
      return item;
    }
    if (!hover || token?.isCancellationRequested) return item;

    const text = hoverContentsToMarkdown(hover.contents);
    if (!text) return item;
    const md = new vscode.MarkdownString(text);
    md.supportHtml = true;
    md.isTrusted = true;
    item.tooltip = md;
    return item;
  }

  async getChildren(element) {
    const client = this._getClient();
    if (!client) return [];

    // Scope the request to the active editor's project, so each lore.toml
    // gets its own tree view instead of one merged blob across campaigns.
    const activeUri = vscode.window.activeTextEditor?.document?.uri?.toString();
    const params = activeUri
      ? { textDocument: { uri: activeUri } }
      : {};
    let response;
    try {
      response = await client.sendRequest("lore/entityList", params);
    } catch {
      return [];
    }
    const all = response?.entities ?? [];
    const entities = applyFilter(all, this._filter);

    if (!element) {
      const counts = new Map();
      for (const e of entities) {
        const type = e.type || "(untyped)";
        counts.set(type, (counts.get(type) || 0) + 1);
      }
      const types = [...counts.keys()].sort((a, b) => a.localeCompare(b));
      // When a filter is active, expand everything so matches are visible
      // without an extra click.
      const collapseState = this._filter
        ? vscode.TreeItemCollapsibleState.Expanded
        : vscode.TreeItemCollapsibleState.Collapsed;
      return types.map((type) => {
        const item = new vscode.TreeItem(type, collapseState);
        item.description = String(counts.get(type));
        item.iconPath = new vscode.ThemeIcon("symbol-namespace");
        item.contextValue = "loreEntityType";
        item._loreType = type;
        return item;
      });
    }

    const type = element._loreType;
    const matched = entities
      .filter((e) => (e.type || "(untyped)") === type)
      .sort((a, b) => a.name.localeCompare(b.name));

    return matched.map((e) => {
      const uri = vscode.Uri.parse(e.location.uri);
      const range = new vscode.Range(
        e.location.range.start.line,
        e.location.range.start.character,
        e.location.range.end.line,
        e.location.range.end.character
      );
      const item = new vscode.TreeItem(
        e.name,
        vscode.TreeItemCollapsibleState.None
      );
      item.iconPath = new vscode.ThemeIcon("symbol-object");
      const tags = e.tags ?? [];
      if (tags.length > 0) {
        item.description = tags.map((t) => `+${t}`).join(" ");
      }
      // Leave item.tooltip undefined — VSCode only calls resolveTreeItem for
      // properties that are undefined, so we'd skip the lazy hover fetch
      // entirely if we set a placeholder here.
      item.command = {
        command: "vscode.open",
        title: "Open Entity",
        arguments: [uri, { selection: range, preview: true }],
      };
      item.contextValue = "loreEntity";
      item._loreLocation = {
        uri: e.location.uri,
        position: {
          line: e.location.range.start.line,
          character: e.location.range.start.character,
        },
      };
      return item;
    });
  }
}

// hoverContentsToMarkdown flattens an LSP Hover.contents value (string |
// MarkedString | MarkedString[] | MarkupContent) into a single markdown
// string suitable for a TreeItem tooltip. Returns "" if the structure is
// unrecognised so the caller can fall back to its placeholder.
function hoverContentsToMarkdown(contents) {
  if (!contents) return "";
  if (typeof contents === "string") return contents;
  if (Array.isArray(contents)) {
    return contents.map(markedStringToMarkdown).filter(Boolean).join("\n\n");
  }
  if (typeof contents === "object") {
    if ("value" in contents && typeof contents.value === "string") {
      return contents.value;
    }
    return markedStringToMarkdown(contents);
  }
  return "";
}

function markedStringToMarkdown(item) {
  if (typeof item === "string") return item;
  if (item && typeof item === "object") {
    if (typeof item.value === "string") {
      if (typeof item.language === "string" && item.language) {
        return "```" + item.language + "\n" + item.value + "\n```";
      }
      return item.value;
    }
  }
  return "";
}

// applyFilter keeps an entity if any of its searchable fields contains query
// as a case-insensitive substring. Empty query passes everything through.
// Searchable fields: canonical name, type, every alias, every tag.
function applyFilter(entities, query) {
  if (!query) return entities;
  const needle = query.toLowerCase();
  return entities.filter((e) => {
    if (e.name && e.name.toLowerCase().includes(needle)) return true;
    if (e.type && e.type.toLowerCase().includes(needle)) return true;
    for (const alias of e.aliases ?? []) {
      if (alias.toLowerCase().includes(needle)) return true;
    }
    for (const tag of e.tags ?? []) {
      if (tag.toLowerCase().includes(needle)) return true;
    }
    return false;
  });
}
