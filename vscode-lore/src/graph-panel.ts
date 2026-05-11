import * as vscode from "vscode";
import type { LanguageClient } from "vscode-languageclient/node";
import { EntityFocusBus } from "./entity-focus-bus.ts";

// arrowSizeFromConfig pulls the user's `lore.graph.arrowSize` value. The
// graph webview interprets 0 as "no arrowheads" — the default — and any
// positive number as the SVG marker size in user-space units.
function arrowSizeFromConfig(): number {
    const v = vscode.workspace
        .getConfiguration("lore")
        .get<number>("graph.arrowSize");
    return typeof v === "number" && v >= 0 ? v : 0;
}

interface GraphNode {
    label: string;
    name: string;
    type?: string;
    colourIndex: number;
}

interface GraphDefEdge {
    from: string;
    to: string;
    count: number;
}

interface GraphResponse {
    nodes?: GraphNode[];
    defEdges?: GraphDefEdge[];
    // Server-supplied placeholder text, shown when the request couldn't
    // resolve a project to scope to (e.g. multiple projects open and no
    // active editor to disambiguate). Empty/absent means render the graph
    // normally.
    message?: string;
}

interface IncomingMessage {
    type: string;
    entity?: string;
}

/**
 * Singleton webview panel for the entity knowledge graph. Mirrors
 * LoreWikiPanel's transport shape: extension fetches lore/graph from the
 * LSP server and posts the payload into the webview, which renders nodes
 * and edges. Cross-panel selection coordination flows through
 * EntityFocusBus — graph node clicks fire focus events that the wiki
 * picks up, and wiki navigation fires events the graph uses to highlight.
 */
export class LoreGraphPanel {
    private panel: vscode.WebviewPanel | undefined;
    private currentFocus: string | undefined;
    private currentSource: string | undefined;
    private busSub: vscode.Disposable | undefined;
    // Whitelist of entity types to display. null = no filter (all types
    // visible). Cleared back to null when the user picks every type so
    // selecting "everything" doesn't leave an opaque filter active.
    private typeFilter: Set<string> | null = null;
    // Cache of the most recent server payload — needed so the type-filter
    // quick-pick can list types and counts without an extra fetch.
    private lastGraph: GraphResponse | null = null;

    constructor(
        private readonly getClient: () => LanguageClient | undefined,
        private readonly palette: string[],
        private readonly context: vscode.ExtensionContext,
        private readonly bus: EntityFocusBus,
    ) {}

    async show(source: string | undefined, focus?: string): Promise<void> {
        // Preserve the last source when none is provided — reopening the
        // panel with no active editor (or an editor outside any lore
        // project) should keep showing the graph it was already on, not
        // blank out.
        if (source) this.currentSource = source;
        if (focus) this.currentFocus = focus;
        this.ensurePanel();
        await this.refresh();
    }

    // refresh re-fetches the graph data and pushes it into the webview. Called
    // on initial open, on focus changes, and from the debounced edit listener.
    async refresh(): Promise<void> {
        if (!this.panel) return;
        const client = this.getClient();
        if (!client) {
            this.panel.webview.postMessage({
                type: "error",
                message: "Language server not running.",
            });
            return;
        }
        const td = this.currentSource ? { uri: this.currentSource } : undefined;
        let payload: GraphResponse | null = null;
        try {
            payload = await client.sendRequest<GraphResponse>("lore/graph", {
                textDocument: td,
            });
        } catch (err) {
            const message = err instanceof Error ? err.message : String(err);
            this.panel.webview.postMessage({ type: "error", message });
            return;
        }
        this.lastGraph = payload;
        if (payload?.message) {
            this.panel.webview.postMessage({
                type: "info",
                message: payload.message,
            });
            return;
        }
        this.panel.webview.postMessage({
            type: "graph",
            payload,
            palette: this.palette,
            focus: this.currentFocus ?? null,
            arrowSize: arrowSizeFromConfig(),
            filteredTypes: this.typeFilter ? [...this.typeFilter] : null,
        });
    }

    // showTypeFilter pops a multi-select VSCode quick-pick listing every
    // type in the cached graph response, with entity counts. The chosen
    // subset becomes the visibility whitelist; selecting all types (or
    // none) clears the filter back to "everything visible".
    async showTypeFilter(): Promise<void> {
        if (!this.lastGraph) {
            vscode.window.showInformationMessage("Lore: open the graph first.");
            return;
        }
        const counts = new Map<string, number>();
        for (const n of this.lastGraph.nodes ?? []) {
            const t = n.type ?? "(untyped)";
            counts.set(t, (counts.get(t) ?? 0) + 1);
        }
        if (counts.size === 0) {
            vscode.window.showInformationMessage(
                "Lore: no entities to filter.",
            );
            return;
        }
        const types = [...counts.entries()].sort(
            (a, b) => b[1] - a[1] || a[0].localeCompare(b[0]),
        );
        const items: vscode.QuickPickItem[] = types.map(([name, count]) => ({
            label: name,
            description: String(count),
            picked: !this.typeFilter || this.typeFilter.has(name),
        }));
        const picked = await vscode.window.showQuickPick(items, {
            canPickMany: true,
            title: "Filter graph by entity type",
            placeHolder:
                "Tick types to include — leave all ticked to disable the filter",
        });
        if (!picked) return; // user cancelled
        if (picked.length === 0 || picked.length === types.length) {
            this.typeFilter = null;
        } else {
            this.typeFilter = new Set(picked.map((i) => i.label));
        }
        if (this.panel) {
            this.panel.webview.postMessage({
                type: "filteredTypes",
                filteredTypes: this.typeFilter ? [...this.typeFilter] : null,
            });
        }
    }

    // setFocus is called by the entity-focus bus when something else (the
    // wiki, an external command) selects an entity. Posts the new focus into
    // the webview so the graph re-centres / highlights without a full refetch.
    private setFocus(entity: string, source: string | undefined): void {
        if (entity === this.currentFocus && source === this.currentSource)
            return;
        this.currentFocus = entity;
        if (source) this.currentSource = source;
        if (this.panel) {
            this.panel.webview.postMessage({ type: "focus", entity });
        }
    }

    async restore(panel: vscode.WebviewPanel, raw: unknown): Promise<void> {
        const state =
            raw && typeof raw === "object"
                ? (raw as { focus?: unknown; source?: unknown })
                : {};
        if (typeof state.focus === "string") this.currentFocus = state.focus;
        if (typeof state.source === "string") this.currentSource = state.source;
        this.attach(panel);
    }

    private ensurePanel(): void {
        if (this.panel) {
            this.panel.reveal(undefined, true);
            return;
        }
        const webviewRoot = vscode.Uri.joinPath(
            this.context.extensionUri,
            "out",
            "graph",
        );
        this.panel = vscode.window.createWebviewPanel(
            "loreGraph",
            "Lore Graph",
            { viewColumn: vscode.ViewColumn.Beside, preserveFocus: true },
            {
                enableScripts: true,
                retainContextWhenHidden: true,
                localResourceRoots: [webviewRoot],
            },
        );
        this.attach(this.panel);
    }

    private attach(panel: vscode.WebviewPanel): void {
        this.panel = panel;
        const webviewRoot = vscode.Uri.joinPath(
            this.context.extensionUri,
            "out",
            "graph",
        );
        panel.webview.options = {
            enableScripts: true,
            localResourceRoots: [webviewRoot],
        };
        panel.webview.html = this.renderShell();
        panel.webview.onDidReceiveMessage((msg: IncomingMessage) =>
            this.onMessage(msg),
        );
        panel.onDidDispose(() => {
            this.panel = undefined;
            this.busSub?.dispose();
            this.busSub = undefined;
        });
        // Subscribe to focus events from the wiki / other panels. Skip our own
        // emissions to avoid feedback when the user clicks a node here.
        this.busSub?.dispose();
        this.busSub = this.bus.onDidFocus((focus) => {
            if (focus.origin === "graph") return;
            this.setFocus(focus.entity, focus.source);
        });
    }

    private async onMessage(msg: IncomingMessage): Promise<void> {
        switch (msg.type) {
            case "ready":
                await this.refresh();
                return;
            case "focusEntity":
                if (msg.entity) {
                    this.currentFocus = msg.entity;
                    this.bus.fire({
                        entity: msg.entity,
                        source: this.currentSource,
                        origin: "graph",
                    });
                }
                return;
            case "openEntity":
                if (msg.entity) {
                    // Route through the public command so the wiki panel handles the
                    // open the same way as a palette / context-menu invocation.
                    await vscode.commands.executeCommand(
                        "lore.openWiki",
                        msg.entity,
                    );
                }
                return;
            case "openTypeFilter":
                await this.showTypeFilter();
                return;
        }
    }

    private renderShell(): string {
        const webview = this.panel!.webview;
        const mediaUri = (rel: string): vscode.Uri =>
            webview.asWebviewUri(
                vscode.Uri.joinPath(
                    this.context.extensionUri,
                    "out",
                    "graph",
                    rel,
                ),
            );
        const main = mediaUri("main.js");
        const styles = mediaUri("graph.css");
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
<main><div id="root"></div><div id="message-overlay" class="empty">Loading…</div></main>
<script type="module" nonce="${nonce}" src="${main}"></script>
</body>
</html>`;
    }
}

function makeNonce(): string {
    const bytes = new Uint8Array(16);
    for (let i = 0; i < 16; i++) bytes[i] = Math.floor(Math.random() * 256);
    return [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");
}
