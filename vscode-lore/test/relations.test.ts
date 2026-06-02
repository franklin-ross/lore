import "./setup-dom.ts";
import { describe, it, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { renderEntityPage, type EntityDetails } from "../media/wiki/entity-page.ts";
import { createSearchController } from "../media/wiki/search.ts";
import type { PageCtx } from "../media/wiki/ctx.ts";
import { resetDom } from "./setup-dom.ts";

const PALETTE = ["#ff0000", "#00ff00", "#0000ff"];

function makeCtx(openedEntities: string[]): PageCtx {
  const search = createSearchController({
    openEntity: (e) => openedEntities.push(e),
    openType: () => {},
  });
  return {
    palette: PALETTE,
    navigate: () => {},
    openEntity: (e) => openedEntities.push(e),
    openType: () => {},
    openHome: () => {},
    collapsed: new Set<string>(),
    onToggle: () => {},
    search,
  };
}

function mount(nodes: HTMLElement[]): HTMLElement {
  const root = document.createElement("div");
  for (const n of nodes) root.appendChild(n);
  return root;
}

describe("renderRelations", () => {
  beforeEach(() => resetDom());

  it("renders an outgoing group with header, entities, and annotation", () => {
    const opened: string[] = [];
    const details: EntityDetails = {
      found: true,
      name: "Doug",
      type: "person",
      relations: [
        {
          header: "children",
          items: [
            { label: "Sarah", annotation: "daughter", colourIndex: 1 },
            { label: "Tim", colourIndex: 2 },
          ],
        },
      ],
    };
    const root = mount(renderEntityPage(details, makeCtx(opened)));

    const tbl = root.querySelector("table.fields");
    assert.ok(tbl, "expected a relations table");
    assert.match(tbl!.querySelector("td.k")?.textContent ?? "", /children →/);
    assert.match(tbl!.textContent ?? "", /Sarah \(daughter\)/);
    assert.match(tbl!.textContent ?? "", /Tim/);

    // Resolved entity is clickable.
    const ents = root.querySelectorAll(".relation-ent");
    const sarah = Array.from(ents).find((e) => e.textContent === "Sarah") as HTMLElement;
    assert.ok(sarah, "expected Sarah pill");
    sarah.click();
    assert.deepEqual(opened, ["Sarah"]);
  });

  it("renders incoming generic edges with an inward arrow", () => {
    const details: EntityDetails = {
      found: true,
      name: "Mary",
      type: "person",
      relations: [
        { header: "bestie", incoming: true, items: [{ label: "Sarah", colourIndex: 0 }] },
      ],
    };
    const root = mount(renderEntityPage(details, makeCtx([])));
    const key = root.querySelector("table.fields td.k");
    assert.match(key?.textContent ?? "", /bestie ←/);
    assert.match(root.textContent ?? "", /Sarah/);
  });

  it("omits the Relations section when there are none", () => {
    const details: EntityDetails = { found: true, name: "Loner", type: "person" };
    const root = mount(renderEntityPage(details, makeCtx([])));
    assert.equal(root.querySelector("table.fields"), null);
  });

  it("merges state and relation events into one History section", () => {
    const loc = {
      uri: "file:///x.md",
      range: { start: { line: 0, character: 0 }, end: { line: 0, character: 0 } },
    };
    const details: EntityDetails = {
      found: true,
      name: "Doug",
      type: "person",
      history: [
        { op: "add", target: "injured", location: loc },
        { op: "set", target: "gold", value: "5", location: loc },
        { op: "edgeAdd", target: "father", value: "Sarah", location: loc },
        { op: "edgeRemove", target: "friend", value: "Mary", location: loc },
      ],
    };
    const root = mount(renderEntityPage(details, makeCtx([])));
    const text = root.textContent ?? "";
    assert.match(text, /\+injured/);
    assert.match(text, /gold = 5/);
    assert.match(text, /father → Sarah/);
    assert.match(text, /friend -\/> Mary/);
  });
});
