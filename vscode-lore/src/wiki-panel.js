import * as vscode from "vscode";

// LoreWikiPanel manages a singleton webview that renders one entity at a
// time. Subsequent open requests reveal the existing panel and re-target
// it instead of spawning duplicates. Click events from the panel post
// back through vscode.postMessage and resolve to vscode.window.showTextDocument.
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
    /** @type {{entity: string, source: string | undefined} | undefined} */
    this.current = undefined;
  }

  /**
   * Open or reveal the wiki for `entity`. `source` is the active editor
   * URI used to scope the lookup to the right project.
   * @param {string} entity
   * @param {string | undefined} source
   */
  async show(entity, source) {
    if (!this.panel) {
      this.panel = vscode.window.createWebviewPanel(
        "loreWiki",
        "Lore Wiki",
        { viewColumn: vscode.ViewColumn.Beside, preserveFocus: true },
        { enableScripts: true, retainContextWhenHidden: true }
      );
      this.panel.webview.html = this.renderShell();
      this.panel.webview.onDidReceiveMessage((msg) => this.onMessage(msg));
      this.panel.onDidDispose(() => {
        this.panel = undefined;
        this.current = undefined;
      });
    } else {
      this.panel.reveal(vscode.ViewColumn.Beside, true);
    }
    this.current = { entity, source };
    this.panel.title = `Lore: ${entity}`;
    await this.refresh();
  }

  /**
   * Re-fetch the current entity's data and push it into the webview.
   * Called on initial show and from the extension's debounced edit
   * listener so the wiki stays current.
   */
  async refresh() {
    if (!this.panel || !this.current) return;
    const client = this.getClient();
    if (!client) {
      this.panel.webview.postMessage({ type: "error", message: "Language server not running." });
      return;
    }
    let response;
    try {
      response = await client.sendRequest("lore/entityDetails", {
        entity: this.current.entity,
        textDocument: this.current.source ? { uri: this.current.source } : undefined,
      });
    } catch (err) {
      this.panel.webview.postMessage({
        type: "error",
        message: (err && err.message) || String(err),
      });
      return;
    }
    this.panel.webview.postMessage({
      type: "details",
      details: response,
      palette: this.palette,
      requested: this.current.entity,
    });
  }

  /** @param {{type: string, uri?: string, line?: number}} msg */
  async onMessage(msg) {
    if (msg.type === "ready") {
      await this.refresh();
      return;
    }
    if (msg.type === "navigate" && msg.uri) {
      try {
        const uri = vscode.Uri.parse(msg.uri);
        const doc = await vscode.workspace.openTextDocument(uri);
        const line = Math.max(0, (msg.line ?? 1) - 1);
        const lineText = doc.lineAt(Math.min(line, doc.lineCount - 1));
        await vscode.window.showTextDocument(doc, {
          selection: lineText.range,
          viewColumn: vscode.ViewColumn.One,
          preserveFocus: false,
        });
      } catch (err) {
        vscode.window.showErrorMessage(
          `Lore: failed to open ${msg.uri}: ${(err && err.message) || err}`
        );
      }
    }
  }

  renderShell() {
    return `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<style>
  body {
    font-family: var(--vscode-font-family);
    font-size: var(--vscode-font-size);
    color: var(--vscode-foreground);
    padding: 16px 20px 32px;
    line-height: 1.5;
  }
  h1, h2, h3 { margin: 0; }
  h1 { font-size: 1.6em; font-weight: 700; }
  h2 { font-size: 1.05em; text-transform: uppercase; letter-spacing: 0.06em;
       color: var(--vscode-descriptionForeground); margin: 24px 0 8px; }
  h3 { font-size: 1em; font-weight: 600; margin: 14px 0 4px; }
  .type { color: var(--vscode-descriptionForeground); font-weight: 400;
          font-style: italic; margin-left: 8px; font-size: 0.9em; }
  .aliases { color: var(--vscode-descriptionForeground); font-size: 0.95em;
             margin-top: 4px; }
  .pill {
    display: inline-block;
    padding: 1px 8px;
    margin: 2px 4px 2px 0;
    border-radius: 10px;
    background: var(--vscode-badge-background);
    color: var(--vscode-badge-foreground);
    font-size: 0.85em;
    font-family: var(--vscode-editor-font-family);
  }
  table.fields { border-collapse: collapse; margin: 4px 0 8px; }
  table.fields td {
    padding: 2px 12px 2px 0;
    font-family: var(--vscode-editor-font-family);
    font-size: 0.95em;
  }
  table.fields td.k { color: var(--vscode-descriptionForeground); }
  .desc-block { margin: 12px 0 16px; }
  .desc-meta {
    display: flex; align-items: center; gap: 8px;
    font-size: 0.85em; color: var(--vscode-descriptionForeground);
    margin-bottom: 4px;
  }
  .desc-text { white-space: pre-wrap; }
  .jump {
    background: transparent; border: 1px solid var(--vscode-button-border, transparent);
    color: var(--vscode-textLink-foreground); cursor: pointer;
    padding: 1px 8px; border-radius: 3px; font-size: 0.85em;
  }
  .jump:hover { background: var(--vscode-list-hoverBackground); }
  .ref-group { margin: 8px 0 16px; }
  .ref-group .label {
    display: inline-block; font-weight: 600; font-size: 0.95em;
    margin-bottom: 4px;
  }
  .ref-row {
    display: grid;
    grid-template-columns: minmax(120px, max-content) 1fr;
    align-items: baseline;
    gap: 12px;
    padding: 4px 8px;
    margin: 2px 0;
    border-left: 2px solid var(--vscode-panel-border);
    cursor: pointer;
    font-family: var(--vscode-editor-font-family);
    font-size: 0.9em;
  }
  .ref-row:hover { background: var(--vscode-list-hoverBackground); }
  .ref-row .loc { color: var(--vscode-descriptionForeground); white-space: nowrap; }
  .ref-row .ctx {
    color: var(--vscode-foreground);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .empty { color: var(--vscode-descriptionForeground); font-style: italic; }
  .err { color: var(--vscode-errorForeground); }
  .history-row {
    display: grid;
    grid-template-columns: 80px 1fr minmax(120px, max-content);
    gap: 12px;
    align-items: baseline;
    padding: 4px 8px;
    cursor: pointer;
    font-family: var(--vscode-editor-font-family);
    font-size: 0.9em;
    border-left: 2px solid var(--vscode-panel-border);
  }
  .history-row:hover { background: var(--vscode-list-hoverBackground); }
  .history-row .op { font-weight: 600; }
  .history-row .loc { color: var(--vscode-descriptionForeground); white-space: nowrap; text-align: right; }
  .op-add { color: var(--vscode-charts-green); }
  .op-remove { color: var(--vscode-charts-red); }
  .op-set, .op-increment { color: var(--vscode-charts-blue); }
</style>
</head>
<body>
<div id="root"><p class="empty">Loading…</p></div>
<script>
  const vscode = acquireVsCodeApi();
  const root = document.getElementById("root");
  let palette = [];

  function colour(idx) {
    if (idx == null || idx < 0 || idx >= palette.length) return undefined;
    return palette[idx];
  }

  function el(tag, attrs, ...children) {
    const node = document.createElement(tag);
    if (attrs) {
      for (const [k, v] of Object.entries(attrs)) {
        if (v == null) continue;
        if (k === "style") Object.assign(node.style, v);
        else if (k === "class") node.className = v;
        else if (k === "onclick") node.onclick = v;
        else if (k === "data") for (const [dk, dv] of Object.entries(v)) node.dataset[dk] = dv;
        else node.setAttribute(k, v);
      }
    }
    for (const c of children) {
      if (c == null) continue;
      if (typeof c === "string") node.appendChild(document.createTextNode(c));
      else node.appendChild(c);
    }
    return node;
  }

  function navigate(uri, line) {
    vscode.postMessage({ type: "navigate", uri, line });
  }

  function basename(uri) {
    try {
      const path = new URL(uri).pathname;
      const idx = path.lastIndexOf("/");
      return idx >= 0 ? path.slice(idx + 1) : path;
    } catch {
      return uri;
    }
  }

  function locLine(loc) {
    return (loc?.range?.start?.line ?? 0) + 1;
  }

  function renderHeader(d) {
    const c = colour(d.colourIndex);
    const name = el("span", { style: c ? { color: c } : undefined }, d.name);
    const h1 = el("h1", null, name);
    if (d.type) h1.appendChild(el("span", { class: "type" }, d.type));
    const out = [h1];
    if (d.aliases && d.aliases.length) {
      out.push(el("div", { class: "aliases" }, "Also known as: " + d.aliases.join(", ")));
    }
    return out;
  }

  function renderState(d) {
    const hasTags = d.tags && d.tags.length;
    const hasFields = d.fields && d.fields.length;
    if (!hasTags && !hasFields) return [];
    const out = [el("h2", null, "State")];
    if (hasTags) {
      const wrap = el("div", null);
      for (const t of d.tags) wrap.appendChild(el("span", { class: "pill" }, "+" + t));
      out.push(wrap);
    }
    if (hasFields) {
      const tbl = el("table", { class: "fields" });
      for (const f of d.fields) {
        tbl.appendChild(el("tr", null,
          el("td", { class: "k" }, f.name),
          el("td", null, f.value)
        ));
      }
      out.push(tbl);
    }
    return out;
  }

  function renderSegments(segments, parent) {
    if (!segments || !segments.length) return parent;
    for (const seg of segments) {
      const c = colour(seg.colourIndex);
      if (c) {
        parent.appendChild(el("span", { style: { color: c } }, seg.text));
      } else {
        parent.appendChild(document.createTextNode(seg.text));
      }
    }
    return parent;
  }

  function renderDescriptions(d) {
    if (!d.descriptions || !d.descriptions.length) return [];
    const out = [el("h2", null, "Descriptions")];
    for (const block of d.descriptions) {
      const range = block.endLine && block.endLine > block.startLine
        ? "L" + block.startLine + "–" + block.endLine
        : "L" + block.startLine;
      const meta = el("div", { class: "desc-meta" },
        basename(block.location.uri) + " · " + range,
        el("button", {
          class: "jump",
          onclick: () => navigate(block.location.uri, block.startLine),
        }, "Jump ↗")
      );
      const text = el("div", { class: "desc-text" });
      renderSegments(block.segments, text);
      out.push(el("div", { class: "desc-block" }, meta, text));
    }
    return out;
  }

  function renderRefGroups(title, groups, freeTextLabel) {
    if (!groups || !groups.length) return [];
    const out = [el("h2", null, title)];
    for (const g of groups) {
      const label = g.source || freeTextLabel;
      if (!label) continue;
      const c = colour(g.colourIndex);
      const heading = el("h3", { style: c ? { color: c } : undefined }, label);
      const group = el("div", { class: "ref-group" }, heading);
      for (const r of g.refs) {
        const line = locLine(r.location);
        const ctx = el("span", { class: "ctx" });
        renderSegments(r.segments, ctx);
        const row = el("div", {
          class: "ref-row",
          onclick: () => navigate(r.location.uri, line),
        },
          el("span", { class: "loc" }, basename(r.location.uri) + ":" + line),
          ctx
        );
        group.appendChild(row);
      }
      out.push(group);
    }
    return out;
  }

  function renderStateHistory(d) {
    if (!d.stateHistory || !d.stateHistory.length) return [];
    const out = [el("h2", null, "State History")];
    for (const ev of d.stateHistory) {
      const line = locLine(ev.location);
      const op = el("span", { class: "op op-" + ev.op }, ev.op);
      const value = ev.value
        ? ev.target + " = " + ev.value
        : (ev.op === "remove" ? "−" : "+") + ev.target;
      const row = el("div", {
        class: "history-row",
        onclick: () => navigate(ev.location.uri, line),
      },
        op,
        el("span", null, value),
        el("span", { class: "loc" }, basename(ev.location.uri) + ":" + line)
      );
      out.push(row);
    }
    return out;
  }

  function render(details) {
    root.innerHTML = "";
    if (!details || !details.found) {
      root.appendChild(el("p", { class: "empty" },
        "Entity not found in this project."));
      return;
    }
    for (const node of [
      ...renderHeader(details),
      ...renderState(details),
      ...renderDescriptions(details),
      ...renderRefGroups("Mentioned by", details.inboundRefs, "Free text"),
      ...renderRefGroups("Mentions", details.outboundRefs, ""),
      ...renderStateHistory(details),
    ]) root.appendChild(node);
  }

  window.addEventListener("message", (e) => {
    const msg = e.data;
    if (msg.type === "details") {
      palette = msg.palette || [];
      render(msg.details);
    } else if (msg.type === "error") {
      root.innerHTML = "";
      root.appendChild(el("p", { class: "err" }, msg.message || "Unknown error"));
    }
  });

  vscode.postMessage({ type: "ready" });
</script>
</body>
</html>`;
  }
}
