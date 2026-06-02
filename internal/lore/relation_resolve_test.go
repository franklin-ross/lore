package lore

import (
	"strings"
	"testing"
)

func focusGroups(t *testing.T, world *World, name string) []RelationGroup {
	t.Helper()
	v := NewRelationVocab(BuiltinRelations())
	ent, err := world.FindEntity(name)
	if err != nil {
		t.Fatalf("find %q: %v", name, err)
	}
	return world.ResolveRelations(v, ent)
}

// findGroup returns the items for a canonical relation, or nil.
func findGroup(groups []RelationGroup, canonical string) []RelationItem {
	for _, g := range groups {
		if g.Canonical == canonical {
			return g.Items
		}
	}
	return nil
}

func TestRelationAsymmetricReciprocal(t *testing.T) {
	world := setupTestWorld(t, "Sarah (person): father -> Doug\n\nDoug (person): A dwarf.\n")

	doug := findGroup(focusGroups(t, world, "Doug"), "child")
	if len(doug) != 1 || doug[0].Other != "Sarah" {
		t.Fatalf("Doug child group = %+v; want [Sarah]", doug)
	}
	if doug[0].Surface != "child" {
		t.Fatalf("Doug child surface = %q; want child (canonical default)", doug[0].Surface)
	}

	sarah := findGroup(focusGroups(t, world, "Sarah"), "parent")
	if len(sarah) != 1 || sarah[0].Other != "Doug" {
		t.Fatalf("Sarah parent group = %+v; want [Doug]", sarah)
	}
	if sarah[0].Surface != "father" {
		t.Fatalf("Sarah parent surface = %q; want father (preserved)", sarah[0].Surface)
	}
}

func TestRelationBothSidesDeclaredMergeToOneEdge(t *testing.T) {
	world := setupTestWorld(t, "Sarah (person): father -> Doug\n\nDoug (person): daughter -> Sarah\n")

	doug := findGroup(focusGroups(t, world, "Doug"), "child")
	if len(doug) != 1 {
		t.Fatalf("Doug child items = %+v; want exactly 1 (no duplicate edge)", doug)
	}
	if doug[0].Surface != "daughter" {
		t.Fatalf("Doug child surface = %q; want daughter (his declared label)", doug[0].Surface)
	}

	sarah := findGroup(focusGroups(t, world, "Sarah"), "parent")
	if len(sarah) != 1 || sarah[0].Surface != "father" {
		t.Fatalf("Sarah parent = %+v; want one item, surface father", sarah)
	}
}

func TestRelationSymmetricShowsOnBothSides(t *testing.T) {
	world := setupTestWorld(t, "Aragorn (person): spouse -> Arwen\n\nArwen (person): An elf.\n")

	for _, name := range []string{"Aragorn", "Arwen"} {
		g := findGroup(focusGroups(t, world, name), "spouse")
		if len(g) != 1 {
			t.Fatalf("%s spouse group = %+v; want 1 item", name, g)
		}
	}
	if findGroup(focusGroups(t, world, "Aragorn"), "spouse")[0].Other != "Arwen" {
		t.Fatal("Aragorn's spouse should be Arwen")
	}
	if findGroup(focusGroups(t, world, "Arwen"), "spouse")[0].Other != "Aragorn" {
		t.Fatal("Arwen's spouse should be Aragorn (reciprocal, undeclared side)")
	}
}

func TestRelationGenericEdgeIncomingOnObjectSide(t *testing.T) {
	world := setupTestWorld(t, "Sarah (person): bestie -> Mary\n\nMary (person): A friend.\n")

	sarah := findGroup(focusGroups(t, world, "Sarah"), "bestie")
	if len(sarah) != 1 || sarah[0].Incoming || sarah[0].Other != "Mary" {
		t.Fatalf("Sarah bestie = %+v; want outgoing to Mary", sarah)
	}

	mary := findGroup(focusGroups(t, world, "Mary"), "bestie")
	if len(mary) != 1 || !mary[0].Incoming || mary[0].Other != "Sarah" {
		t.Fatalf("Mary bestie = %+v; want incoming from Sarah", mary)
	}
}

func TestRelationMembershipReciprocal(t *testing.T) {
	world := setupTestWorld(t, "Party (group): members -> Aragorn\n\nAragorn (person): A ranger.\n")

	party := findGroup(focusGroups(t, world, "Party"), "member")
	if len(party) != 1 || party[0].Other != "Aragorn" {
		t.Fatalf("Party member = %+v; want [Aragorn]", party)
	}
	aragorn := findGroup(focusGroups(t, world, "Aragorn"), "member-of")
	if len(aragorn) != 1 || aragorn[0].Other != "Party" {
		t.Fatalf("Aragorn member-of = %+v; want [Party]", aragorn)
	}
}

// Containment is generic and must read true from either endpoint. The old
// vocab filed `within`/`inside` as aliases of the container-side canonical, so
// one side always rendered inverted; `contains`/`container` are a proper pair.
func TestRelationContainmentBothDirections(t *testing.T) {
	// Declared from the container: "Barovia contains the town". `contains` is a
	// verb alias of the `contents` canonical.
	w1 := setupTestWorld(t, "Region (place): contains -> Town\n\nTown (place): A hamlet.\n")
	if g := findGroup(focusGroups(t, w1, "Region"), "contents"); len(g) != 1 || g[0].Other != "Town" || g[0].Incoming {
		t.Fatalf("Region contents = %+v; want outgoing to Town", g)
	}
	// Town's reverse must say the region is its container, not its contents.
	if g := findGroup(focusGroups(t, w1, "Town"), "container"); len(g) != 1 || g[0].Other != "Region" {
		t.Fatalf("Town container = %+v; want [Region] (region is the town's container)", g)
	}

	// Declared from the contained, generic case: "the sword is within the box".
	w2 := setupTestWorld(t, "Sword (item): within -> Box\n\nBox (item): A chest.\n")
	box := findGroup(focusGroups(t, w2, "Box"), "contents")
	if len(box) != 1 || box[0].Other != "Sword" {
		t.Fatalf("Box contents = %+v; want [Sword] (box contains the sword, not vice versa)", box)
	}
	sword := findGroup(focusGroups(t, w2, "Sword"), "container")
	if len(sword) != 1 || sword[0].Surface != "within" {
		t.Fatalf("Sword container = %+v; want one item, surface 'within' (preserved)", sword)
	}
}

// Audit: for representative reciprocal pairs, declaring `A: <label> -> B` must
// converge on one edge whose reverse renders as the true inverse canonical on
// B. This locks the semantics a structurally-valid-but-inverted vocab (the old
// containment bug) would silently violate.
func TestRelationReciprocalSemanticsAudit(t *testing.T) {
	cases := []struct {
		label, fwdCanon, revCanon string
	}{
		{"father", "parent", "child"},
		{"members", "member", "member-of"},
		{"owner", "owner", "possession"},
		{"contains", "contents", "container"},
		{"within", "container", "contents"},
		{"leader", "leader", "follower"},
		// New pairs.
		{"ally", "ally", "ally"},
		{"enemy", "enemy", "enemy"},
		{"mentor", "mentor", "student"},
		{"teacher", "mentor", "student"},
		{"creator", "creator", "creation"},
		// Verb aliases must resolve to the side whose subject plays the verb's
		// role — the trap that inverts an edge if placed wrong.
		{"owns", "possession", "owner"},    // A owns B -> B is A's possession
		{"serves", "leader", "follower"},   // A serves B -> B is A's leader
		{"leads", "follower", "leader"},    // A leads B -> B is A's follower
		{"forged", "creation", "creator"},  // A forged B -> B is A's creation
		{"holds", "contents", "container"}, // A holds B -> A contains B
	}
	for _, c := range cases {
		world := setupTestWorld(t, "A (thing): "+c.label+" -> B\n\nB (thing): x.\n")
		fwd := findGroup(focusGroups(t, world, "A"), c.fwdCanon)
		if len(fwd) != 1 || fwd[0].Other != "B" || fwd[0].Incoming {
			t.Errorf("%s: A %s group = %+v; want outgoing to B", c.label, c.fwdCanon, fwd)
		}
		rev := findGroup(focusGroups(t, world, "B"), c.revCanon)
		if len(rev) != 1 || rev[0].Other != "A" {
			t.Errorf("%s: B reverse %s group = %+v; want [A]", c.label, c.revCanon, rev)
		}
	}
}

// A relation's plural resolves as an input label, so the natural
// `Cuthbert: children -> ...` works, not just `child -> ...`. The header on
// the multi-item group must stay "children", not double to "childrens".
func TestRelationPluralAsInputLabel(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations())
	if canon, known := v.Resolve("children"); !known || canon != "child" {
		t.Fatalf("Resolve(children) = %q,%v; want child,true", canon, known)
	}

	world := setupTestWorld(t, "Cuthbert (person): children -> Milly, Bobby, Sally\n\nMilly (person): x.\nBobby (person): x.\nSally (person): x.\n")
	kids := findGroup(focusGroups(t, world, "Cuthbert"), "child")
	if len(kids) != 3 {
		t.Fatalf("Cuthbert child group = %+v; want 3 items", kids)
	}
	// Reverse: each child sees Cuthbert as a parent.
	if p := findGroup(focusGroups(t, world, "Milly"), "parent"); len(p) != 1 || p[0].Other != "Cuthbert" {
		t.Fatalf("Milly parent = %+v; want [Cuthbert]", p)
	}

	// Header is the shared surface, left as the plural the author typed.
	block := FormatRelationsBlock(focusGroups(t, world, "Cuthbert"), v)
	if !strings.Contains(block, "children → ") || strings.Contains(block, "childrens") {
		t.Fatalf("header should read 'children →', not doubled; got %q", block)
	}
}

func TestRelationRemovalCancelsEdge(t *testing.T) {
	world := setupTestWorld(t, "Sarah (person): friend -> Mary; friend -/> Mary\n\nMary (person): x.\n")
	if g := findGroup(focusGroups(t, world, "Sarah"), "friend"); g != nil {
		t.Fatalf("Sarah friend should be empty after removal, got %+v", g)
	}
}

func TestEdgeRemovalIssues(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations())

	// Removing an edge that was never set warns.
	w1 := setupTestWorld(t, "Sarah (person): friend -/> Mary\n\nMary (person): x\n")
	if got := w1.EdgeRemovalIssues(v); len(got) != 1 {
		t.Fatalf("unset removal: want 1 issue, got %d: %+v", len(got), got)
	}

	// Add then remove: no warning.
	w2 := setupTestWorld(t, "Sarah (person): friend -> Mary; friend -/> Mary\n\nMary (person): x\n")
	if got := w2.EdgeRemovalIssues(v); len(got) != 0 {
		t.Fatalf("matched removal: want 0 issues, got %+v", got)
	}

	// Reciprocal-label removal of a set edge: no warning.
	w3 := setupTestWorld(t, "Sarah (person): father -> Doug\n\nDoug (person): daughter -/> Sarah\n")
	if got := w3.EdgeRemovalIssues(v); len(got) != 0 {
		t.Fatalf("reciprocal removal of set edge: want 0 issues, got %+v", got)
	}
}

func TestRelationInlineAsideTarget(t *testing.T) {
	// `father -> (Doug: daughter -> Sarah)`: one line, custom label each side.
	world := setupTestWorld(t, "Sarah (person): father -> (Doug (person): daughter -> Sarah)\n")
	v := NewRelationVocab(BuiltinRelations())

	doug, err := world.FindEntity("Doug")
	if err != nil {
		t.Fatalf("aside should define Doug: %v", err)
	}
	sarah, _ := world.FindEntity("Sarah")

	if g := findGroup(world.ResolveRelations(v, sarah), "parent"); len(g) != 1 || g[0].Other != "Doug" || g[0].Surface != "father" {
		t.Fatalf("Sarah parent = %+v; want one father -> Doug", g)
	}
	if g := findGroup(world.ResolveRelations(v, doug), "child"); len(g) != 1 || g[0].Other != "Sarah" || g[0].Surface != "daughter" {
		t.Fatalf("Doug child = %+v; want one daughter -> Sarah", g)
	}
}

func TestResolveAllRelationsForGraph(t *testing.T) {
	world := setupTestWorld(t, "Sarah (person): father -> Doug\n\nDoug (person): x\n\nParty (group): members -> Sarah\n")
	v := NewRelationVocab(BuiltinRelations())
	all := world.ResolveAllRelations(v)
	if len(all) != 2 {
		t.Fatalf("want 2 edges, got %d: %+v", len(all), all)
	}
	// Sorted by FromName: Doug, then Party.
	if all[0].FromName != "Doug" || all[0].ToName != "Sarah" || all[0].Label != "child" {
		t.Fatalf("edge0 = %+v; want Doug -> Sarah labelled child", all[0])
	}
	if all[0].FromType != "person" || all[0].ToType != "person" {
		t.Fatalf("edge0 types = %q/%q; want person/person", all[0].FromType, all[0].ToType)
	}
	if all[1].FromName != "Party" || all[1].ToName != "Sarah" || all[1].Label != "members" {
		t.Fatalf("edge1 = %+v; want Party -> Sarah labelled members", all[1])
	}
}

func TestRelationResolvesAtCursor(t *testing.T) {
	// friend added on line 1, removed on line 3. Hovering at line 1 should
	// still see the friendship; at line 3 (after the removal) it's gone.
	world := setupTestWorld(t, "Sarah (person): friend -> Mary\n\nMary (person): ok\n\nSarah (person): friend -/> Mary\n")
	v := NewRelationVocab(BuiltinRelations())
	sarah, _ := world.FindEntity("Sarah")

	at1 := world.ResolveRelationsAt(v, sarah, world.FileOrder, "test.md", 1)
	if findGroup(at1, "friend") == nil {
		t.Fatalf("at line 1 Sarah should still be friends with Mary: %+v", at1)
	}

	at5 := world.ResolveRelationsAt(v, sarah, world.FileOrder, "test.md", 5)
	if findGroup(at5, "friend") != nil {
		t.Fatalf("at line 5 the friendship is removed: %+v", at5)
	}
}

func TestRelationRemovalFromReciprocalLabel(t *testing.T) {
	// father (Sarah->Doug) then daughter -/> (Doug->Sarah) must cancel the
	// same canonical edge.
	world := setupTestWorld(t, "Sarah (person): father -> Doug\n\nDoug (person): daughter -/> Sarah\n")
	if g := findGroup(focusGroups(t, world, "Doug"), "child"); g != nil {
		t.Fatalf("edge should be cancelled by reciprocal-label removal, got %+v", g)
	}
	if g := findGroup(focusGroups(t, world, "Sarah"), "parent"); g != nil {
		t.Fatalf("Sarah side should also be gone, got %+v", g)
	}
}
