import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { createSearchController, type Catalog } from "../media/wiki/search.ts";

const noop = (): void => {};

function fixture(): Catalog {
  return {
    entities: [
      { name: "Sildar Hallwinter", type: "character" },
      { name: "Strahd von Zarovich", type: "character" },
      { name: "Cragmaw Hideout", type: "location" },
      { name: "Saltmarsh", type: "location" },
    ],
    types: [
      { name: "character", count: 2 },
      { name: "location", count: 2 },
    ],
  };
}

describe("createSearchController", () => {
  describe("ranking", () => {
    it("returns no suggestions for an empty query", () => {
      const sc = createSearchController({ openEntity: noop, openType: noop });
      sc.setCatalog(fixture());
      sc._setQueryForTest("");
      assert.equal(sc._suggestions().length, 0);
    });

    it("matches both entities and types as substrings", () => {
      const sc = createSearchController({ openEntity: noop, openType: noop });
      sc.setCatalog(fixture());
      sc._setQueryForTest("loca");
      const labels = sc._suggestions().map((s) => s.label);
      assert.ok(labels.includes("location"));
      assert.equal(sc._suggestions().every((s) => s.kind === "type"), true);
    });

    it("orders exact > startsWith > substring", () => {
      const sc = createSearchController({ openEntity: noop, openType: noop });
      sc.setCatalog({
        entities: [
          { name: "Sildar Hallwinter", type: "character" },
          { name: "Sild", type: "character" },
          { name: "Other Sild", type: "character" },
        ],
        types: [{ name: "character", count: 3 }],
      });
      sc._setQueryForTest("sild");
      const order = sc._suggestions().map((s) => s.label);
      assert.deepEqual(order.slice(0, 3), ["Sild", "Sildar Hallwinter", "Other Sild"]);
    });

    it("formats type qualifier with entity count", () => {
      const sc = createSearchController({ openEntity: noop, openType: noop });
      sc.setCatalog({
        entities: [],
        types: [
          { name: "character", count: 1 },
          { name: "location", count: 5 },
        ],
      });
      sc._setQueryForTest("char");
      assert.equal(sc._suggestions()[0]!.qual, "1 entity");

      sc._setQueryForTest("loca");
      assert.equal(sc._suggestions()[0]!.qual, "5 entities");
    });
  });

  describe("commit", () => {
    it("dispatches to openEntity or openType and clears query", () => {
      const calls: string[] = [];
      const sc = createSearchController({
        openEntity: (e) => calls.push("entity:" + e),
        openType: (t) => calls.push("type:" + t),
      });
      sc.setCatalog(fixture());
      sc._setQueryForTest("sild");
      sc._commitForTest({ kind: "entity", label: "Sildar Hallwinter", qual: "character", score: 0 });
      assert.deepEqual(calls, ["entity:Sildar Hallwinter"]);
      assert.equal(sc._query(), "");

      sc._commitForTest({ kind: "type", label: "character", qual: "2 entities", score: 0 });
      assert.deepEqual(calls, ["entity:Sildar Hallwinter", "type:character"]);
    });
  });
});
