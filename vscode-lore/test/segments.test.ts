import "./setup-dom.ts";
import { describe, it, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { renderSegments, type ContextSegment } from "../media/wiki/segments.ts";
import { resetDom } from "./setup-dom.ts";

const PALETTE = ["#ff0000", "#00ff00", "#0000ff"];

describe("renderSegments", () => {
  beforeEach(() => resetDom());

  describe("click ownership", () => {
    it("opens entity wiki when linkToWiki is true", () => {
      const parent = document.createElement("div");
      const opened: string[] = [];
      const segments: ContextSegment[] = [
        { text: "Strahd", entity: "Strahd von Zarovich", colourIndex: 1 },
      ];
      renderSegments(segments, parent, PALETTE, (e) => opened.push(e), true);
      const span = parent.querySelector("span")!;
      span.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
      assert.deepEqual(opened, ["Strahd von Zarovich"]);
      assert.equal(span.style.cursor, "pointer");
    });

    it("leaves segments inert when linkToWiki is false (parent owns the click)", () => {
      const parent = document.createElement("div");
      const opened: string[] = [];
      const segments: ContextSegment[] = [
        { text: "Strahd", entity: "Strahd", colourIndex: 1 },
      ];
      renderSegments(segments, parent, PALETTE, (e) => opened.push(e), false);
      const span = parent.querySelector("span")!;
      span.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
      assert.deepEqual(opened, []);
      assert.notEqual(span.style.cursor, "pointer");
    });
  });

  it("marks ambiguous segments with wavy underline + tooltip", () => {
    const parent = document.createElement("div");
    const segments: ContextSegment[] = [
      { text: "Strahd", entity: "Strahd (npc)", ambiguous: true, colourIndex: 0 },
    ];
    renderSegments(segments, parent, PALETTE, () => {}, false);
    const span = parent.querySelector("span")!;
    assert.match(span.style.textDecoration, /wavy/);
    assert.equal(span.getAttribute("title"), "Ambiguous reference — Strahd (npc)");
  });
});
