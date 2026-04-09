package lore

import "testing"

func TestDisambiguateEntitiesWithSameNameDifferentTypes(t *testing.T) {
	world := setupTestWorld(t, "Barovia (town): Gothic, dark, misty, rundown.\n\nBarovia (nation): Perpetually cloudy. Nobody can leave.\n")

	if len(world.Entities) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(world.Entities))
	}

	town, err := world.FindEntity("Barovia (town)")
	if err != nil {
		t.Fatal(err)
	}
	if town.Type != "town" {
		t.Fatalf("type = %q", town.Type)
	}
	if !ContainsIgnoreCase(town.Descriptions[0].Text, "Gothic") {
		t.Fatal("town should contain Gothic")
	}

	nation, err := world.FindEntity("Barovia (nation)")
	if err != nil {
		t.Fatal(err)
	}
	if nation.Type != "nation" {
		t.Fatalf("type = %q", nation.Type)
	}
	if !ContainsIgnoreCase(nation.Descriptions[0].Text, "cloudy") {
		t.Fatal("nation should contain cloudy")
	}
}

func TestDisambiguatedReferencesResolve(t *testing.T) {
	world := setupTestWorld(t, "Barovia (town): The main town.\n\nBarovia (nation): The country.\n\nWe entered Barovia (town) from the west.\n")

	town, _ := world.FindEntity("Barovia (town)")
	refs := world.GetReferences(town.Name)

	found := false
	for _, ref := range refs {
		if ContainsIgnoreCase(ref.Context, "entered") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected free text reference to Barovia (town)")
	}
}

func TestGetReferencesExcludesSelfReferences(t *testing.T) {
	world := setupTestWorld(t, `Sildar Hallwinter (character) | Sildar: Fighter. Member of the Lords Alliance.

Cragmaw Hideout (location): North of Triboar Trail. Sildar was captured here.

Sildar: Told us Gundren was taken to Cragmaw Castle.
`)

	refs := world.GetReferences("Sildar Hallwinter")
	for _, ref := range refs {
		if ref.SourceEntity == "Sildar Hallwinter" {
			t.Errorf("self-reference not filtered: %s:%d %q", ref.File, ref.Line, ref.Context)
		}
	}
	// Should still include the cross-reference from Cragmaw Hideout.
	found := false
	for _, ref := range refs {
		if ref.SourceEntity == "Cragmaw Hideout" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected cross-reference from Cragmaw Hideout")
	}
}

func TestGetReferencesExcludesSelfReferencesByAlias(t *testing.T) {
	world := setupTestWorld(t, `Sildar Hallwinter (character) | Sildar: Fighter.

We met Sildar at the tavern.
`)

	// Look up by alias — self-refs should still be excluded.
	refs := world.GetReferences("Sildar Hallwinter")
	for _, ref := range refs {
		if ref.SourceEntity == "Sildar Hallwinter" {
			t.Errorf("self-reference not filtered: %s:%d %q", ref.File, ref.Line, ref.Context)
		}
	}
	// Free text reference should remain.
	found := false
	for _, ref := range refs {
		if ref.SourceEntity == "" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected free text reference")
	}
}

func TestFindEntityAmbiguous(t *testing.T) {
	world := setupTestWorld(t, "Barovia (town): The main town.\n\nBarovia (nation): The country.\n")

	_, err := world.FindEntity("Barovia")
	if err == nil {
		t.Fatal("expected error for ambiguous lookup")
	}
	ambig, ok := err.(*AmbiguousError)
	if !ok {
		t.Fatalf("expected AmbiguousError, got %T", err)
	}
	if len(ambig.Matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(ambig.Matches))
	}

	if _, err := world.FindEntity("Barovia (town)"); err != nil {
		t.Fatalf("disambiguated lookup should work: %v", err)
	}
	if _, err := world.FindEntity("Barovia (nation)"); err != nil {
		t.Fatalf("disambiguated lookup should work: %v", err)
	}

	_, err = world.FindEntity("Barovia (city)")
	if err != ErrNotFound {
		t.Fatal("expected ErrNotFound for unknown type")
	}
	_, err = world.FindEntity("Neverwinter")
	if err != ErrNotFound {
		t.Fatal("expected ErrNotFound for unknown entity")
	}
}
