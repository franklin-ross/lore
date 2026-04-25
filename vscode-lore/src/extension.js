import * as vscode from "vscode";
import {
  LanguageClient,
  TransportKind,
} from "vscode-languageclient/node";

/** @type {LanguageClient | undefined} */
let client;

// Palette is loaded from the extension manifest's
// editor.semanticTokenColorCustomizations rules at activation time, so the
// foreground hexes that style entity names and the background/underline
// hexes that style definition spans share a single source of truth.
// Index matches the colourIndex returned by the server's lore/definitionRanges
// request, which itself matches the loreColour{A..Z} bit position used by
// semantic-token modifiers.
/** @type {string[]} */
let palette = [];

// loadPaletteFromManifest extracts the foreground hex for each
// loreEntity.loreColour{A..Z} rule contributed by this extension. Returns
// an array indexed 0..25 = A..Z. Missing entries fall back to white so a
// malformed manifest still renders (visibly broken) rather than crashing.
function loadPaletteFromManifest(extension) {
  const rules =
    extension?.packageJSON?.contributes?.configurationDefaults?.[
      "editor.semanticTokenColorCustomizations"
    ]?.["[*]"]?.rules ?? {};
  const out = new Array(26).fill("#FFFFFF");
  for (const [key, val] of Object.entries(rules)) {
    const m = /^loreEntity\.loreColour([A-Z])$/.exec(key);
    if (!m || !val || typeof val.foreground !== "string") continue;
    out[m[1].charCodeAt(0) - 65] = val.foreground;
  }
  return out;
}

/** @type {vscode.TextEditorDecorationType[]} */
let underlineDecorations = [];
/** @type {vscode.TextEditorDecorationType[]} */
let backgroundDecorations = [];

function buildDecorations() {
  underlineDecorations = palette.map((hex) =>
    vscode.window.createTextEditorDecorationType({
      textDecoration: `underline solid ${hex}`,
      isWholeLine: false,
      rangeBehavior: vscode.DecorationRangeBehavior.ClosedClosed,
    })
  );
  backgroundDecorations = palette.map((hex) =>
    vscode.window.createTextEditorDecorationType({
      backgroundColor: hexToRgba(hex, 0.18),
      isWholeLine: false,
      rangeBehavior: vscode.DecorationRangeBehavior.ClosedClosed,
    })
  );
}

function disposeDecorations() {
  for (const d of underlineDecorations) d.dispose();
  for (const d of backgroundDecorations) d.dispose();
  underlineDecorations = [];
  backgroundDecorations = [];
}

function hexToRgba(hex, alpha) {
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  return `rgba(${r},${g},${b},${alpha})`;
}

function definitionStyle() {
  return (
    vscode.workspace.getConfiguration("lore").get("definitionStyle") ||
    "background"
  );
}

function clearDecorations(editor) {
  for (const d of underlineDecorations) editor.setDecorations(d, []);
  for (const d of backgroundDecorations) editor.setDecorations(d, []);
}

async function refreshEditor(editor) {
  if (!editor || editor.document.languageId !== "markdown") return;
  if (!client) return;
  if (underlineDecorations.length === 0) return;

  const style = definitionStyle();
  if (style === "none") {
    clearDecorations(editor);
    return;
  }

  /** @type {{ranges: {range: {start: {line: number, character: number}, end: {line: number, character: number}}, colourIndex: number}[]} | undefined} */
  let response;
  try {
    response = await client.sendRequest("lore/definitionRanges", {
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

  /** @type {vscode.Range[][]} */
  const perColour = palette.map(() => []);
  for (const r of response.ranges) {
    if (r.colourIndex < 0 || r.colourIndex >= palette.length) continue;
    perColour[r.colourIndex].push(
      new vscode.Range(
        r.range.start.line,
        r.range.start.character,
        r.range.end.line,
        r.range.end.character
      )
    );
  }

  const active = style === "background" ? backgroundDecorations : underlineDecorations;
  const inactive = style === "background" ? underlineDecorations : backgroundDecorations;
  for (let i = 0; i < palette.length; i++) {
    editor.setDecorations(active[i], perColour[i]);
    editor.setDecorations(inactive[i], []);
  }
}

function refreshAllVisible() {
  for (const editor of vscode.window.visibleTextEditors) {
    refreshEditor(editor);
  }
}

function buildInitializationOptions() {
  const config = vscode.workspace.getConfiguration("lore");
  return {
    hoverStateMode: config.get("hover.stateMode") || "both",
    hoverShowStateDirectives: config.get("hover.showStateDirectives") === true,
  };
}

function buildClient() {
  const config = vscode.workspace.getConfiguration("lore");
  const serverPath = config.get("serverPath") || "lore";

  /** @type {import("vscode-languageclient/node").ServerOptions} */
  const serverOptions = {
    command: serverPath,
    args: ["lsp"],
    transport: TransportKind.stdio,
  };

  /** @type {import("vscode-languageclient/node").LanguageClientOptions} */
  const clientOptions = {
    documentSelector: [{ scheme: "file", language: "markdown" }],
    synchronize: {
      fileEvents: vscode.workspace.createFileSystemWatcher("**/*.md"),
    },
    initializationOptions: buildInitializationOptions(),
  };

  return new LanguageClient(
    "lore",
    "Lore Language Server",
    serverOptions,
    clientOptions
  );
}

async function restartClient() {
  if (client) {
    await client.stop();
  }
  client = buildClient();
  await client.start();
  refreshAllVisible();
}

export function activate(/** @type {vscode.ExtensionContext} */ context) {
  palette = loadPaletteFromManifest(context.extension);
  buildDecorations();

  client = buildClient();
  client.start().then(refreshAllVisible);

  context.subscriptions.push(
    vscode.commands.registerCommand(
      "lore.toggleHoverStateDirectives",
      async () => {
        const config = vscode.workspace.getConfiguration("lore");
        const current = config.get("hover.showStateDirectives") === true;
        await config.update(
          "hover.showStateDirectives",
          !current,
          vscode.ConfigurationTarget.Global
        );
        await restartClient();
        vscode.window.setStatusBarMessage(
          `Lore: hover state directives ${!current ? "on" : "off"}`,
          2000
        );
      }
    )
  );

  context.subscriptions.push(
    vscode.window.onDidChangeActiveTextEditor((editor) => refreshEditor(editor)),
    vscode.window.onDidChangeVisibleTextEditors(refreshAllVisible),
    vscode.workspace.onDidChangeConfiguration((e) => {
      if (e.affectsConfiguration("lore.definitionStyle")) refreshAllVisible();
    })
  );

  // Debounced reapply on edits — the server reparses after a short delay so
  // we follow with our decorations.
  /** @type {ReturnType<typeof setTimeout> | undefined} */
  let debounce;
  context.subscriptions.push(
    vscode.workspace.onDidChangeTextDocument((e) => {
      if (e.document.languageId !== "markdown") return;
      if (debounce) clearTimeout(debounce);
      debounce = setTimeout(refreshAllVisible, 250);
    })
  );

  context.subscriptions.push({
    dispose: () => {
      disposeDecorations();
      return client?.stop();
    },
  });
}

export function deactivate() {
  disposeDecorations();
  return client?.stop();
}
