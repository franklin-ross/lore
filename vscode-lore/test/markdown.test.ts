import "./setup-dom.ts";
import { describe, it, beforeEach } from "node:test";
import assert from "node:assert/strict";
import {
  renderMarkdownTree,
  flattenSegments,
  type MarkdownNode,
} from "../media/wiki/markdown.ts";
import type { ContextSegment } from "../media/wiki/segments.ts";
import { resetDom } from "./setup-dom.ts";

const PALETTE = ["#ff0000", "#00ff00", "#0000ff"];

// txt is a tiny helper for a "text" leaf carrying a single plain
// segment, used to keep the test trees readable.
function txt(text: string): MarkdownNode {
  return { kind: "text", segments: [{ text }] };
}

describe("renderMarkdownTree", () => {
  beforeEach(() => resetDom());

  it("renders a paragraph as <p>", () => {
    const parent = document.createElement("div");
    renderMarkdownTree(
      [{ kind: "paragraph", children: [txt("Ranger of the North.")] }],
      parent,
      PALETTE,
      () => {},
      true,
    );
    const p = parent.querySelector("p");
    assert.ok(p, "expected a <p>");
    assert.equal(p!.textContent, "Ranger of the North.");
  });

  it("renders hr as <hr>", () => {
    const parent = document.createElement("div");
    renderMarkdownTree([{ kind: "hr" }], parent, PALETTE, () => {}, true);
    assert.ok(parent.querySelector("hr"));
  });

  it("renders blockquote with nested paragraph", () => {
    const parent = document.createElement("div");
    renderMarkdownTree(
      [{
        kind: "blockquote",
        children: [{ kind: "paragraph", children: [txt("Lots of prose.")] }],
      }],
      parent,
      PALETTE,
      () => {},
      true,
    );
    const bq = parent.querySelector("blockquote");
    assert.ok(bq, "expected a <blockquote>");
    const text = bq!.textContent ?? "";
    assert.equal(text.includes(">"), false, "`>` leaked into rendered text: " + text);
    assert.match(text, /Lots of prose/);
  });

  it("renders headings starting at <h2> (h1 reserved for entity name)", () => {
    const parent = document.createElement("div");
    renderMarkdownTree(
      [
        { kind: "heading", level: 1, children: [txt("Top")] },
        { kind: "heading", level: 2, children: [txt("Sub")] },
      ],
      parent,
      PALETTE,
      () => {},
      true,
    );
    assert.equal(parent.querySelector("h2")?.textContent, "Top");
    assert.equal(parent.querySelector("h3")?.textContent, "Sub");
  });

  it("renders unordered and ordered lists", () => {
    const parent = document.createElement("div");
    renderMarkdownTree(
      [
        {
          kind: "list",
          ordered: false,
          children: [
            { kind: "list-item", children: [{ kind: "paragraph", children: [txt("a")] }] },
            { kind: "list-item", children: [{ kind: "paragraph", children: [txt("b")] }] },
          ],
        },
        {
          kind: "list",
          ordered: true,
          children: [
            { kind: "list-item", children: [{ kind: "paragraph", children: [txt("one")] }] },
          ],
        },
      ],
      parent,
      PALETTE,
      () => {},
      true,
    );
    assert.equal(parent.querySelectorAll("ul li").length, 2);
    assert.equal(parent.querySelectorAll("ol li").length, 1);
  });

  it("renders fenced code blocks via <pre><code>", () => {
    const parent = document.createElement("div");
    renderMarkdownTree(
      [{ kind: "code-block", lang: "go", text: "func f() {}\n" }],
      parent,
      PALETTE,
      () => {},
      true,
    );
    const code = parent.querySelector("pre > code");
    assert.ok(code, "expected <pre><code>");
    assert.equal(code!.textContent, "func f() {}\n");
    assert.match(code!.className, /lang-go/);
  });

  it("renders inline emphasis, strong, code, link", () => {
    const parent = document.createElement("div");
    renderMarkdownTree(
      [{
        kind: "paragraph",
        children: [
          txt("see "),
          { kind: "emphasis", children: [txt("italic")] },
          txt(" and "),
          { kind: "strong", children: [txt("bold")] },
          txt(" plus "),
          { kind: "code-inline", text: "code" },
          txt(" and "),
          { kind: "link", href: "https://example.com", children: [txt("link")] },
        ],
      }],
      parent,
      PALETTE,
      () => {},
      true,
    );
    assert.equal(parent.querySelector("em")?.textContent, "italic");
    assert.equal(parent.querySelector("strong")?.textContent, "bold");
    assert.equal(parent.querySelector("code")?.textContent, "code");
    const a = parent.querySelector("a");
    assert.ok(a);
    assert.equal(a!.getAttribute("href"), "https://example.com");
    assert.equal(a!.textContent, "link");
  });

  it("preserves entity colour spans inside paragraph text leaves", () => {
    const parent = document.createElement("div");
    const segments: ContextSegment[] = [
      { text: "Met " },
      { text: "Aragorn", entity: "Aragorn", colourIndex: 0 },
      { text: " at dusk." },
    ];
    renderMarkdownTree(
      [{ kind: "paragraph", children: [{ kind: "text", segments }] }],
      parent,
      PALETTE,
      () => {},
      true,
    );
    const span = parent.querySelector("p > span");
    assert.ok(span, "expected entity-colour span inside paragraph");
    assert.equal(span!.textContent, "Aragorn");
  });

  it("preserves entity colour spans inside blockquote", () => {
    const parent = document.createElement("div");
    const segments: ContextSegment[] = [
      { text: "Met " },
      { text: "Aragorn", entity: "Aragorn", colourIndex: 0 },
      { text: " at dusk." },
    ];
    renderMarkdownTree(
      [{
        kind: "blockquote",
        children: [{ kind: "paragraph", children: [{ kind: "text", segments }] }],
      }],
      parent,
      PALETTE,
      () => {},
      true,
    );
    const span = parent.querySelector("blockquote span");
    assert.ok(span, "expected entity-colour span inside blockquote");
    assert.equal(span!.textContent, "Aragorn");
  });

  it("never executes embedded HTML — user prose is text only", () => {
    // A `<script>` smuggled in via a text leaf must not land as a real
    // <script> element. createElement / text nodes only — angle
    // brackets stay as literal characters.
    const parent = document.createElement("div");
    renderMarkdownTree(
      [{
        kind: "paragraph",
        children: [txt("<script>alert(1)</script>")],
      }],
      parent,
      PALETTE,
      () => {},
      true,
    );
    assert.equal(parent.querySelector("script"), null, "<script> leaked into DOM");
    assert.match(parent.textContent ?? "", /<script>alert\(1\)<\/script>/);
  });
});

describe("flattenSegments", () => {
  it("collects text-leaf segments in source order", () => {
    const out = flattenSegments([
      { kind: "paragraph", children: [
        { kind: "text", segments: [{ text: "alpha " }] },
        { kind: "strong", children: [{ kind: "text", segments: [{ text: "beta" }] }] },
        { kind: "text", segments: [{ text: " gamma." }] },
      ] },
    ]);
    assert.deepEqual(out.map((s) => s.text), ["alpha ", "beta", " gamma."]);
  });

  it("includes code spans and code blocks as plain text", () => {
    const out = flattenSegments([
      { kind: "paragraph", children: [
        { kind: "text", segments: [{ text: "before " }] },
        { kind: "code-inline", text: "X" },
        { kind: "text", segments: [{ text: " after" }] },
      ] },
      { kind: "code-block", text: "fn() {}\n" },
    ]);
    assert.deepEqual(out.map((s) => s.text), ["before ", "X", " after", "fn() {}\n"]);
  });
});
