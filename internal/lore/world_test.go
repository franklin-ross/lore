package lore

import (
	"strings"
	"testing"
)

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

// References inside an inline aside should attribute to the aside-defined
// entity, not the surrounding line owner. Without per-position attribution
// every match on the line collapses to a single SourceEntity, making the
// wiki's outbound list show targets that aren't actually mentioned in the
// owner's prose.
func TestReferenceAttributionRespectsInlineAsideRanges(t *testing.T) {
	world := setupTestWorld(t, `Captain Casimir (npc) | Casimir: Dusk elf.

Patrina Vilikavna (npc) | Patrina: Sister of Casimir.

Vallaki (location): Walled town.

Vistani (faction): Wandering folk.

Yltry (character): A wizard.

Yltry meets (Captain Casimir (npc) | Casimir: location = Vistani Camp near Vallaki. A dusk elf with ties to the Vistani.). He's having bad dreams.
`)

	// Refs to Vallaki should come from inside Casimir's aside (line 11),
	// attributed to Casimir — not bleeding into Yltry's outbound list.
	for _, target := range []string{"Vallaki", "Vistani"} {
		refs := world.GetReferences(target)
		var sourcesAtAside []string
		for _, r := range refs {
			if r.Line == 11 {
				sourcesAtAside = append(sourcesAtAside, r.SourceEntity)
			}
		}
		if len(sourcesAtAside) == 0 {
			t.Fatalf("expected line-11 ref to %s; got %+v", target, refs)
		}
		for _, src := range sourcesAtAside {
			if src != "Captain Casimir" {
				t.Errorf("line-11 ref to %s attributed to %q, want Captain Casimir", target, src)
			}
		}
	}

	// Yltry is mentioned outside the aside — attribution should be free
	// text (empty SourceEntity), never Casimir.
	yltryRefs := world.GetReferences("Yltry")
	for _, r := range yltryRefs {
		if r.Line == 11 && r.SourceEntity == "Captain Casimir" {
			t.Errorf("Yltry mention on line 11 wrongly attributed to Captain Casimir: %q", r.Context)
		}
	}
}

// When an entity has an alias that's a substring of its canonical name
// (e.g. "Casimir" alias for "Captain Casimir"), a single occurrence in
// prose matches both the name and the alias scanner. The greedy overlap
// resolution should keep only the longer span so the wiki shows one ref
// per mention, not two.
func TestReferenceAttributionDeduplicatesAliasInsideName(t *testing.T) {
	world := setupTestWorld(t, `Captain Casimir (npc) | Casimir: Dusk elf.

Amber Temple (location): Captain Casimir wants to investigate it with us.
`)

	refs := world.GetReferences("Captain Casimir")
	count := 0
	for _, r := range refs {
		if r.Line == 3 && r.SourceEntity == "Amber Temple" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one ref to Captain Casimir on line 3 (name + alias overlap), got %d: %+v", count, refs)
	}
}

// When a longer entity name like "Vistani Camp near Vallaki" subsumes
// the words of shorter entities ("Vistani", "Vallaki") at the same span,
// only the longest match should fire. Otherwise the wiki shows phantom
// refs to Vistani and Vallaki at every mention of the place's full name.
func TestReferenceAttributionPrefersLongestEntityNameAtOverlap(t *testing.T) {
	world := setupTestWorld(t, `Vistani (faction): Wandering folk.

Vallaki (location): Walled town.

Vistani Camp near Vallaki (location): A camp.

Casimir (npc): Lives at Vistani Camp near Vallaki.
`)

	// Casimir's description mentions "Vistani Camp near Vallaki" exactly
	// once, so the place itself should have one ref — and the bare
	// Vistani/Vallaki entities should have zero from that span.
	for target, want := range map[string]int{
		"Vistani Camp near Vallaki": 1,
		"Vistani":                   0,
		"Vallaki":                   0,
	} {
		got := 0
		for _, r := range world.GetReferences(target) {
			if r.SourceEntity == "Casimir" {
				got++
			}
		}
		if got != want {
			t.Errorf("Casimir → %s: want %d ref, got %d", target, want, got)
		}
	}
}

// An aside's header (Name (type) | Alias: portion) reads naturally as
// part of the surrounding sentence. References to the aside-defined
// entity that appear in the header itself should attribute to free
// text, not to the aside entity (which would otherwise self-mention
// every aside header). Body content remains the entity's territory.
func TestReferenceAttributionAsideHeaderIsFreeText(t *testing.T) {
	world := setupTestWorld(t, `Yltry (character): A wizard.

Yltry meets (Captain Casimir (npc) | Casimir: Dusk elf with ties to the Vistani.). He had bad dreams.

Vistani (faction): Wandering folk.
`)

	// "Captain Casimir" in the aside header → free text mention.
	refs := world.GetReferences("Captain Casimir")
	var line3Sources []string
	for _, r := range refs {
		if r.Line == 3 {
			line3Sources = append(line3Sources, r.SourceEntity)
		}
	}
	if len(line3Sources) == 0 {
		t.Fatalf("expected line-3 ref to Captain Casimir; got %+v", refs)
	}
	for _, src := range line3Sources {
		if src != "" {
			t.Errorf("aside-header ref to Captain Casimir attributed to %q, want free text", src)
		}
	}

	// "Vistani" in the aside body → Captain Casimir's outbound.
	for _, r := range world.GetReferences("Vistani") {
		if r.Line == 3 && r.SourceEntity != "Captain Casimir" {
			t.Errorf("aside-body ref to Vistani attributed to %q, want Captain Casimir", r.SourceEntity)
		}
	}
}

// Long sentences containing multiple references would otherwise render
// as identical rows in the wiki. Trimming the Context to a few words
// before each match disambiguates them visually.
func TestReferenceContextTrimsToFourWordsBeforeMatch(t *testing.T) {
	world := setupTestWorld(t, `Yltry (character): A wizard.

Vallaki (location): Walled town.

Casimir (npc): Yltry meets the dusk elf at the gates of Vallaki on a misty morning.
`)

	refs := world.GetReferences("Vallaki")
	var ctx string
	for _, r := range refs {
		if r.SourceEntity == "Casimir" {
			ctx = r.Context
		}
	}
	if ctx == "" {
		t.Fatal("expected ref to Vallaki from Casimir")
	}
	// Match position is "Vallaki" near the end; 4 words before is
	// "the gates of Vallaki" — leading prose should be elided.
	if !strings.HasPrefix(ctx, "… ") {
		t.Errorf("expected leading ellipsis, got %q", ctx)
	}
	if !strings.Contains(ctx, "Vallaki") {
		t.Errorf("trimmed context lost the match: %q", ctx)
	}
}

func TestReferenceContextNoTrimWhenShort(t *testing.T) {
	world := setupTestWorld(t, `Vallaki (location): Walled town.

Casimir (npc): Met Vallaki guards.
`)
	for _, r := range world.GetReferences("Vallaki") {
		if r.SourceEntity != "Casimir" {
			continue
		}
		if strings.HasPrefix(r.Context, "… ") {
			t.Errorf("short context should not be trimmed: %q", r.Context)
		}
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
