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
      return;
    }
    if (msg.type === "openWiki" && msg.entity) {
      // Re-target the existing panel rather than spawning a new one. show()
      // handles current/title bookkeeping and triggers a refresh.
      await this.show(msg.entity, this.current?.source);
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
  .desc-block {
    display: flex;
    align-items: stretch;
    gap: 8px;
    margin: 12px 0 16px;
  }
  .desc-text { white-space: pre-wrap; flex: 1; min-width: 0; }
  .desc-jump {
    display: flex;
    align-items: center;
    justify-content: center;
    flex: 0 0 14px;
    width: 14px;
    color: var(--vscode-descriptionForeground);
    background: var(--vscode-toolbar-hoverBackground, var(--vscode-list-hoverBackground));
    border-radius: 3px;
    cursor: pointer;
    font-size: 0.95em;
    line-height: 1;
    user-select: none;
  }
  .desc-jump:hover {
    background: var(--vscode-list-activeSelectionBackground, var(--vscode-list-hoverBackground));
    color: var(--vscode-list-activeSelectionForeground, var(--vscode-textLink-foreground));
  }
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
    grid-template-columns: 1fr minmax(120px, max-content);
    gap: 12px;
    align-items: baseline;
    padding: 4px 8px;
    cursor: pointer;
    font-family: var(--vscode-editor-font-family);
    font-size: 0.9em;
    border-left: 2px solid var(--vscode-panel-border);
  }
  .history-row:hover { background: var(--vscode-list-hoverBackground); }
  .history-row .loc { color: var(--vscode-descriptionForeground); white-space: nowrap; text-align: right; }
  .tabs {
    display: flex;
    gap: 4px;
    border-bottom: 1px solid var(--vscode-panel-border);
    margin: 4px 0 8px;
  }
  .tab {
    background: transparent;
    border: none;
    border-bottom: 2px solid transparent;
    color: var(--vscode-descriptionForeground);
    cursor: pointer;
    padding: 4px 10px;
    font: inherit;
    font-size: 0.95em;
    margin-bottom: -1px;
  }
  .tab:hover { color: var(--vscode-foreground); }
  .tab.active {
    color: var(--vscode-foreground);
    border-bottom-color: var(--vscode-focusBorder, var(--vscode-textLink-foreground));
  }
  .tab-panel { display: none; }
  .tab-panel.active { display: block; }
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

  function openWiki(entity) {
    vscode.postMessage({ type: "openWiki", entity });
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

  function renderStateBody(d) {
    const hasTags = d.tags && d.tags.length;
    const hasFields = d.fields && d.fields.length;
    if (!hasTags && !hasFields) {
      return [el("p", { class: "empty" }, "No state.")];
    }
    const out = [];
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

  function directiveText(ev) {
    switch (ev.op) {
      case "add": return "+" + ev.target;
      case "remove":
        return ev.value ? ev.target + " -= " + ev.value : "-" + ev.target;
      case "set": return ev.target + " = " + ev.value;
      case "increment": return ev.target + " += " + ev.value;
    }
    return ev.target;
  }

  function renderHistoryBody(d) {
    if (!d.stateHistory || !d.stateHistory.length) {
      return [el("p", { class: "empty" }, "No state history.")];
    }
    const out = [];
    for (const ev of d.stateHistory) {
      const line = locLine(ev.location);
      const row = el("div", {
        class: "history-row",
        onclick: () => navigate(ev.location.uri, line),
      },
        el("span", null, directiveText(ev)),
        el("span", { class: "loc" }, basename(ev.location.uri) + ":" + line)
      );
      out.push(row);
    }
    return out;
  }

  function renderStateSection(d) {
    const hasState = (d.tags && d.tags.length) || (d.fields && d.fields.length);
    const hasHistory = d.stateHistory && d.stateHistory.length;
    if (!hasState && !hasHistory) return [];

    const stateTab = el("button", { class: "tab active" }, "State");
    const historyTab = el("button", { class: "tab" }, "History");
    const tabs = el("div", { class: "tabs" }, stateTab, historyTab);

    const statePanel = el("div", { class: "tab-panel active" }, ...renderStateBody(d));
    const historyPanel = el("div", { class: "tab-panel" }, ...renderHistoryBody(d));

    stateTab.onclick = () => {
      stateTab.classList.add("active");
      historyTab.classList.remove("active");
      statePanel.classList.add("active");
      historyPanel.classList.remove("active");
    };
    historyTab.onclick = () => {
      historyTab.classList.add("active");
      stateTab.classList.remove("active");
      historyPanel.classList.add("active");
      statePanel.classList.remove("active");
    };

    return [el("h2", null, "State"), tabs, statePanel, historyPanel];
  }

  // linkToWiki=true makes coloured entity spans clickable, opening the
  // matched entity's wiki page. Reference-context previews pass false so
  // their row-level click still navigates to the editor location.
  // Ambiguous segments get a wavy warning underline and a tooltip naming
  // the disambiguated candidate, so a row of ambiguous candidates reads
  // clearly as a set of options rather than a typo.
  function renderSegments(segments, parent, linkToWiki) {
    if (!segments || !segments.length) return parent;
    for (const seg of segments) {
      const c = colour(seg.colourIndex);
      if (c) {
        const style = { color: c };
        const attrs = { style };
        if (seg.ambiguous) {
          style.textDecoration =
            "underline wavy var(--vscode-editorWarning-foreground)";
          attrs.title = "Ambiguous reference — " + (seg.entity || seg.text);
        }
        if (linkToWiki && seg.entity) {
          style.cursor = "pointer";
          attrs.onclick = (ev) => {
            ev.stopPropagation();
            openWiki(seg.entity);
          };
        }
        parent.appendChild(el("span", attrs, seg.text));
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
      const tooltip = block.endLine && block.endLine > block.startLine
        ? basename(block.location.uri) + ":" + block.startLine + "–" + block.endLine
        : basename(block.location.uri) + ":" + block.startLine;
      const jump = el("span", {
        class: "desc-jump",
        title: "Jump to " + tooltip,
        onclick: () => navigate(block.location.uri, block.startLine),
      });
      const text = el("div", { class: "desc-text" });
      renderSegments(block.segments, text, true);
      out.push(el("div", { class: "desc-block" }, jump, text));
    }
    return out;
  }

  // Renders one inbound or outbound reference section.
  //
  // freeTextLabel is the heading shown for refs whose source is empty
  // (mentions outside any entity definition). Pass "Free text" for the
  // inbound section so those refs appear under a real heading. Pass ""
  // for outbound: an entity can't legitimately mention something with no
  // target name, so any empty-source group is a glitch and we drop it.
  function renderRefGroups(title, groups, freeTextLabel) {
    if (!groups || !groups.length) return [];
    const out = [el("h2", null, title)];
    for (const g of groups) {
      const label = g.source || freeTextLabel;
      if (!label) continue;
      const c = colour(g.colourIndex);
      // Heading opens the source entity's wiki when the group's source
      // resolves to a known entity. Free-text and unresolved sources fall
      // through to a plain heading.
      const headingAttrs = {};
      const headingStyle = c ? { color: c } : {};
      if (g.source) {
        headingStyle.cursor = "pointer";
        headingAttrs.onclick = () => openWiki(g.source);
      }
      if (Object.keys(headingStyle).length) headingAttrs.style = headingStyle;
      const heading = el("h3", headingAttrs, label);
      const group = el("div", { class: "ref-group" }, heading);
      for (const r of g.refs) {
        const line = locLine(r.location);
        const ctx = el("span", { class: "ctx" });
        renderSegments(r.segments, ctx, false);
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

  function render(details) {
    root.innerHTML = "";
    if (!details || !details.found) {
      root.appendChild(el("p", { class: "empty" },
        "Entity not found in this project."));
      return;
    }
    for (const node of [
      ...renderHeader(details),
      ...renderStateSection(details),
      ...renderDescriptions(details),
      ...renderRefGroups("Mentioned by", details.inboundRefs, "Free text"),
      ...renderRefGroups("Mentions", details.outboundRefs, ""),
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
