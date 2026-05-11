import "./setup-dom.ts";
import { describe, it, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { renderEntityPage, type EntityDetails } from "../media/wiki/entity-page.ts";
import { renderTypePage, type TypeDetails } from "../media/wiki/type-page.ts";
import { renderHomePage } from "../media/wiki/home-page.ts";
import { createSearchController } from "../media/wiki/search.ts";
import type { PageCtx } from "../media/wiki/ctx.ts";
import { resetDom } from "./setup-dom.ts";

const PALETTE = ["#ff0000", "#00ff00", "#0000ff"];

interface TestCtx extends PageCtx {
  navigated: { uri: string; line: number }[];
  openedEntities: string[];
  openedTypes: string[];
}

function makeCtx(): TestCtx {
  const navigated: { uri: string; line: number }[] = [];
  const openedEntities: string[] = [];
  const openedTypes: string[] = [];
  const search = createSearchController({
    openEntity: (e) => openedEntities.push(e),
    openType: (t) => openedTypes.push(t),
  });
  return {
    palette: PALETTE,
    navigate: (uri, line) => navigated.push({ uri, line }),
    openEntity: (e) => openedEntities.push(e),
    openType: (t) => openedTypes.push(t),
    openHome: () => {},
    collapsed: new Set<string>(),
    onToggle: () => {},
    activeTab: "state",
    setActiveTab: () => {},
    search,
    navigated,
    openedEntities,
    openedTypes,
  };
}

function mountAll(nodes: HTMLElement[]): HTMLElement {
  const root = document.createElement("div");
  for (const n of nodes) root.appendChild(n);
  return root;
}

describe("renderEntityPage", () => {
  beforeEach(() => resetDom());

  it("renders header with name, aliases, and clickable type", () => {
    const ctx = makeCtx();
    const details: EntityDetails = {
      found: true,
      name: "Sildar Hallwinter",
      type: "character",
      colourIndex: 0,
      aliases: ["Sildar"],
    };
    const root = mountAll(renderEntityPage(details, ctx));
    const h1 = root.querySelector("h1")!;
    assert.match(h1.textContent ?? "", /Sildar Hallwinter/);
    assert.match(h1.textContent ?? "", /· Sildar/);

    const typeSpan = h1.querySelector(".type") as HTMLElement;
    assert.equal(typeSpan.textContent, "character");
    typeSpan.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    assert.deepEqual(ctx.openedTypes, ["character"]);
  });

  it("jumps to file:line when description jump button is clicked", () => {
    const ctx = makeCtx();
    const details: EntityDetails = {
      found: true,
      name: "Cragmaw Hideout",
      type: "location",
      descriptions: [
        {
          content: [
            { kind: "paragraph", children: [{ kind: "text", segments: [{ text: "North of Triboar Trail." }] }] },
          ],
          location: {
            uri: "file:///camp/notes.md",
            range: {
              start: { line: 9, character: 0 },
              end: { line: 9, character: 30 },
            },
          },
          startLine: 10,
          endLine: 10,
        },
      ],
    };
    const root = mountAll(renderEntityPage(details, ctx));
    (root.querySelector(".desc-jump") as HTMLElement)
      .dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    assert.deepEqual(ctx.navigated, [{ uri: "file:///camp/notes.md", line: 10 }]);
  });
});

describe("renderTypePage", () => {
  beforeEach(() => resetDom());

  it("routes h3 click to openEntity and ref-row click to navigate", () => {
    const ctx = makeCtx();
    const details: TypeDetails = {
      found: true,
      type: "character",
      entities: [
        {
          name: "Sildar Hallwinter",
          colourIndex: 0,
          aliases: ["Sildar"],
          location: {
            uri: "file:///camp/sildar.md",
            range: { start: { line: 0, character: 0 }, end: { line: 0, character: 18 } },
          },
          definitions: [
            {
              content: [
                { kind: "paragraph", children: [{ kind: "text", segments: [{ text: "Fighter. Member of Lords Alliance." }] }] },
              ],
              location: {
                uri: "file:///camp/sildar.md",
                range: { start: { line: 0, character: 0 }, end: { line: 0, character: 30 } },
              },
              startLine: 1,
              endLine: 1,
            },
          ],
        },
      ],
    };
    const root = mountAll(renderTypePage(details, ctx));
    const entry = root.querySelector(".type-entry") as HTMLElement;

    (entry.querySelector(".ref-row") as HTMLElement)
      .dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    assert.deepEqual(ctx.navigated, [{ uri: "file:///camp/sildar.md", line: 1 }]);

    (entry.querySelector("h3") as HTMLElement)
      .dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    assert.deepEqual(ctx.openedEntities, ["Sildar Hallwinter"]);
  });
});

describe("renderHomePage", () => {
  beforeEach(() => resetDom());

  it("renders type chips in catalog order and opens type on click", () => {
    const ctx = makeCtx();
    const root = mountAll(renderHomePage(
      {
        entities: [],
        types: [
          { name: "character", count: 5 },
          { name: "location", count: 2 },
        ],
      },
      ctx,
    ));

    const pills = [...root.querySelectorAll(".type-pill")] as HTMLElement[];
    assert.equal(pills.length, 2);
    assert.match(pills[0]!.textContent ?? "", /character/);
    assert.match(pills[0]!.textContent ?? "", /5/);

    pills[0]!.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    assert.deepEqual(ctx.openedTypes, ["character"]);
  });
});
