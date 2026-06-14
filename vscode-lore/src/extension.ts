import * as vscode from "vscode";
import * as path from "node:path";
import * as fs from "node:fs";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import {
  LanguageClient,
  TransportKind,
  type ServerOptions,
  type LanguageClientOptions,
} from "vscode-languageclient/node";
import { LoreWikiPanel } from "./wiki-panel.ts";
import { LoreGraphPanel } from "./graph-panel.ts";
import { EntityFocusBus } from "./entity-focus-bus.ts";

let extensionPath = "";

interface DefinitionRangeItem {
  range: { start: { line: number; character: number }; end: { line: number; character: number } };
  colourIndex: number;
}
interface DefinitionRangesResponse {
  ranges?: DefinitionRangeItem[];
}

const execFileAsync = promisify(execFile);

// resolveServerPath picks the lore binary in this order:
//   1. lore.serverPath setting (explicit user override)
//   2. bundled binary at <extensionPath>/bin/lore[.exe]
//   3. "lore" on PATH (fallback for source installs and dev)
async function resolveServerPath(): Promise<string> {
  const override = vscode.workspace.getConfiguration("lore").get<string>("serverPath");
  if (override) return override;

  const exe = process.platform === "win32" ? "lore.exe" : "lore";
  const bundled = path.join(extensionPath, "bin", exe);
  if (!fs.existsSync(bundled)) return "lore";

  if (process.platform !== "win32") {
    try {
      fs.chmodSync(bundled, 0o755);
    } catch {
      // Ignore — file may already have correct mode or live on read-only FS.
    }
  }
  if (process.platform === "darwin") {
    // Defensive — Node downloads usually don't tag quarantine, but strip if
    // present. Async so the extension host isn't blocked on `xattr` during
    // activation; awaited before the LSP binary is spawned so a quarantined
    // binary still gets cleared before launch.
    try {
      await execFileAsync("xattr", ["-d", "com.apple.quarantine", bundled], {
        windowsHide: true,
      });
    } catch {
      // No quarantine attr to strip — expected.
    }
  }
  return bundled;
}

let client: LanguageClient | undefined;

// Palette is loaded from the extension manifest's
// editor.semanticTokenColorCustomizations rules at activation time, so the
// foreground hexes that style entity names and the background/underline
// hexes that style definition spans share a single source of truth.
// Index matches the colourIndex returned by the server's lore/definitionRanges
// request, which itself matches the loreColour{A..Z} bit position used by
// semantic-token modifiers.
let palette: string[] = [];

interface RuleEntry { foreground?: string }

// loadPaletteFromManifest extracts the foreground hex for each
// loreEntity.loreColour{A..Z} rule contributed by this extension. Returns
// an array indexed 0..25 = A..Z. Missing entries fall back to white so a
// malformed manifest still renders (visibly broken) rather than crashing.
function loadPaletteFromManifest(extension: vscode.Extension<unknown> | undefined): string[] {
  const pkg = extension?.packageJSON as Record<string, unknown> | undefined;
  const contrib = pkg?.contributes as Record<string, unknown> | undefined;
  const defaults = contrib?.configurationDefaults as Record<string, unknown> | undefined;
  const tokens = defaults?.["editor.semanticTokenColorCustomizations"] as Record<string, unknown> | undefined;
  const star = tokens?.["[*]"] as Record<string, unknown> | undefined;
  const rules = (star?.rules as Record<string, RuleEntry> | undefined) ?? {};
  const out = new Array<string>(26).fill("#FFFFFF");
  for (const [key, val] of Object.entries(rules)) {
    const m = /^loreEntity\.loreColour([A-Z])$/.exec(key);
    if (!m || !val || typeof val.foreground !== "string") continue;
    out[m[1]!.charCodeAt(0) - 65] = val.foreground;
  }
  return out;
}

let underlineDecorations: vscode.TextEditorDecorationType[] = [];
let backgroundDecorations: vscode.TextEditorDecorationType[] = [];

function buildDecorations(): void {
  underlineDecorations = palette.map((hex) =>
    vscode.window.createTextEditorDecorationType({
      textDecoration: `underline solid ${hex}`,
      isWholeLine: false,
      rangeBehavior: vscode.DecorationRangeBehavior.ClosedClosed,
    }),
  );
  backgroundDecorations = palette.map((hex) =>
    vscode.window.createTextEditorDecorationType({
      backgroundColor: hexToRgba(hex, 0.18),
      isWholeLine: false,
      rangeBehavior: vscode.DecorationRangeBehavior.ClosedClosed,
    }),
  );
}

function disposeDecorations(): void {
  for (const d of underlineDecorations) d.dispose();
  for (const d of backgroundDecorations) d.dispose();
  underlineDecorations = [];
  backgroundDecorations = [];
}

function hexToRgba(hex: string, alpha: number): string {
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  return `rgba(${r},${g},${b},${alpha})`;
}

function definitionStyle(): "background" | "underline" | "none" {
  const v = vscode.workspace.getConfiguration("lore").get<string>("definitionStyle") ?? "background";
  return v as "background" | "underline" | "none";
}

function clearDecorations(editor: vscode.TextEditor): void {
  for (const d of underlineDecorations) editor.setDecorations(d, []);
  for (const d of backgroundDecorations) editor.setDecorations(d, []);
}

// Cache of resolved fsPath → bool so the F12 context-key update doesn't
// stat the filesystem on every cursor move. Flushed by the `**/lore.toml`
// watcher in activate() whenever a config file appears, disappears, or is
// renamed — otherwise stale entries would mis-classify files until the
// window reloaded.
const projectCache = new Map<string, boolean>();

function isInLoreProject(uri: vscode.Uri | undefined): boolean {
  if (!uri || uri.scheme !== "file") return false;
  const cached = projectCache.get(uri.fsPath);
  if (cached !== undefined) return cached;
  let dir = path.dirname(uri.fsPath);
  const root = path.parse(dir).root;
  while (dir && dir !== root) {
    if (fs.existsSync(path.join(dir, "lore.toml"))) {
      projectCache.set(uri.fsPath, true);
      return true;
    }
    const parent = path.dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  projectCache.set(uri.fsPath, false);
  return false;
}

function updateInProjectContext(editor: vscode.TextEditor | undefined): void {
  const inProject = !!editor
    && editor.document.languageId === "markdown"
    && isInLoreProject(editor.document.uri);
  vscode.commands.executeCommand("setContext", "lore.inProject", inProject);
}

async function refreshEditor(editor: vscode.TextEditor | undefined): Promise<void> {
  if (!editor || editor.document.languageId !== "markdown") return;
  if (!client) return;
  if (underlineDecorations.length === 0) return;

  const style = definitionStyle();
  if (style === "none") {
    clearDecorations(editor);
    return;
  }

  let response: DefinitionRangesResponse | undefined;
  try {
    response = await client.sendRequest<DefinitionRangesResponse>("lore/definitionRanges", {
      textDocument: { uri: editor.document.uri.toString() },
    });
  } catch {
    clearDecorations(editor);
    return;
  }
  if (!response || !response.ranges) {
    clearDecorations(editor);
    return;
  }

  const perColour: vscode.Range[][] = palette.map(() => []);
  for (const r of response.ranges) {
    if (r.colourIndex < 0 || r.colourIndex >= palette.length) continue;
    perColour[r.colourIndex]!.push(
      new vscode.Range(
        r.range.start.line,
        r.range.start.character,
        r.range.end.line,
        r.range.end.character,
      ),
    );
  }

  const active = style === "background" ? backgroundDecorations : underlineDecorations;
  const inactive = style === "background" ? underlineDecorations : backgroundDecorations;
  for (let i = 0; i < palette.length; i++) {
    editor.setDecorations(active[i]!, perColour[i]!);
    editor.setDecorations(inactive[i]!, []);
  }
}

function refreshAllVisible(): void {
  for (const editor of vscode.window.visibleTextEditors) {
    refreshEditor(editor);
  }
}

function buildInitializationOptions(): Record<string, unknown> {
  const config = vscode.workspace.getConfiguration("lore");
  return {
    hoverStateMode: config.get("hover.stateMode") || "both",
    hoverShowStateDirectives: config.get("hover.showStateDirectives") === true,
    palette,
  };
}

async function buildClient(): Promise<LanguageClient> {
  const serverPath = await resolveServerPath();

  const serverOptions: ServerOptions = {
    command: serverPath,
    args: ["lsp"],
    transport: TransportKind.stdio,
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: "file", language: "markdown" }],
    synchronize: {
      fileEvents: vscode.workspace.createFileSystemWatcher("**/*.md"),
    },
    initializationOptions: buildInitializationOptions(),
    middleware: {
      // Hovers from the server include `<span style="color:#hex">` to mirror
      // buffer entity colours. Markdown rendering strips HTML by default,
      // and the MarkdownString instances vscode-languageclient produces
      // sometimes don't survive an `instanceof vscode.MarkdownString`
      // check (different module identity once bundled). Reconstruct each
      // content item as a fresh MarkdownString with supportHtml on, so
      // DOMPurify keeps our colour spans regardless of upstream wiring.
      provideHover: async (document, position, token, next) => {
        const hover = await next(document, position, token);
        if (!hover) return hover;
        hover.contents = hover.contents.map((c) => {
          let value: string;
          let isTrusted: boolean | { readonly enabledCommands: readonly string[] } | undefined;
          if (typeof c === "string") {
            value = c;
          } else if (c && typeof c === "object" && "value" in c) {
            value = (c as { value: string }).value;
            isTrusted = (c as { isTrusted?: boolean }).isTrusted;
          } else {
            return c;
          }
          const md = new vscode.MarkdownString(value);
          md.supportHtml = true;
          if (isTrusted) md.isTrusted = isTrusted;
          return md;
        });
        return hover;
      },
    },
  };

  return new LanguageClient(
    "lore",
    "Lore Language Server",
    serverOptions,
    clientOptions,
  );
}

async function restartClient(): Promise<void> {
  if (client) {
    await client.stop();
  }
  client = await buildClient();
  await client.start();
  refreshAllVisible();
}

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  extensionPath = context.extensionPath;
  palette = loadPaletteFromManifest(context.extension);
  buildDecorations();

  const focusBus = new EntityFocusBus();
  const wikiPanel = new LoreWikiPanel(() => client, palette, context, focusBus);
  const graphPanel = new LoreGraphPanel(() => client, palette, context, focusBus);
  updateInProjectContext(vscode.window.activeTextEditor);

  // Serializers hand each webview back to us across editor restarts so
  // the panels restore their last state instead of staying blank.
  context.subscriptions.push(
    vscode.window.registerWebviewPanelSerializer("loreWiki", {
      async deserializeWebviewPanel(panel: vscode.WebviewPanel, state: unknown): Promise<void> {
        await wikiPanel.restore(panel, state);
      },
    }),
    vscode.window.registerWebviewPanelSerializer("loreGraph", {
      async deserializeWebviewPanel(panel: vscode.WebviewPanel, state: unknown): Promise<void> {
        await graphPanel.restore(panel, state);
      },
    }),
    { dispose: () => focusBus.dispose() },
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("lore.openWikiAtCursor", async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) return;
      const doc = editor.document;
      const sel = editor.selection;
      let entity = "";
      let offset: number | undefined;
      if (!sel.isEmpty) {
        entity = doc.getText(sel).trim();
      } else {
        // Send the whole line plus the cursor's byte offset within it. The
        // server scans the line for entity mentions and returns the one
        // covering the cursor, so multi-word and period-containing names
        // match without the client trying to guess a name's shape. A
        // rarely-used, manually-invoked command — a generous payload is fine.
        const line = doc.lineAt(sel.active.line).text;
        const lead = line.length - line.trimStart().length;
        entity = line.trim();
        const cut = Math.min(
          entity.length,
          Math.max(0, sel.active.character - lead),
        );
        // UTF-8 byte offset to match the server's byte-indexed entity spans.
        offset = new TextEncoder().encode(entity.slice(0, cut)).length;
      }
      if (!entity) return;
      await wikiPanel.showWord(entity, doc.uri.toString(), offset);
    }),
    vscode.commands.registerCommand("lore.openWiki", async (arg: unknown) => {
      const entity = typeof arg === "string" ? arg : "";
      const editor = vscode.window.activeTextEditor;
      const source = editor ? editor.document.uri.toString() : undefined;
      if (entity) {
        await wikiPanel.show(entity, source);
      } else {
        await wikiPanel.showHome(source);
      }
    }),
    vscode.commands.registerCommand("lore.openGraph", async (arg: unknown, sourceArg: unknown) => {
      const entity = typeof arg === "string" ? arg : undefined;
      // Prefer an explicit source (the wiki passes its current page's source,
      // since the active surface there is a webview, not a text editor).
      // Fall back to the active editor for palette / F12 invocations.
      const editor = vscode.window.activeTextEditor;
      const source = typeof sourceArg === "string"
        ? sourceArg
        : (editor ? editor.document.uri.toString() : undefined);
      await graphPanel.show(source, entity);
    }),
  );

  client = await buildClient();
  client.start().then(() => {
    refreshAllVisible();
    // Server fires this after `discoverAllProjects` whenever a lore.toml
    // event reshapes the world. Pull everything that depends on the
    // project layout — see notifyProjectsChanged() in watched.go for why
    // the server is the one driving this rather than the client watcher.
    client?.onNotification("lore/projectsChanged", () => {
      refreshAllVisible();
      wikiPanel.refresh();
      graphPanel.refresh();
    });
  });

  context.subscriptions.push(
    vscode.commands.registerCommand(
      "lore.toggleHoverStateDirectives",
      async () => {
        const config = vscode.workspace.getConfiguration("lore");
        const current = config.get("hover.showStateDirectives") === true;
        await config.update(
          "hover.showStateDirectives",
          !current,
          vscode.ConfigurationTarget.Global,
        );
        await restartClient();
        vscode.window.setStatusBarMessage(
          `Lore: hover state directives ${!current ? "on" : "off"}`,
          2000,
        );
      },
    ),
  );

  context.subscriptions.push(
    vscode.window.onDidChangeActiveTextEditor((editor) => {
      refreshEditor(editor);
      updateInProjectContext(editor);
    }),
    vscode.window.onDidChangeVisibleTextEditors(refreshAllVisible),
    vscode.workspace.onDidChangeConfiguration((e) => {
      if (e.affectsConfiguration("lore.definitionStyle")) refreshAllVisible();
      if (e.affectsConfiguration("lore.graph.arrowSize")) graphPanel.refresh();
    }),
    vscode.workspace.onDidChangeWorkspaceFolders(() => {
      projectCache.clear();
      updateInProjectContext(vscode.window.activeTextEditor);
    }),
  );

  // Server also watches `**/lore.toml` to rebuild its project list. The
  // client mirror exists because `isInLoreProject` gates the `lore.inProject`
  // context key (which drives keybindings) and is hot on cursor move — we
  // don't want to round-trip to the server for it. Refresh of the actual
  // server-derived views (decorations, wiki, graph) is driven by the
  // `lore/projectsChanged` notification below, not by this watcher: both
  // fire on the same FS event but the server's project rebuild and this
  // callback race, and refreshing from the client side first would just
  // re-cache the stale world.
  const configWatcher = vscode.workspace.createFileSystemWatcher("**/lore.toml");
  const onConfigChanged = (): void => {
    projectCache.clear();
    updateInProjectContext(vscode.window.activeTextEditor);
  };
  context.subscriptions.push(
    configWatcher,
    configWatcher.onDidCreate(onConfigChanged),
    configWatcher.onDidDelete(onConfigChanged),
  );

  // Debounced reapply on edits — the server reparses after a short delay so
  // we follow with our decorations. Skip non-lore-project edits entirely; a
  // single in-flight refresh is allowed at a time, with a trailing rerun if
  // another edit lands mid-flight, so fast typing can't pile concurrent
  // duplicates of the five request streams (definitionRanges × N editors,
  // entityDetails, entityList, graph × 2 panels) on top of each other.
  let debounce: ReturnType<typeof setTimeout> | undefined;
  let refreshInflight = false;
  let refreshPending = false;
  const runRefresh = async (): Promise<void> => {
    if (refreshInflight) { refreshPending = true; return; }
    refreshInflight = true;
    try {
      do {
        refreshPending = false;
        await Promise.all([
          ...vscode.window.visibleTextEditors.map(refreshEditor),
          wikiPanel.refresh(),
          graphPanel.refresh(),
        ]);
      } while (refreshPending);
    } finally {
      refreshInflight = false;
    }
  };
  context.subscriptions.push(
    vscode.workspace.onDidChangeTextDocument((e) => {
      if (e.document.languageId !== "markdown") return;
      if (!isInLoreProject(e.document.uri)) return;
      if (debounce) clearTimeout(debounce);
      debounce = setTimeout(runRefresh, 250);
    }),
  );

  context.subscriptions.push({
    dispose: () => {
      disposeDecorations();
      return client?.stop();
    },
  });
}

export function deactivate(): Thenable<void> | undefined {
  disposeDecorations();
  return client?.stop();
}
