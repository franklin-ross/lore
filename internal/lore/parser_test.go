package lore

import (
	"strings"
	"testing"
)

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

func TestParseHeaderRejectsMidSentenceParens(t *testing.T) {
	// A prose line that happens to contain a parenthesised aside and a later
	// colon must not be misread as a typed entity definition.
	h, ok := ParseHeader("We took shelter (it was raining) and waited: nobody came.")
	if !ok {
		t.Fatal("expected untyped header (lookup name) for prose-with-colon line")
	}
	if h.Type != "" {
		t.Fatalf("expected no type, got %q", h.Type)
	}
	if h.Name != "We took shelter (it was raining) and waited" {
		t.Fatalf("name = %q", h.Name)
	}
}

func TestParseHeaderProseEntityNotCreated(t *testing.T) {
	// End-to-end: a free-text line with parens-then-colon must not become an
	// entity. Previously the parser extracted "it was raining" as a type and
	// created an entity called "We took shelter".
	world := setupTestWorld(t, "We took shelter (it was raining) and waited: nobody came.\n")
	if len(world.Entities) != 0 {
		t.Fatalf("expected 0 entities, got %d: %+v", len(world.Entities), world.Entities)
	}
}

func TestParseHeaderNoColon(t *testing.T) {
	if _, ok := ParseHeader("No colon here"); ok {
		t.Fatal("expected not ok without colon")
	}
}

func TestLocateNameInHeader(t *testing.T) {
	cases := []struct {
		header, target  string
		wantStart, want int
		ok              bool
	}{
		{"Sildar Hallwinter (character) | Sildar", "Sildar Hallwinter", 0, 17, true},
		{"Sildar Hallwinter (character) | Sildar", "Sildar", 32, 38, true},
		{"(location) Cragmaw Hideout", "Cragmaw Hideout", 11, 26, true},
		{"Cragmaw Hideout", "Cragmaw Hideout", 0, 15, true},
		{"Sildar Hallwinter (character) | Sildar", "Mary", 0, 0, false},
		{"  Padded  (type)  |  Alias  ", "Padded", 2, 8, true},
		{"  Padded  (type)  |  Alias  ", "Alias", 21, 26, true},
	}
	for _, tc := range cases {
		gotStart, gotEnd, ok := LocateNameInHeader(tc.header, tc.target)
		if ok != tc.ok {
			t.Errorf("LocateNameInHeader(%q,%q) ok=%v want %v", tc.header, tc.target, ok, tc.ok)
			continue
		}
		if !ok {
			continue
		}
		if gotStart != tc.wantStart || gotEnd != tc.want {
			t.Errorf("LocateNameInHeader(%q,%q) = [%d,%d) want [%d,%d)", tc.header, tc.target, gotStart, gotEnd, tc.wantStart, tc.want)
		}
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

func TestParseDescriptionCapturesDirectives(t *testing.T) {
	world := setupTestWorld(t, "Sildar (character): Fighter. +injured\n")
	sildar, err := world.FindEntity("Sildar")
	if err != nil {
		t.Fatal(err)
	}
	if len(sildar.Descriptions) != 1 {
		t.Fatalf("descriptions: %+v", sildar.Descriptions)
	}
	if len(sildar.Descriptions[0].Events) != 1 {
		t.Fatalf("events: %+v", sildar.Descriptions[0].Events)
	}
	ev := sildar.Descriptions[0].Events[0]
	if ev.Op != StateOpAdd || ev.Target != "injured" {
		t.Fatalf("event: %+v", ev)
	}
	if ev.Span.File != "test.md" {
		t.Fatalf("span file: %+v", ev.Span)
	}
}

func TestMergeResolvesEntityStateNumericAccrossFiles(t *testing.T) {
	project := setupTestProject(t, map[string]string{
		"a-first.md":  "Phandalin (location): Sleepy. population = 100\n",
		"b-second.md": "Phandalin: Raided. population -= 50\n",
	})
	world, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}
	p, err := world.FindEntity("Phandalin")
	if err != nil {
		t.Fatal(err)
	}
	pop, ok := p.Fields["population"]
	if !ok || pop.Number != 50 {
		t.Fatalf("population: %+v", p.Fields)
	}
	if len(p.StateIssues) != 0 {
		t.Fatalf("issues: %+v", p.StateIssues)
	}
}

func TestMergeResolvesEntityTagsAcrossFiles(t *testing.T) {
	project := setupTestProject(t, map[string]string{
		"a-intro.md":   "Sildar (character): Fighter. +injured\n",
		"b-session.md": "Sildar: Patched up. -injured\n",
	})
	world, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}
	sildar, _ := world.FindEntity("Sildar")
	if sildar.Tags["injured"] {
		t.Fatalf("expected tag to be removed, got: %+v", sildar.Tags)
	}
}

func TestMergeSurfacesStateIssues(t *testing.T) {
	world := setupTestWorld(t, "Sildar (character): Fighter. -injured\n")
	sildar, _ := world.FindEntity("Sildar")
	if len(sildar.StateIssues) != 1 {
		t.Fatalf("issues: %+v", sildar.StateIssues)
	}
}

func TestMergeCapturesStateHistory(t *testing.T) {
	world := setupTestWorld(t, "Phandalin (location): Town. population = 100 population += 50\n")
	p, _ := world.FindEntity("Phandalin")
	if len(p.StateHistory) != 2 {
		t.Fatalf("history: %+v", p.StateHistory)
	}
}

func TestMergeBarewordFieldValueKeepsSlashesAndParens(t *testing.T) {
	world := setupTestWorld(t, "Session 01 (session):\n  date = 2026/02/01\n  location = Barovia (town)\n")
	sess, err := world.FindEntity("Session 01")
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.StateIssues) != 0 {
		t.Fatalf("unexpected state issues: %+v", sess.StateIssues)
	}
	date, ok := sess.Fields["date"]
	if !ok || date.Kind != FieldText || len(date.Text) != 1 || date.Text[0] != "2026/02/01" {
		t.Fatalf("date field: %+v", sess.Fields)
	}
	loc, ok := sess.Fields["location"]
	if !ok || loc.Kind != FieldText || len(loc.Text) != 1 || loc.Text[0] != "Barovia (town)" {
		t.Fatalf("location field: %+v", sess.Fields)
	}
}

func TestMergeNewlineTerminatesDirectiveValue(t *testing.T) {
	// A description that spans two lines should not let the first line's
	// text value leak into the second line. The joiner must be a newline so
	// `date = "2026-02-01"` terminates cleanly before `location = Barovia`
	// starts — missing a trailing comma on line 1 is not an error.
	world := setupTestWorld(t, "Session 01 (session):\n  date = \"2026-02-01\"\n  location = Barovia\n")
	sess, err := world.FindEntity("Session 01")
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.StateIssues) != 0 {
		t.Fatalf("unexpected state issues: %+v", sess.StateIssues)
	}
	date, ok := sess.Fields["date"]
	if !ok || date.Kind != FieldText || len(date.Text) != 1 || date.Text[0] != "2026-02-01" {
		t.Fatalf("date field: %+v", sess.Fields)
	}
	loc, ok := sess.Fields["location"]
	if !ok || loc.Kind != FieldText || len(loc.Text) != 1 || loc.Text[0] != "Barovia" {
		t.Fatalf("location field: %+v", sess.Fields)
	}
}

func TestMergeDirectiveSpansMapToOriginalLineAndColumn(t *testing.T) {
	// The description spans two lines. The directive `+injured` appears on
	// the continuation line (line 2 in the file). After merge, the event's
	// span must reflect that real file line and the column where +injured
	// starts on that line — NOT the header line or a byte offset into the
	// joined description.
	content := "Sildar (character): Fighter.\n" +
		"  Took an arrow. +injured\n"
	world := setupTestWorld(t, content)
	sildar, err := world.FindEntity("Sildar")
	if err != nil {
		t.Fatal(err)
	}
	if len(sildar.StateHistory) != 1 {
		t.Fatalf("history: %+v", sildar.StateHistory)
	}
	ev := sildar.StateHistory[0]
	if ev.Target != "injured" {
		t.Fatalf("target: %q", ev.Target)
	}
	if ev.Span.Line != 2 {
		t.Fatalf("line: %d, want 2 (continuation line)", ev.Span.Line)
	}
	// The continuation line is "  Took an arrow. +injured".
	// Leading whitespace is 2 bytes, "Took an arrow. " is 15 bytes,
	// so '+' sits at column 17 (0-based).
	const wantCol = 17
	if ev.Span.StartByte != wantCol {
		t.Fatalf("start byte: %d, want %d", ev.Span.StartByte, wantCol)
	}
	if ev.Span.EndByte != wantCol+len("+injured") {
		t.Fatalf("end byte: %d, want %d", ev.Span.EndByte, wantCol+len("+injured"))
	}
}

func TestMergeDirectiveSpansOnHeaderLine(t *testing.T) {
	// A directive on the header line itself should have Line = 1 and
	// StartByte pointing at its column in the header line.
	content := "Sildar (character): +injured Fighter.\n"
	world := setupTestWorld(t, content)
	sildar, _ := world.FindEntity("Sildar")
	if len(sildar.StateHistory) != 1 {
		t.Fatalf("history: %+v", sildar.StateHistory)
	}
	ev := sildar.StateHistory[0]
	if ev.Span.Line != 1 {
		t.Fatalf("line: %d", ev.Span.Line)
	}
	// The header is "Sildar (character): +injured Fighter."
	// Colon + space at column 19, so '+' is at column 20.
	const wantCol = 20
	if ev.Span.StartByte != wantCol {
		t.Fatalf("start byte: %d, want %d", ev.Span.StartByte, wantCol)
	}
}

func TestMergeInlineAsideRoutesToNamedEntity(t *testing.T) {
	// An inline `(Strahd: ...)` aside inside Ireena's description should
	// land in Strahd's StateHistory, not Ireena's. Strahd's resolved state
	// reflects the directive; Ireena's does not.
	content := "Strahd (character): Vampire lord. hp = 100\n" +
		"\n" +
		"Ireena (character): She fled the keep (Strahd: hp -= 5; +smitten) and ran for cover.\n"
	world := setupTestWorld(t, content)
	strahd, err := world.FindEntity("Strahd")
	if err != nil {
		t.Fatal(err)
	}
	ireena, err := world.FindEntity("Ireena")
	if err != nil {
		t.Fatal(err)
	}
	if len(ireena.StateHistory) != 0 {
		t.Fatalf("ireena should have no events: %+v", ireena.StateHistory)
	}
	if len(strahd.StateHistory) != 3 {
		t.Fatalf("strahd history: %+v", strahd.StateHistory)
	}
	if !strahd.Tags["smitten"] {
		t.Fatalf("strahd tags: %+v", strahd.Tags)
	}
	hp, ok := strahd.Fields["hp"]
	if !ok || hp.Kind != FieldNumeric || hp.Number != 95 {
		t.Fatalf("strahd hp: %+v", strahd.Fields)
	}
	// Ireena's clean text replaces the aside with the subject's name.
	if len(ireena.Descriptions) != 1 {
		t.Fatalf("ireena descriptions: %+v", ireena.Descriptions)
	}
	clean := ireena.Descriptions[0].CleanText
	if clean != "She fled the keep Strahd and ran for cover." {
		t.Fatalf("clean text: %q", clean)
	}
}

func TestMergeInlineAsideInFreeProseIntroducesTypedEntity(t *testing.T) {
	// Asides with a typed subject act like inline header definitions even
	// when written in free prose between two real definitions.
	content := "Strahd (character): Vampire lord.\n" +
		"\n" +
		"We meet a crying woman. (Mad Mary (npc): old lady, daughter Gertrude is missing.) Tells us things.\n" +
		"\n" +
		"Castle Ravenloft (location): Strahd's castle.\n"
	world := setupTestWorld(t, content)

	mary, err := world.FindEntity("Mad Mary")
	if err != nil {
		t.Fatalf("Mad Mary not found: %v", err)
	}
	if mary.Type != "npc" {
		t.Fatalf("type = %q, want npc", mary.Type)
	}
	if len(mary.Descriptions) != 1 {
		t.Fatalf("descs: %+v", mary.Descriptions)
	}
	if !strings.Contains(mary.Descriptions[0].Text, "old lady") {
		t.Fatalf("desc text: %q", mary.Descriptions[0].Text)
	}
}

func TestMergeInlineAsideRoutesDirectives(t *testing.T) {
	// `(Strahd: hp -= 5)` in some other entity's body must route the
	// directive to Strahd, not the owner. The owner's clean text loses the
	// aside; the owner's events list does not include the inner directive.
	content := "Strahd (character): hp = 100\n" +
		"\n" +
		"Ireena (character): She fled (Strahd: hp -= 5; +smitten) and ran.\n"
	world := setupTestWorld(t, content)
	strahd, _ := world.FindEntity("Strahd")
	ireena, _ := world.FindEntity("Ireena")
	hp := strahd.Fields["hp"]
	if hp.Number != 95 {
		t.Fatalf("strahd hp = %v", hp.Number)
	}
	if !strahd.Tags["smitten"] {
		t.Fatalf("strahd tags: %+v", strahd.Tags)
	}
	for _, ev := range ireena.StateHistory {
		if ev.Target == "hp" || ev.Target == "smitten" {
			t.Fatalf("ireena event leaked: %+v", ev)
		}
	}
	if ireena.Descriptions[0].CleanText != "She fled Strahd and ran." {
		t.Fatalf("ireena clean: %q", ireena.Descriptions[0].CleanText)
	}
}

func TestMergeInlineAsideUnknownSubjectSilentlyDropped(t *testing.T) {
	// An aside whose subject doesn't resolve to any entity behaves the same
	// way a free-floating untyped header does: it's silently skipped at
	// merge. The owner's CleanText still substitutes the bare subject name —
	// substitution is syntactic and doesn't depend on resolution.
	content := "Ireena (character): She murmured (Nobody: hp -= 5) and slipped away.\n"
	world := setupTestWorld(t, content)
	ireena, _ := world.FindEntity("Ireena")
	clean := ireena.Descriptions[0].CleanText
	if clean != "She murmured Nobody and slipped away." {
		t.Fatalf("clean text: %q", clean)
	}
	if _, err := world.FindEntity("Nobody"); err == nil {
		t.Fatal("untyped unknown subject should not auto-create an entity")
	}
}

func TestMergeInlineAsideProseOnlyAttachesDescription(t *testing.T) {
	// `(Strahd: hello there)` carries no directive but still attaches the
	// body to Strahd as if `Strahd: hello there` had been written on its
	// own line.
	content := "Strahd (character): Vampire lord.\n" +
		"\n" +
		"Ireena (character): She murmured (Strahd: hello there) and walked on.\n"
	world := setupTestWorld(t, content)
	strahd, _ := world.FindEntity("Strahd")
	ireena, _ := world.FindEntity("Ireena")
	if len(strahd.Descriptions) != 2 {
		t.Fatalf("strahd descs: %+v", strahd.Descriptions)
	}
	if strahd.Descriptions[1].Text != "hello there" {
		t.Fatalf("aside desc text: %q", strahd.Descriptions[1].Text)
	}
	if strahd.Descriptions[1].Line != 3 {
		t.Fatalf("aside desc line: %d, want 3", strahd.Descriptions[1].Line)
	}
	clean := ireena.Descriptions[0].CleanText
	if clean != "She murmured Strahd and walked on." {
		t.Fatalf("ireena clean: %q", clean)
	}
}

func TestMergeInlineAsideNestedParensRoutes(t *testing.T) {
	content := "Strahd (character): hp = 100\n" +
		"Bill (character): NPC.\n" +
		"\n" +
		"Tavern (location): The bard winced (Strahd: attacked Sir Bill (npc); hp -= 5) and ran.\n"
	world := setupTestWorld(t, content)
	strahd, _ := world.FindEntity("Strahd")
	tav, _ := world.FindEntity("Tavern")
	hp := strahd.Fields["hp"]
	if hp.Number != 95 {
		t.Fatalf("strahd hp = %v", hp.Number)
	}
	if len(strahd.Descriptions) != 2 {
		t.Fatalf("strahd descs: %+v", strahd.Descriptions)
	}
	body := strahd.Descriptions[1].Text
	if !strings.Contains(body, "attacked Sir Bill (npc)") {
		t.Fatalf("aside body: %q", body)
	}
	clean := tav.Descriptions[0].CleanText
	if clean != "The bard winced Strahd and ran." {
		t.Fatalf("tavern clean: %q", clean)
	}
}
