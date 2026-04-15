import * as vscode from "vscode";
import {
  LanguageClient,
  TransportKind,
} from "vscode-languageclient/node";

/** @type {LanguageClient | undefined} */
let client;

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
}

export function activate(/** @type {vscode.ExtensionContext} */ context) {
  client = buildClient();
  client.start();

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

  context.subscriptions.push({ dispose: () => client?.stop() });
}

export function deactivate() {
  return client?.stop();
}
