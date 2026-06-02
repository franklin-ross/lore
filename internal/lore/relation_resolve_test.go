package lore

import "testing"

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
	aragorn := findGroup(focusGroups(t, world, "Aragorn"), "memberof")
	if len(aragorn) != 1 || aragorn[0].Other != "Party" {
		t.Fatalf("Aragorn memberof = %+v; want [Party]", aragorn)
	}
}

func TestRelationRemovalCancelsEdge(t *testing.T) {
	world := setupTestWorld(t, "Sarah (person): friend -> Mary; friend -/> Mary\n\nMary (person): x.\n")
	if g := findGroup(focusGroups(t, world, "Sarah"), "friend"); g != nil {
		t.Fatalf("Sarah friend should be empty after removal, got %+v", g)
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
