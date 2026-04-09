import * as vscode from "vscode";
import {
  LanguageClient,
  TransportKind,
} from "vscode-languageclient/node";

/** @type {LanguageClient | undefined} */
let client;

export function activate(/** @type {vscode.ExtensionContext} */ context) {
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
  };

  client = new LanguageClient(
    "lore",
    "Lore Language Server",
    serverOptions,
    clientOptions
  );

  client.start();
  context.subscriptions.push({ dispose: () => client?.stop() });
}

export function deactivate() {
  return client?.stop();
}
