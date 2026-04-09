package lore

import "testing"

func TestParseEntityHeaderWithTypeAndAlias(t *testing.T) {
	h, ok := parseEntityHeader("Sildar Hallwinter (character) | Sildar: Fighter.")
	if !ok {
		t.Fatal("expected ok")
	}
	if h.name != "Sildar Hallwinter" {
		t.Fatalf("name = %q", h.name)
	}
	if h.entityType != "character" {
		t.Fatalf("type = %q", h.entityType)
	}
	if len(h.aliases) != 1 || h.aliases[0] != "Sildar" {
		t.Fatalf("aliases = %v", h.aliases)
	}
	if h.descriptionStart != "Fighter." {
		t.Fatalf("desc = %q", h.descriptionStart)
	}
}

func TestParseEntityHeaderTypeAtStart(t *testing.T) {
	h, ok := parseEntityHeader("(location) Cragmaw Hideout: North of trail.")
	if !ok {
		t.Fatal("expected ok")
	}
	if h.name != "Cragmaw Hideout" {
		t.Fatalf("name = %q", h.name)
	}
	if h.entityType != "location" {
		t.Fatalf("type = %q", h.entityType)
	}
}

func TestParseEntityHeaderTypeInMiddle(t *testing.T) {
	h, ok := parseEntityHeader("Count Strahd (character) | Strahd: Vampire.")
	if !ok {
		t.Fatal("expected ok")
	}
	if h.name != "Count Strahd" {
		t.Fatalf("name = %q", h.name)
	}
	if len(h.aliases) != 1 || h.aliases[0] != "Strahd" {
		t.Fatalf("aliases = %v", h.aliases)
	}
}

func TestParseEntityHeaderNoType(t *testing.T) {
	if _, ok := parseEntityHeader("Just some text with a colon: here"); ok {
		t.Fatal("expected not ok without type")
	}
}

func TestParseEntityHeaderNoColon(t *testing.T) {
	if _, ok := parseEntityHeader("No colon here"); ok {
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
