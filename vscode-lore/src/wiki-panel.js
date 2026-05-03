import * as vscode from "vscode";

/**
 * @typedef {{kind: "entity", value: string, source: string | undefined}
 *         | {kind: "type", value: string, source: string | undefined}
 *         | {kind: "home", value: "", source: string | undefined}} Page
 */

/**
 * Pages a wiki shows: entity (one entity), type (every entity of a type),
 * home (landing page with search). LoreWikiPanel owns the singleton webview,
 * a navigation history stack, and the back/forward cursor.
 *
 * Webview content lives in vscode-lore/media/wiki/ as ESM modules. The
 * extension only owns transport: it fetches data from the LSP server, posts
 * it to the webview, and routes user-driven messages back into navigation.
 */
export class LoreWikiPanel {
  /**
   * @param {() => import("vscode-languageclient/node").LanguageClient | undefined} getClient
   * @param {string[]} palette
   * @param {vscode.ExtensionContext} context
   */
  constructor(getClient, palette, context) {
    this.getClient = getClient;
    this.palette = palette;
    this.context = context;
    /** @type {vscode.WebviewPanel | undefined} */
    this.panel = undefined;
    /** @type {Page[]} */
    this.history = [];
    this.cursor = -1;
  }

  /** @returns {Page | undefined} */
  current() {
    return this.cursor >= 0 ? this.history[this.cursor] : undefined;
  }

  /**
   * Open or reveal the wiki for an entity. Equivalent to navigating to an
   * entity page; kept as a named method for older call sites.
   * @param {string} entity
   * @param {string | undefined} source
   */
  async show(entity, source) {
    await this.navigate({ kind: "entity", value: entity, source });
  }

  /** @param {string | undefined} source */
  async showHome(source) {
    await this.navigate({ kind: "home", value: "", source });
  }

  /**
   * @param {string} type
   * @param {string | undefined} source
   */
  async showType(type, source) {
    await this.navigate({ kind: "type", value: type, source });
  }

  /**
   * Push a page onto the history stack. If the user has rewound with
   * back(), forward entries past the cursor are discarded — same behaviour
   * as a browser. Re-navigating to the page already at the cursor doesn't
   * grow the stack but still triggers a refresh.
   * @param {Page} page
   */
  async navigate(page) {
    this.ensurePanel();
    const top = this.current();
    if (!samePage(top, page)) {
      this.history.splice(this.cursor + 1);
      this.history.push(page);
      this.cursor = this.history.length - 1;
    }
    this.updateTitle();
    await this.refresh();
  }

  async back() {
    if (this.cursor <= 0) return;
    this.cursor--;
    this.updateTitle();
    await this.refresh();
  }

  async forward() {
    if (this.cursor >= this.history.length - 1) return;
    this.cursor++;
    this.updateTitle();
    await this.refresh();
  }

  /**
   * Jump to an arbitrary point in the history stack — used by breadcrumb
   * clicks. Out-of-range indices are ignored.
   * @param {number} index
   */
  async goto(index) {
    if (index < 0 || index >= this.history.length || index === this.cursor) return;
    this.cursor = index;
    this.updateTitle();
    await this.refresh();
  }

  /**
   * Re-fetch the current page's data and push it into the webview together
   * with a snapshot of breadcrumbs and the autocomplete catalog. Called on
   * navigate/back/forward and from the extension's debounced edit listener
   * so the wiki stays current.
   */
  async refresh() {
    if (!this.panel) return;
    const page = this.current();
    if (!page) return;

    const client = this.getClient();
    if (!client) {
      this.panel.webview.postMessage({ type: "error", message: "Language server not running." });
      return;
    }

    let payload = null;
    let catalog = null;
    try {
      [payload, catalog] = await Promise.all([
        this.fetchPayload(client, page),
        this.fetchCatalog(client, page.source),
      ]);
    } catch (err) {
      this.panel.webview.postMessage({
        type: "error",
        message: (err && err.message) || String(err),
      });
      return;
    }

    this.panel.webview.postMessage({
      type: "page",
      page,
      payload,
      catalog,
      palette: this.palette,
      breadcrumbs: this.breadcrumbsSnapshot(),
      cursor: this.cursor,
      canBack: this.cursor > 0,
      canForward: this.cursor < this.history.length - 1,
    });
  }

  /**
   * @param {import("vscode-languageclient/node").LanguageClient} client
   * @param {Page} page
   */
  async fetchPayload(client, page) {
    const td = page.source ? { uri: page.source } : undefined;
    if (page.kind === "entity") {
      return client.sendRequest("lore/entityDetails", {
        entity: page.value,
        textDocument: td,
      });
    }
    if (page.kind === "type") {
      return client.sendRequest("lore/typeDetails", {
        type: page.value,
        textDocument: td,
      });
    }
    return null; // home — no per-page payload
  }

  /**
   * Fetches the entity catalog used by the search box. Same source scoping
   * as page lookups so suggestions match the active project.
   * @param {import("vscode-languageclient/node").LanguageClient} client
   * @param {string | undefined} source
   */
  async fetchCatalog(client, source) {
    const td = source ? { uri: source } : undefined;
    const list = await client.sendRequest("lore/entityList", { textDocument: td });
    const entities = (list?.entities || []).map((e) => ({
      name: e.name,
      type: e.type,
    }));
    const counts = new Map();
    for (const e of entities) {
      if (!e.type) continue;
      counts.set(e.type, (counts.get(e.type) || 0) + 1);
    }
    const types = [...counts.entries()].map(([name, count]) => ({ name, count }));
    // Most populous type first; alphabetical tie-break so identical counts
    // render in a stable order.
    types.sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));
    return { entities, types };
  }

  /** Snapshot of the history stack for breadcrumb rendering. */
  breadcrumbsSnapshot() {
    return this.history.map((p) => ({ kind: p.kind, value: p.value }));
  }

  ensurePanel() {
    if (this.panel) {
      this.panel.reveal(undefined, true);
      return;
    }
    const mediaRoot = vscode.Uri.joinPath(this.context.extensionUri, "media");
    this.panel = vscode.window.createWebviewPanel(
      "loreWiki",
      "Lore Wiki",
      { viewColumn: vscode.ViewColumn.Beside, preserveFocus: true },
      {
        enableScripts: true,
        retainContextWhenHidden: true,
        localResourceRoots: [mediaRoot],
      }
    );
    this.panel.webview.html = this.renderShell();
    this.panel.webview.onDidReceiveMessage((msg) => this.onMessage(msg));
    this.panel.onDidDispose(() => {
      this.panel = undefined;
      this.history = [];
      this.cursor = -1;
    });
  }

  updateTitle() {
    if (!this.panel) return;
    const page = this.current();
    if (!page || page.kind === "home") {
      this.panel.title = "Lore Wiki";
    } else if (page.kind === "type") {
      this.panel.title = `Lore: ${page.value} (type)`;
    } else {
      this.panel.title = `Lore: ${page.value}`;
    }
  }

  /** @param {{type: string, [k: string]: any}} msg */
  async onMessage(msg) {
    switch (msg.type) {
      case "ready":
        await this.refresh();
        return;
      case "navigate":
        if (msg.uri) await this.openInEditor(msg.uri, msg.line);
        return;
      case "openEntity":
        if (msg.entity) await this.show(msg.entity, this.current()?.source);
        return;
      case "openType":
        if (msg.value) await this.showType(msg.value, this.current()?.source);
        return;
      case "openHome":
        await this.showHome(this.current()?.source);
        return;
      case "back":
        await this.back();
        return;
      case "forward":
        await this.forward();
        return;
      case "goto":
        if (typeof msg.index === "number") await this.goto(msg.index);
        return;
    }
  }

  /** @param {string} uri @param {number | undefined} line */
  async openInEditor(uri, line) {
    try {
      const parsed = vscode.Uri.parse(uri);
      const doc = await vscode.workspace.openTextDocument(parsed);
      const lineIdx = Math.max(0, (line ?? 1) - 1);
      const lineText = doc.lineAt(Math.min(lineIdx, doc.lineCount - 1));
      await vscode.window.showTextDocument(doc, {
        selection: lineText.range,
        viewColumn: vscode.ViewColumn.One,
        preserveFocus: false,
      });
    } catch (err) {
      vscode.window.showErrorMessage(
        `Lore: failed to open ${uri}: ${(err && err.message) || err}`
      );
    }
  }

  renderShell() {
    const webview = this.panel.webview;
    const mediaUri = (rel) =>
      webview.asWebviewUri(
        vscode.Uri.joinPath(this.context.extensionUri, "media", "wiki", rel)
      );
    const main = mediaUri("main.js");
    const styles = mediaUri("wiki.css");
    const nonce = makeNonce();
    const csp = [
      "default-src 'none'",
      `style-src ${webview.cspSource} 'unsafe-inline'`,
      `font-src ${webview.cspSource}`,
      `img-src ${webview.cspSource} https: data:`,
      `script-src 'nonce-${nonce}' ${webview.cspSource}`,
    ].join("; ");
    return `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta http-equiv="Content-Security-Policy" content="${csp}">
<link rel="stylesheet" href="${styles}">
</head>
<body>
<header id="toolbar"></header>
<main id="root"><p class="empty">Loading…</p></main>
<script type="module" nonce="${nonce}" src="${main}"></script>
</body>
</html>`;
  }
}

/**
 * @param {Page | undefined} a
 * @param {Page} b
 */
function samePage(a, b) {
  if (!a) return false;
  return a.kind === b.kind && a.value === b.value;
}

function makeNonce() {
  const bytes = new Uint8Array(16);
  for (let i = 0; i < 16; i++) bytes[i] = Math.floor(Math.random() * 256);
  return [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");
}
