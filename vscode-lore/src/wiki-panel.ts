import * as vscode from "vscode";
import type { LanguageClient } from "vscode-languageclient/node";
import type { EntityFocusBus } from "./entity-focus-bus.ts";

export type WikiPageKind = "entity" | "type" | "home";

export interface WikiPage {
  kind: WikiPageKind;
  value: string;
  source: string | undefined;
}

interface CatalogEntity { name: string; type: string }
interface CatalogType { name: string; count: number }
interface Catalog { entities: CatalogEntity[]; types: CatalogType[] }

interface EntityListResponse {
  entities?: { name: string; type: string }[];
  // Server signals "no scope" (e.g. workspace has no lore.toml). When set,
  // the wiki should render the message in place of an empty home page.
  message?: string;
}

interface GraphNode { label: string; name: string; type?: string; colourIndex: number }
interface GraphDefEdge { from: string; to: string; count: number }
interface GraphRelationEdge { from: string; to: string; label: string; symmetric?: boolean }
interface GraphResponse {
  nodes?: GraphNode[];
  defEdges?: GraphDefEdge[];
  relationEdges?: GraphRelationEdge[];
}

interface LspRange {
  start: { line: number; character: number };
  end: { line: number; character: number };
}

interface IncomingMessage {
  type: string;
  uri?: string;
  line?: number;
  range?: LspRange;
  entity?: string;
  value?: string;
  index?: number;
  scrollToGraph?: boolean;
}

/**
 * Pages a wiki shows: entity (one entity), type (every entity of a type),
 * home (landing page with search). LoreWikiPanel owns the singleton webview,
 * a navigation history stack, and the back/forward cursor.
 *
 * Webview content lives in vscode-lore/media/wiki/ as TS modules, bundled by
 * esbuild into out/wiki/main.js. The extension only owns transport: it fetches
 * data from the LSP server, posts it to the webview, and routes user-driven
 * messages back into navigation.
 */
export class LoreWikiPanel {
  private panel: vscode.WebviewPanel | undefined;
  private history: WikiPage[] = [];
  private cursor = -1;
  private busSub: vscode.Disposable | undefined;

  // pendingScrollToGraph: when set, the next refresh tells the webview to
  // scroll the embedded graph into view instead of restoring saved position.
  // Set by the openEntity handler when the click came from a node in the
  // wiki's own embedded graph (so the user keeps the graph in sight while
  // browsing through it).
  private pendingScrollToGraph = false;

  constructor(
    private readonly getClient: () => LanguageClient | undefined,
    private readonly palette: string[],
    private readonly context: vscode.ExtensionContext,
    private readonly bus: EntityFocusBus,
  ) {}

  current(): WikiPage | undefined {
    return this.cursor >= 0 ? this.history[this.cursor] : undefined;
  }

  // Open or reveal the wiki for an entity. Equivalent to navigating to an
  // entity page; kept as a named method for older call sites.
  // reveal controls whether an already-open panel is brought to the front.
  // Graph clicks pass reveal=false: the wiki content should follow the
  // selection, but bringing its tab forward would steal focus from the graph.
  async show(entity: string, source: string | undefined, reveal = true): Promise<void> {
    await this.navigate({ kind: "entity", value: entity, source }, reveal);
  }

  async showHome(source: string | undefined): Promise<void> {
    await this.navigate({ kind: "home", value: "", source });
  }

  async showType(type: string, source: string | undefined): Promise<void> {
    await this.navigate({ kind: "type", value: type, source });
  }

  // showWord asks the server to classify `word` as an entity, type, or
  // miss before navigating. Used by F12-at-cursor where the cursor word
  // could be either the entity name or its inline `(type)` label.
  // `offset` is the cursor's byte offset within `word`; when the word range
  // spans several entities the server uses it to pick the one at the cursor.
  async showWord(
    word: string,
    source: string | undefined,
    offset?: number,
  ): Promise<void> {
    const client = this.getClient();
    if (!client) {
      await this.show(word, source);
      return;
    }
    let result: { kind?: string; value?: string } | undefined;
    try {
      result = await client.sendRequest("lore/lookup", {
        name: word,
        offset,
        textDocument: source ? { uri: source } : undefined,
      });
    } catch {
      // Fall through to entity show — the panel will surface the not-found
      // message rather than silently swallow the click.
    }
    if (result?.kind === "type" && result.value) {
      await this.showType(result.value, source);
    } else if (result?.kind === "entity" && result.value) {
      await this.show(result.value, source);
    } else {
      await this.show(word, source);
    }
  }

  // Push a page onto the history stack. If the user has rewound with
  // back(), forward entries past the cursor are discarded — same behaviour
  // as a browser. Re-navigating to the page already at the cursor doesn't
  // grow the stack but still triggers a refresh.
  async navigate(page: WikiPage, reveal = true): Promise<void> {
    this.ensurePanel(reveal);
    const top = this.current();
    if (!samePage(top, page)) {
      this.history.splice(this.cursor + 1);
      this.history.push(page);
      this.cursor = this.history.length - 1;
    }
    this.updateTitle();
    await this.refresh();
  }

  async back(): Promise<void> {
    if (this.cursor <= 0) return;
    this.cursor--;
    this.updateTitle();
    await this.refresh();
  }

  async forward(): Promise<void> {
    if (this.cursor >= this.history.length - 1) return;
    this.cursor++;
    this.updateTitle();
    await this.refresh();
  }

  // Jump to an arbitrary point in the history stack — used by breadcrumb
  // clicks. Out-of-range indices are ignored.
  async goto(index: number): Promise<void> {
    if (index < 0 || index >= this.history.length || index === this.cursor) return;
    this.cursor = index;
    this.updateTitle();
    await this.refresh();
  }

  // Re-fetch the current page's data and push it into the webview together
  // with a snapshot of breadcrumbs and the autocomplete catalog. Called on
  // navigate/back/forward and from the extension's debounced edit listener
  // so the wiki stays current.
  async refresh(): Promise<void> {
    if (!this.panel) return;
    const page = this.current();
    if (!page) return;

    const client = this.getClient();
    if (!client) {
      this.panel.webview.postMessage({ type: "error", message: "Language server not running." });
      return;
    }

    let payload: unknown = null;
    let catalogResult: { catalog: Catalog; message?: string } = { catalog: { entities: [], types: [] } };
    let graph: GraphResponse | null = null;
    try {
      [payload, catalogResult, graph] = await Promise.all([
        this.fetchPayload(client, page),
        this.fetchCatalog(client, page.source),
        // Embedded entity graph only renders on entity pages — skip the
        // round-trip on home/type pages where the section is hidden.
        page.kind === "entity" ? this.fetchGraph(client, page.source) : Promise.resolve(null),
      ]);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.panel.webview.postMessage({ type: "error", message });
      return;
    }
    const catalog = catalogResult.catalog;

    // No project to scope to — render the server-supplied placeholder in
    // place of an empty search box. Skip the rest of the page so toolbar,
    // graph, and history don't appear functional.
    if (catalogResult.message) {
      this.panel.webview.postMessage({
        type: "info",
        message: catalogResult.message,
      });
      return;
    }

    const scrollToGraph = this.pendingScrollToGraph;
    this.pendingScrollToGraph = false;

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
      graph,
      scrollToGraph,
    });

    // Broadcast focus so the graph panel (when open) can re-centre on the
    // active entity without a separate request roundtrip.
    if (page.kind === "entity") {
      this.bus.fire({ entity: page.value, source: page.source, origin: "wiki" });
    }
  }

  private async fetchPayload(client: LanguageClient, page: WikiPage): Promise<unknown> {
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

  // fetchGraph pulls the full project graph for the current source. The
  // webview filters it down to the focus entity's depth-1 neighbourhood
  // (plus the next hop, faded) before rendering — keeping the filter
  // client-side avoids a separate LSP request shape and lets graph
  // updates flow through the same edit-debounce path.
  private async fetchGraph(client: LanguageClient, source: string | undefined): Promise<GraphResponse | null> {
    const td = source ? { uri: source } : undefined;
    try {
      return await client.sendRequest<GraphResponse>("lore/graph", { textDocument: td });
    } catch {
      return null;
    }
  }

  // Fetches the entity catalog used by the search box. Same source scoping
  // as page lookups so suggestions match the active project. Returns the
  // server-supplied "no scope" message alongside the catalog so callers
  // can short-circuit rendering when there's no project.
  private async fetchCatalog(
    client: LanguageClient,
    source: string | undefined,
  ): Promise<{ catalog: Catalog; message?: string }> {
    const td = source ? { uri: source } : undefined;
    const list = await client.sendRequest<EntityListResponse>("lore/entityList", { textDocument: td });
    const entities: CatalogEntity[] = (list?.entities ?? []).map((e) => ({
      name: e.name,
      type: e.type,
    }));
    const counts = new Map<string, number>();
    for (const e of entities) {
      if (!e.type) continue;
      counts.set(e.type, (counts.get(e.type) ?? 0) + 1);
    }
    const types: CatalogType[] = [...counts.entries()].map(([name, count]) => ({ name, count }));
    // Most populous type first; alphabetical tie-break so identical counts
    // render in a stable order.
    types.sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));
    return { catalog: { entities, types }, message: list?.message };
  }

  // Snapshot of the history stack for breadcrumb rendering. Includes
  // `source` so the webview can persist it via setState — the panel
  // serializer reads it back on editor restart to restore the last page.
  private breadcrumbsSnapshot(): WikiPage[] {
    return this.history.map((p) => ({ ...p }));
  }

  private ensurePanel(reveal = true): void {
    if (this.panel) {
      // reveal=false: leave the panel where it is (possibly behind the graph
      // tab) and just let refresh update its content — no tab switch, no
      // focus change. preserveFocus on reveal isn't enough on its own,
      // because bringing the tab forward in a shared column hides the graph.
      if (reveal) this.panel.reveal(undefined, true);
      return;
    }
    const webviewRoot = vscode.Uri.joinPath(this.context.extensionUri, "out", "wiki");
    this.panel = vscode.window.createWebviewPanel(
      "loreWiki",
      "Lore Wiki",
      { viewColumn: vscode.ViewColumn.Beside, preserveFocus: true },
      {
        enableScripts: true,
        retainContextWhenHidden: true,
        localResourceRoots: [webviewRoot],
      },
    );
    this.attach(this.panel);
  }

  // attach wires up message + dispose handlers and (re)installs the
  // webview HTML. Used both by initial creation and by the panel
  // serializer when VSCode hands back a panel after a restart.
  private attach(panel: vscode.WebviewPanel): void {
    this.panel = panel;
    const webviewRoot = vscode.Uri.joinPath(this.context.extensionUri, "out", "wiki");
    panel.webview.options = {
      enableScripts: true,
      localResourceRoots: [webviewRoot],
    };
    panel.webview.html = this.renderShell();
    panel.webview.onDidReceiveMessage((msg: IncomingMessage) => this.onMessage(msg));
    panel.onDidDispose(() => {
      this.panel = undefined;
      this.history = [];
      this.cursor = -1;
      this.busSub?.dispose();
      this.busSub = undefined;
    });
    // React to focus events from elsewhere (graph panel today, others
    // later). Skip our own emissions to avoid feedback when the user
    // navigates inside the wiki itself.
    this.busSub?.dispose();
    this.busSub = this.bus.onDidFocus((focus) => {
      if (focus.origin === "wiki") return;
      // Graph (and other external) focus updates the content but must not
      // pull the wiki tab forward — the user is working in the graph.
      void this.show(focus.entity, focus.source, false);
    });
  }

  // restore is the WebviewPanelSerializer entry point. VSCode hands back
  // the previously-persisted panel along with the webview's last setState
  // value; we hydrate the navigation history from that and let the
  // refresh on `ready` paint the last page.
  async restore(panel: vscode.WebviewPanel, raw: unknown): Promise<void> {
    const state = (raw && typeof raw === "object") ? raw as { history?: unknown; cursor?: unknown } : {};
    const history = sanitiseHistory(state.history);
    if (history.length > 0) {
      this.history = history;
      const c = typeof state.cursor === "number" ? state.cursor : history.length - 1;
      this.cursor = c >= 0 && c < history.length ? c : history.length - 1;
    } else {
      // No persisted history — happens when the panel was open in a
      // no-project workspace last session (webview never received a
      // `page` to record). Seed a home page so refresh() has a current
      // entry to fetch, otherwise the placeholder "Loading…" sticks.
      this.history = [{ kind: "home", value: "", source: undefined }];
      this.cursor = 0;
    }
    this.attach(panel);
    this.updateTitle();
    // The webview will load main.js, post `ready`, and the resulting
    // refresh() will fetch the current page's data and paint it.
  }

  private updateTitle(): void {
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

  private async onMessage(msg: IncomingMessage): Promise<void> {
    switch (msg.type) {
      case "ready":
        await this.refresh();
        return;
      case "navigate":
        if (msg.uri) await this.openInEditor(msg.uri, msg.line, msg.range);
        return;
      case "openEntity":
        if (msg.entity) {
          if (msg.scrollToGraph) this.pendingScrollToGraph = true;
          await this.show(msg.entity, this.current()?.source);
        }
        return;
      case "openGraphHere": {
        const entity = this.current()?.kind === "entity" ? this.current()?.value : undefined;
        // Pass the page's own source explicitly: the active surface is this
        // webview, so the command's activeTextEditor fallback would yield no
        // source and the graph would render its "open a lore project" empty
        // state instead of scoping to the current entity's project.
        await vscode.commands.executeCommand("lore.openGraph", entity, this.current()?.source);
        return;
      }
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

  private async openInEditor(
    uri: string,
    line: number | undefined,
    range: LspRange | undefined,
  ): Promise<void> {
    try {
      const parsed = vscode.Uri.parse(uri);
      const doc = await vscode.workspace.openTextDocument(parsed);
      const selection = range
        ? new vscode.Range(
            range.start.line, range.start.character,
            range.end.line, range.end.character,
          )
        : (() => {
            const lineIdx = Math.max(0, (line ?? 1) - 1);
            return doc.lineAt(Math.min(lineIdx, doc.lineCount - 1)).range;
          })();
      await vscode.window.showTextDocument(doc, {
        selection,
        viewColumn: vscode.ViewColumn.One,
        preserveFocus: false,
      });
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      vscode.window.showErrorMessage(`Lore: failed to open ${uri}: ${message}`);
    }
  }

  private renderShell(): string {
    const webview = this.panel!.webview;
    const mediaUri = (rel: string): vscode.Uri =>
      webview.asWebviewUri(
        vscode.Uri.joinPath(this.context.extensionUri, "out", "wiki", rel),
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
<section id="entity-graph" class="entity-graph-section" hidden></section>
<script type="module" nonce="${nonce}" src="${main}"></script>
</body>
</html>`;
  }
}

function samePage(a: WikiPage | undefined, b: WikiPage): boolean {
  if (!a) return false;
  return a.kind === b.kind && a.value === b.value;
}

// sanitiseHistory validates a deserialised history blob. Returns [] if the
// shape doesn't match — the panel falls back to a fresh state rather than
// trusting persisted garbage from an older extension version.
function sanitiseHistory(raw: unknown): WikiPage[] {
  if (!Array.isArray(raw)) return [];
  const out: WikiPage[] = [];
  for (const item of raw) {
    if (!item || typeof item !== "object") continue;
    const p = item as { kind?: unknown; value?: unknown; source?: unknown };
    if (p.kind !== "entity" && p.kind !== "type" && p.kind !== "home") continue;
    if (typeof p.value !== "string") continue;
    out.push({
      kind: p.kind,
      value: p.value,
      source: typeof p.source === "string" ? p.source : undefined,
    });
  }
  return out;
}

function makeNonce(): string {
  const bytes = new Uint8Array(16);
  for (let i = 0; i < 16; i++) bytes[i] = Math.floor(Math.random() * 256);
  return [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");
}
