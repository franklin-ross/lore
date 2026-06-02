import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { filterToNeighbourhood } from "../media/wiki/entity-graph.ts";
import type { GraphPayload } from "../media/shared/graph-view.ts";

// Alice—Bob linked ONLY by a typed relation (no mention/defEdge between them);
// Bob—Carol linked by a mention. Dan is unconnected to the focus.
const PAYLOAD: GraphPayload = {
  nodes: [
    { label: "Alice", name: "Alice", colourIndex: 0 },
    { label: "Bob", name: "Bob", colourIndex: 1 },
    { label: "Carol", name: "Carol", colourIndex: 2 },
    { label: "Dan", name: "Dan", colourIndex: 3 },
  ],
  defEdges: [{ from: "Bob", to: "Carol", count: 1 }],
  relationEdges: [{ from: "Alice", to: "Bob", label: "parent", symmetric: false }],
};

describe("filterToNeighbourhood", () => {
  it("pulls relation-only neighbours into the kept set", () => {
    // Without relation adjacency, Bob (linked to Alice only by a relation)
    // would never enter Alice's neighbourhood.
    const out = filterToNeighbourhood(PAYLOAD, "Alice");
    const labels = out.nodes!.map((n) => n.label).sort();
    // Alice (focus), Bob (ring1 via relation), Carol (ring2 via Bob's mention).
    assert.deepEqual(labels, ["Alice", "Bob", "Carol"]);
    assert.ok(!labels.includes("Dan"), "unconnected node excluded");
  });

  it("preserves relation edges whose endpoints are both kept", () => {
    const out = filterToNeighbourhood(PAYLOAD, "Alice");
    assert.deepEqual(out.relationEdges, [
      { from: "Alice", to: "Bob", label: "parent", symmetric: false },
    ]);
  });

  it("drops edges with an endpoint outside the neighbourhood", () => {
    // Focus Alice: ring1 = {Bob}, ring2 = {Carol}. Dan sits at depth 3
    // (Alice–Bob–Carol–Dan), so it's outside the kept set and the
    // Carol→Dan relation edge must be dropped.
    const payload: GraphPayload = {
      ...PAYLOAD,
      relationEdges: [
        { from: "Alice", to: "Bob", label: "parent" },
        { from: "Carol", to: "Dan", label: "rival" },
      ],
    };
    const out = filterToNeighbourhood(payload, "Alice");
    assert.ok(!out.nodes!.some((n) => n.label === "Dan"), "Dan is depth-3, excluded");
    assert.deepEqual(
      out.relationEdges!.map((e) => `${e.from}->${e.to}`),
      ["Alice->Bob"],
    );
  });
});
