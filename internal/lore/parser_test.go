package lore

import "testing"

func TestParseHeaderWithTypeAndAlias(t *testing.T) {
	h, ok := ParseHeader("Sildar Hallwinter (character) | Sildar: Fighter.")
	if !ok {
		t.Fatal("expected ok")
	}
	if h.Name != "Sildar Hallwinter" {
		t.Fatalf("name = %q", h.Name)
	}
	if h.Type != "character" {
		t.Fatalf("type = %q", h.Type)
	}
	if len(h.Aliases) != 1 || h.Aliases[0] != "Sildar" {
		t.Fatalf("aliases = %v", h.Aliases)
	}
	if h.DescStart != "Fighter." {
		t.Fatalf("desc = %q", h.DescStart)
	}
}

func TestParseHeaderTypeAtStart(t *testing.T) {
	h, ok := ParseHeader("(location) Cragmaw Hideout: North of trail.")
	if !ok {
		t.Fatal("expected ok")
	}
	if h.Name != "Cragmaw Hideout" {
		t.Fatalf("name = %q", h.Name)
	}
	if h.Type != "location" {
		t.Fatalf("type = %q", h.Type)
	}
}

func TestParseHeaderTypeInMiddle(t *testing.T) {
	h, ok := ParseHeader("Count Strahd (character) | Strahd: Vampire.")
	if !ok {
		t.Fatal("expected ok")
	}
	if h.Name != "Count Strahd" {
		t.Fatalf("name = %q", h.Name)
	}
	if len(h.Aliases) != 1 || h.Aliases[0] != "Strahd" {
		t.Fatalf("aliases = %v", h.Aliases)
	}
}

func TestParseHeaderUntyped(t *testing.T) {
	h, ok := ParseHeader("Sildar: Was captured at Cragmaw Hideout.")
	if !ok {
		t.Fatal("expected ok")
	}
	if h.Name != "Sildar" || h.Type != "" {
		t.Fatalf("got %+v", h)
	}
}

func TestParseHeaderNoColon(t *testing.T) {
	if _, ok := ParseHeader("No colon here"); ok {
		t.Fatal("expected not ok without colon")
	}
}

func TestParseSingleFileWithEntities(t *testing.T) {
	project := setupTestProject(t, map[string]string{
		"session.md": "Gundren (character) | Gundren Rockseeker: A dwarf merchant.\n  Hired us to deliver supplies.\n\nPhandalin (location): A small frontier town.\n",
	})

	world, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	if len(world.Entities) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(world.Entities))
	}

	gundren, err := world.FindEntity("Gundren")
	if err != nil {
		t.Fatal(err)
	}
	if gundren.Name != "Gundren" {
		t.Fatalf("name = %q", gundren.Name)
	}
	if gundren.Type != "character" {
		t.Fatalf("type = %q", gundren.Type)
	}
	if len(gundren.Aliases) != 1 || gundren.Aliases[0] != "Gundren Rockseeker" {
		t.Fatalf("aliases = %v", gundren.Aliases)
	}

	phandalin, err := world.FindEntity("Phandalin")
	if err != nil {
		t.Fatal(err)
	}
	if phandalin.Type != "location" {
		t.Fatalf("type = %q", phandalin.Type)
	}
}

func TestParseEntityLookupByAlias(t *testing.T) {
	project := setupTestProject(t, map[string]string{
		"test.md": "Count Strahd von Zarovich (character) | Strahd: Vampire lord.\n",
	})

	world, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	ent, err := world.FindEntity("Strahd")
	if err != nil {
		t.Fatal(err)
	}
	if ent.Name != "Count Strahd von Zarovich" {
		t.Fatalf("name = %q", ent.Name)
	}
}

func TestParseMultiFileEntityAccumulation(t *testing.T) {
	project := setupTestProject(t, map[string]string{
		"glossary.md": "Sildar (character): Fighter.\n",
		"session.md":  "Sildar: Was captured at Cragmaw Hideout.\n",
	})

	world, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	if len(world.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(world.Entities))
	}
	sildar, err := world.FindEntity("Sildar")
	if err != nil {
		t.Fatal(err)
	}
	if len(sildar.Descriptions) != 2 {
		t.Fatalf("expected 2 descriptions, got %d", len(sildar.Descriptions))
	}
}

func TestParseCrossReferences(t *testing.T) {
	project := setupTestProject(t, map[string]string{
		"test.md": "Gundren (character): A dwarf.\n\nPhandalin (location): Where Gundren sent us.\n",
	})

	world, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	refs := world.GetReferences("Gundren")
	if len(refs) == 0 {
		t.Fatal("expected references to Gundren")
	}
}

func TestParseSearchFindsText(t *testing.T) {
	project := setupTestProject(t, map[string]string{
		"test.md": "Gundren (character): A dwarf merchant from Neverwinter.\n",
	})

	world, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	results := world.Search("Neverwinter")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestParseBlankLineEndsDescription(t *testing.T) {
	project := setupTestProject(t, map[string]string{
		"test.md": "Gundren (character): A dwarf.\n  More about Gundren.\n\nThis is free text, not part of Gundren's description.\n",
	})

	world, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	gundren, _ := world.FindEntity("Gundren")
	if len(gundren.Descriptions) != 1 {
		t.Fatalf("expected 1 description, got %d", len(gundren.Descriptions))
	}
	if !ContainsIgnoreCase(gundren.Descriptions[0].Text, "More about") {
		t.Fatal("expected description to contain continuation")
	}
	if ContainsIgnoreCase(gundren.Descriptions[0].Text, "free text") {
		t.Fatal("description should not contain free text after blank line")
	}
}

func TestParseFilesSortedAlphabetically(t *testing.T) {
	project := setupTestProject(t, map[string]string{
		"b-second.md": "Sildar (character): From the second file.\n",
		"a-first.md":  "Gundren (character): From the first file.\n",
	})

	world, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	if world.Entities[0].Name != "Gundren" {
		t.Fatalf("expected Gundren first, got %q", world.Entities[0].Name)
	}
	if world.Entities[1].Name != "Sildar" {
		t.Fatalf("expected Sildar second, got %q", world.Entities[1].Name)
	}
}

func TestParseMarkdownHeadersIgnored(t *testing.T) {
	project := setupTestProject(t, map[string]string{
		"test.md": "# Session 1\n\nGundren (character): A dwarf.\n",
	})

	world, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	if len(world.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(world.Entities))
	}
}

func TestDisambiguatedReferenceToleratesSpacing(t *testing.T) {
	project := setupTestProject(t, map[string]string{
		"test.md": "Barovia (town): A village.\n\nBarovia (nation): A gothic land.\n\nWe entered Barovia(town) at dusk.\nLater we arrived in Barovia   (nation).\nFinally we saw Barovia ( town ) from the hills.\n",
	})

	world, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	town, err := world.FindEntity("Barovia (town)")
	if err != nil {
		t.Fatal(err)
	}
	nation, err := world.FindEntity("Barovia (nation)")
	if err != nil {
		t.Fatal(err)
	}

	tight := false
	padded := false
	for _, ref := range world.GetReferences(town.Name) {
		if ContainsIgnoreCase(ref.Context, "Barovia(town)") {
			tight = true
		}
		if ContainsIgnoreCase(ref.Context, "Barovia ( town )") {
			padded = true
		}
	}
	if !tight {
		t.Fatal("expected 'Barovia(town)' to be recognised as a disambiguated reference")
	}
	if !padded {
		t.Fatal("expected 'Barovia ( town )' to be recognised as a disambiguated reference")
	}

	foundNation := false
	for _, ref := range world.GetReferences(nation.Name) {
		if ContainsIgnoreCase(ref.Context, "Barovia   (nation)") {
			foundNation = true
		}
	}
	if !foundNation {
		t.Fatal("expected 'Barovia   (nation)' to be recognised as a disambiguated reference")
	}
}

func TestReferencesRespectWordBoundaries(t *testing.T) {
	project := setupTestProject(t, map[string]string{
		"test.md": "Pip (character): A halfling rogue.\n\nThe soup was piping hot.\n\nPip waved.\n",
	})

	world, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	refs := world.GetReferences("Pip")
	for _, ref := range refs {
		if ContainsIgnoreCase(ref.Context, "piping") {
			t.Fatalf("ref should not match 'piping': %+v", ref)
		}
	}
	if len(refs) == 0 {
		t.Fatal("expected at least one reference to Pip (from 'Pip waved.')")
	}
}

func TestKnownEntityRedefinitionWithoutType(t *testing.T) {
	world := setupTestWorld(t, "Strahd (character): Vampire lord.\n\nStrahd: Showed up at the funeral with flowers.\n")

	if len(world.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(world.Entities))
	}

	strahd, _ := world.FindEntity("Strahd")
	if len(strahd.Descriptions) != 2 {
		t.Fatalf("expected 2 descriptions, got %d", len(strahd.Descriptions))
	}
}
