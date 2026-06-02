package lsp

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"lore/internal/lore"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

const testContent = `# Session 1

Sildar Hallwinter (character) | Sildar: Fighter. Member of the
  Lords Alliance. Rescued from Cragmaw Hideout.

Cragmaw Hideout (location): North of Triboar Trail. Sildar was
  captured here. Infested with goblins.

We followed the goblin trail and found Sildar inside.
`

// soleWorld returns the world of a server's single project. Fails the test
// if the server has zero or many projects — call sites assume single-project
// shape, which the production graphWorld also relies on.
func soleWorld(t *testing.T, s *Server) *lore.World {
	t.Helper()
	if len(s.projects) != 1 {
		t.Fatalf("soleWorld: expected 1 project, got %d", len(s.projects))
	}
	for _, ps := range s.projects {
		return ps.world()
	}
	return nil
}

func setupTestServer(t *testing.T, content string) *Server {
	t.Helper()

	fsys := make(fstest.MapFS)
	fsys["lore.toml"] = &fstest.MapFile{Data: []byte(`files = ["**/*.md"]`)}
	fsys["test.md"] = &fstest.MapFile{Data: []byte(content)}

	cfg := lore.Config{Files: []string{"**/*.md"}}
	matcher := lore.Matcher{Patterns: cfg.Files}
	paths, err := matcher.Find(fsys)
	if err != nil {
		t.Fatal(err)
	}
	project := &lore.Project{FS: fsys, Config: cfg, Matcher: matcher, FilePaths: paths}

	s := NewServer()
	s.root = "/test"
	ps := &projectState{root: "/test", project: project, index: NewIndex()}
	if err := ps.index.LoadProject(project); err != nil {
		t.Fatal(err)
	}
	// Editor buffer test URIs use /test as the root; mirror the content there.
	ps.index.SetFile("test.md", content)
	s.projects = map[string]*projectState{"/test": ps}
	return s
}

func TestHoverOnEntityName(t *testing.T) {
	s := setupTestServer(t, testContent)

	// "Sildar Hallwinter" starts at column 0 on line 2 (0-based).
	result, err := s.hover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
			Position:     protocol.Position{Line: 2, Character: 5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected hover result, got nil")
	}
	mc, ok := result.Contents.(protocol.MarkupContent)
	if !ok {
		t.Fatalf("expected MarkupContent, got %T", result.Contents)
	}
	if mc.Kind != protocol.MarkupKindMarkdown {
		t.Fatalf("expected markdown, got %q", mc.Kind)
	}
	if !lore.ContainsIgnoreCase(mc.Value, "Sildar Hallwinter") {
		t.Fatalf("hover should contain entity name, got %q", mc.Value)
	}
	if !lore.ContainsIgnoreCase(mc.Value, "character") {
		t.Fatalf("hover should contain type, got %q", mc.Value)
	}
}

func TestHoverMergesMultipleDescriptions(t *testing.T) {
	content := `Strahd (character): Vampire lord of Barovia.

Strahd: Wields the Sunsword's nemesis.

Strahd: Trapped in his castle by the mists.
`
	s := setupTestServer(t, content)

	result, err := s.hover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
			Position:     protocol.Position{Line: 0, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected hover result, got nil")
	}
	mc := result.Contents.(protocol.MarkupContent)
	for _, want := range []string{"Vampire lord", "Sunsword", "Trapped"} {
		if !lore.ContainsIgnoreCase(mc.Value, want) {
			t.Errorf("hover should contain %q, got %q", want, mc.Value)
		}
	}
}

func TestHoverOnFreeText(t *testing.T) {
	s := setupTestServer(t, testContent)

	// Line 0 is "# Session 1" — no entity.
	result, err := s.hover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
			Position:     protocol.Position{Line: 0, Character: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatalf("expected nil hover on free text, got %+v", result)
	}
}

func TestDefinitionJumpsToFirstDescription(t *testing.T) {
	s := setupTestServer(t, testContent)

	// Hover over "Sildar" in the free text on line 8 (0-based).
	result, err := s.definition(nil, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
			Position:     protocol.Position{Line: 8, Character: 40},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected definition result, got nil")
	}
	loc, ok := result.(protocol.Location)
	if !ok {
		t.Fatalf("expected Location, got %T", result)
	}
	// First definition of Sildar Hallwinter is on line 3 (1-based) = line 2 (0-based).
	if loc.Range.Start.Line != 2 {
		t.Fatalf("expected line 2 (0-based), got %d", loc.Range.Start.Line)
	}
}

func TestDefinitionNilForNonEntity(t *testing.T) {
	s := setupTestServer(t, testContent)

	result, err := s.definition(nil, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
			Position:     protocol.Position{Line: 0, Character: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatal("expected nil for non-entity text")
	}
}

func TestReferencesIncludesCrossRefs(t *testing.T) {
	s := setupTestServer(t, testContent)

	// Cursor on "Sildar Hallwinter" at line 2.
	results, err := s.references(nil, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
			Position:     protocol.Position{Line: 2, Character: 5},
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one reference")
	}
	// Should include the reference from Cragmaw Hideout's description and the free text.
}

func TestReferencesIncludesDeclaration(t *testing.T) {
	s := setupTestServer(t, testContent)

	results, err := s.references(nil, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
			Position:     protocol.Position{Line: 2, Character: 5},
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	resultsWithout, _ := s.references(nil, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
			Position:     protocol.Position{Line: 2, Character: 5},
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: false},
	})

	if len(results) <= len(resultsWithout) {
		t.Fatalf("IncludeDeclaration=true should return more results: got %d vs %d",
			len(results), len(resultsWithout))
	}
}

func TestWorkspaceSymbolReturnsAllEntities(t *testing.T) {
	s := setupTestServer(t, testContent)

	results, err := s.workspaceSymbol(nil, &protocol.WorkspaceSymbolParams{Query: ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		names := make([]string, len(results))
		for i, r := range results {
			names[i] = r.Name
		}
		t.Fatalf("expected 2 symbols, got %d: %v", len(results), names)
	}

	byName := make(map[string]protocol.SymbolInformation, len(results))
	for _, r := range results {
		byName[r.Name] = r
	}
	sildar, ok := byName["Sildar Hallwinter"]
	if !ok {
		t.Fatalf("missing Sildar Hallwinter symbol")
	}
	if sildar.Kind != protocol.SymbolKindObject {
		t.Errorf("expected SymbolKindObject for Sildar, got %d", sildar.Kind)
	}
	if sildar.ContainerName == nil || *sildar.ContainerName != "character" {
		got := "<nil>"
		if sildar.ContainerName != nil {
			got = *sildar.ContainerName
		}
		t.Errorf("expected container 'character', got %q", got)
	}
	if !strings.HasSuffix(sildar.Location.URI, "/test/test.md") {
		t.Errorf("expected location in test.md, got %q", sildar.Location.URI)
	}
	// First definition is line 3 (1-based) = line 2 (0-based).
	if sildar.Location.Range.Start.Line != 2 {
		t.Errorf("expected start line 2, got %d", sildar.Location.Range.Start.Line)
	}
}

func TestWorkspaceSymbolFiltersByQuery(t *testing.T) {
	s := setupTestServer(t, testContent)

	results, err := s.workspaceSymbol(nil, &protocol.WorkspaceSymbolParams{Query: "crag"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'crag', got %d", len(results))
	}
	if results[0].Name != "Cragmaw Hideout" {
		t.Errorf("expected Cragmaw Hideout, got %q", results[0].Name)
	}
}

func TestWorkspaceSymbolMatchesAlias(t *testing.T) {
	s := setupTestServer(t, testContent)

	// "Sildar" is an alias for "Sildar Hallwinter".
	results, err := s.workspaceSymbol(nil, &protocol.WorkspaceSymbolParams{Query: "sildar"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "Sildar Hallwinter" {
		t.Fatalf("expected alias match to find Sildar Hallwinter, got %+v", results)
	}
}

func TestCompletionReturnsAllEntitiesAndAliases(t *testing.T) {
	s := setupTestServer(t, testContent)

	result, err := s.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	list, ok := result.(*protocol.CompletionList)
	if !ok {
		t.Fatalf("expected *CompletionList, got %T", result)
	}
	// 2 entities: Sildar Hallwinter (+ alias "Sildar"), Cragmaw Hideout = 3 items.
	if len(list.Items) != 3 {
		labels := make([]string, len(list.Items))
		for i, item := range list.Items {
			labels[i] = item.Label
		}
		t.Fatalf("expected 3 completion items, got %d: %v", len(list.Items), labels)
	}
	// All should be text kind.
	for _, item := range list.Items {
		if item.Kind == nil || *item.Kind != protocol.CompletionItemKindText {
			t.Fatalf("expected CompletionItemKindText for %q", item.Label)
		}
	}
}

func TestCompletionQualifiesAmbiguousLabels(t *testing.T) {
	content := `# Session 2

Barovia (town) | The Village: Misty, walled, gothic.

Barovia (region): The whole dreary country.

Strahd (character) | Barovia: Vampire lord, rules the country.
`
	s := setupTestServer(t, content)

	result, err := s.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	list := result.(*protocol.CompletionList)

	got := make(map[string]bool, len(list.Items))
	for _, item := range list.Items {
		if got[item.Label] {
			t.Fatalf("duplicate completion label %q", item.Label)
		}
		got[item.Label] = true
	}

	// "Barovia" is shared by the town, the region, and Strahd's alias, so
	// every occurrence must be qualified. "The Village" and "Strahd" appear
	// only once, so they stay bare.
	want := []string{
		"Barovia (town)",
		"The Village",
		"Barovia (region)",
		"Strahd",
		"Barovia (character)",
	}
	for _, label := range want {
		if !got[label] {
			t.Errorf("missing completion label %q; got %v", label, keys(got))
		}
	}
	// The bare "Barovia" must never be offered — it's ambiguous.
	if got["Barovia"] {
		t.Errorf("bare ambiguous label %q should not be suggested", "Barovia")
	}
	if len(list.Items) != len(want) {
		t.Errorf("expected %d items, got %d: %v", len(want), len(list.Items), keys(got))
	}
}

func TestFormatEntityHoverIncludesState(t *testing.T) {
	ent := &lore.Entity{
		Name: "Sildar",
		Type: "character",
		Descriptions: []lore.Description{
			{Text: "Fighter.", File: "t.md", Line: 1},
		},
		Tags: map[string]bool{"injured": true},
		Fields: map[string]lore.FieldValue{
			"hp": {Kind: lore.FieldNumeric, Number: 3},
		},
	}
	out := formatEntityHover(nil, ent, "", 0, HoverStateModeLatest, true, nil)
	if !strings.Contains(out, "+injured") {
		t.Fatalf("hover missing tag: %q", out)
	}
	if !strings.Contains(out, "hp: 3") {
		t.Fatalf("hover missing field: %q", out)
	}
}

func TestFormatEntityHoverBothModeNoAnnotationWhenEqual(t *testing.T) {
	ent := &lore.Entity{
		Name: "Sildar",
		Type: "character",
		Tags: map[string]bool{"injured": true},
		StateHistory: []lore.StateEvent{
			{Op: lore.StateOpAdd, Target: "injured", Span: lore.StateSpan{File: "t.md", Line: 5}},
		},
	}
	out := formatEntityHover(nil, ent, "t.md", 99, HoverStateModeBoth, true, nil)
	if strings.Contains(out, "(latest") {
		t.Fatalf("expected no latest annotation when equal; got %q", out)
	}
	if !strings.Contains(out, "+injured") {
		t.Fatalf("hover missing tag: %q", out)
	}
}

func TestFormatEntityHoverBothModeAnnotatesDivergentTags(t *testing.T) {
	ent := &lore.Entity{
		Name: "Sildar",
		Type: "character",
		Tags: map[string]bool{"injured": true, "cursed": true},
		StateHistory: []lore.StateEvent{
			{Op: lore.StateOpAdd, Target: "injured", Span: lore.StateSpan{File: "t.md", Line: 5}},
			{Op: lore.StateOpAdd, Target: "cursed", Span: lore.StateSpan{File: "t.md", Line: 20}},
		},
	}
	out := formatEntityHover(nil, ent, "t.md", 10, HoverStateModeBoth, true, nil)
	if !strings.Contains(out, "+injured  (latest: +cursed +injured)") {
		t.Fatalf("expected inline tag latest annotation; got %q", out)
	}
}

func TestFormatEntityHoverBothModeAnnotatesDivergentFields(t *testing.T) {
	ent := &lore.Entity{
		Name: "Barovia",
		Type: "town",
		Fields: map[string]lore.FieldValue{
			"population": {Kind: lore.FieldNumeric, Number: 100},
			"shops":      {Kind: lore.FieldText, Text: []string{"toy store", "general store"}},
		},
		StateHistory: []lore.StateEvent{
			{Op: lore.StateOpSet, Target: "population", Value: &lore.FieldValue{Kind: lore.FieldNumeric, Number: 100}, Span: lore.StateSpan{File: "t.md", Line: 1}},
			{Op: lore.StateOpSet, Target: "shops", Value: &lore.FieldValue{Kind: lore.FieldText, Text: []string{"toy store", "general store", "coffin-maker"}}, Span: lore.StateSpan{File: "t.md", Line: 2}},
			{Op: lore.StateOpRemove, Target: "shops", Value: &lore.FieldValue{Kind: lore.FieldText, Text: []string{"coffin-maker"}}, Span: lore.StateSpan{File: "t.md", Line: 20}},
		},
	}
	out := formatEntityHover(nil, ent, "t.md", 10, HoverStateModeBoth, true, nil)
	// population equal at cursor and latest → no annotation.
	if !strings.Contains(out, "population: 100") || strings.Contains(out, "population: 100 (latest") {
		t.Fatalf("population should be shown without annotation; got %q", out)
	}
	// shops at cursor still has coffin-maker; latest dropped it.
	if !strings.Contains(out, "shops: coffin-maker, general store, toy store (latest: general store, toy store)") {
		t.Fatalf("shops line missing latest annotation; got %q", out)
	}
}

func TestFormatEntityHoverAtCursorModeOnly(t *testing.T) {
	ent := &lore.Entity{
		Name: "Sildar",
		Tags: map[string]bool{"injured": true, "cursed": true},
		StateHistory: []lore.StateEvent{
			{Op: lore.StateOpAdd, Target: "injured", Span: lore.StateSpan{File: "t.md", Line: 5}},
			{Op: lore.StateOpAdd, Target: "cursed", Span: lore.StateSpan{File: "t.md", Line: 20}},
		},
	}
	out := formatEntityHover(nil, ent, "t.md", 10, HoverStateModeAtCursor, true, nil)
	if strings.Contains(out, "(latest") {
		t.Fatalf("atCursor mode should not annotate; got %q", out)
	}
	if !strings.Contains(out, "+injured") || strings.Contains(out, "+cursed") {
		t.Fatalf("expected only at-cursor tag; got %q", out)
	}
}

func TestFormatEntityHoverLatestModeIgnoresCursor(t *testing.T) {
	ent := &lore.Entity{
		Name: "Sildar",
		Tags: map[string]bool{"injured": true, "cursed": true},
		StateHistory: []lore.StateEvent{
			{Op: lore.StateOpAdd, Target: "injured", Span: lore.StateSpan{File: "t.md", Line: 5}},
			{Op: lore.StateOpAdd, Target: "cursed", Span: lore.StateSpan{File: "t.md", Line: 20}},
		},
	}
	out := formatEntityHover(nil, ent, "t.md", 10, HoverStateModeLatest, true, nil)
	if !strings.Contains(out, "+injured") || !strings.Contains(out, "+cursed") {
		t.Fatalf("latest mode should show both tags; got %q", out)
	}
	if strings.Contains(out, "(latest") {
		t.Fatalf("latest mode should not annotate; got %q", out)
	}
}

func TestFormatEntityHoverBothModeNoneAtCursor(t *testing.T) {
	ent := &lore.Entity{
		Name: "Sildar",
		Tags: map[string]bool{"injured": true},
		StateHistory: []lore.StateEvent{
			{Op: lore.StateOpAdd, Target: "injured", Span: lore.StateSpan{File: "t.md", Line: 20}},
		},
	}
	out := formatEntityHover(nil, ent, "t.md", 5, HoverStateModeBoth, true, nil)
	if !strings.Contains(out, "(none)  (latest: +injured)") {
		t.Fatalf("expected tag line with (none)  (latest: +injured); got %q", out)
	}
}

// hoverWorld builds a single-file world with the builtin relation vocab, for
// relation-hover tests that need real edge resolution.
func hoverWorld(t *testing.T, src string) (*lore.World, *lore.Entity, func(name string) *lore.Entity) {
	t.Helper()
	world := lore.Merge([]*lore.FileParse{lore.ParseFile("test.md", src)})
	world.Vocab = lore.NewRelationVocab(lore.BuiltinRelations())
	find := func(name string) *lore.Entity {
		for i := range world.Entities {
			if world.Entities[i].Name == name {
				return &world.Entities[i]
			}
		}
		t.Fatalf("entity %q not found", name)
		return nil
	}
	return world, nil, find
}

// A reverse relation declared after the hovered entity must still appear: in
// latest mode it shows plainly; in both mode it carries a "(latest: …)"
// annotation because at the cursor the forward edge wasn't declared yet.
func TestHoverReverseRelationDeclaredLater(t *testing.T) {
	// Doug at line 1; Sarah declares father -> Doug at line 3.
	world, _, find := hoverWorld(t, "Doug (person): A dwarf.\n\nSarah (person): father -> Doug\n")
	doug := find("Doug")

	latest := formatEntityHover(world, doug, "test.md", 1, HoverStateModeLatest, true, nil)
	if !strings.Contains(latest, "child → Sarah") {
		t.Fatalf("latest mode should show reverse child → Sarah; got %q", latest)
	}

	both := formatEntityHover(world, doug, "test.md", 1, HoverStateModeBoth, true, nil)
	if !strings.Contains(both, "child → (none)  (latest: Sarah)") {
		t.Fatalf("both mode should mark the later-declared reverse edge as latest-only; got %q", both)
	}

	at := formatEntityHover(world, doug, "test.md", 1, HoverStateModeAtCursor, true, nil)
	if strings.Contains(at, "Sarah") {
		t.Fatalf("atCursor at line 1 should not yet see Sarah's edge; got %q", at)
	}
}

// A relation present at the cursor but removed by latest shows the cursor value
// with a "(latest: …)" annotation, like a diverging field.
func TestHoverRelationRemovedByLatest(t *testing.T) {
	world, _, find := hoverWorld(t, "Sarah (person): friend -> Mary\n\nMary (person): ok\n\nSarah (person): friend -/> Mary\n")
	sarah := find("Sarah")

	both := formatEntityHover(world, sarah, "test.md", 1, HoverStateModeBoth, true, nil)
	if !strings.Contains(both, "friend → Mary  (latest: (none))") {
		t.Fatalf("both mode should show cursor friendship with latest (none); got %q", both)
	}
}

func TestParseHoverStateMode(t *testing.T) {
	cases := map[string]HoverStateMode{
		"":         HoverStateModeBoth,
		"both":     HoverStateModeBoth,
		"atCursor": HoverStateModeAtCursor,
		"latest":   HoverStateModeLatest,
		"garbage":  HoverStateModeBoth,
	}
	for raw, want := range cases {
		if got := parseHoverStateMode(raw); got != want {
			t.Errorf("parseHoverStateMode(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestHoverStateModeFromOptions(t *testing.T) {
	if got := hoverStateModeFromOptions(nil); got != HoverStateModeBoth {
		t.Errorf("nil: %q", got)
	}
	if got := hoverStateModeFromOptions(map[string]any{"hoverStateMode": "latest"}); got != HoverStateModeLatest {
		t.Errorf("flat: %q", got)
	}
	nested := map[string]any{"hover": map[string]any{"stateMode": "atCursor"}}
	if got := hoverStateModeFromOptions(nested); got != HoverStateModeAtCursor {
		t.Errorf("nested: %q", got)
	}
}

func TestFormatEntityHoverStripsDirectivesWhenDisabled(t *testing.T) {
	ent := &lore.Entity{
		Name: "Sildar",
		Type: "character",
		Descriptions: []lore.Description{
			{
				Text:      "Took an arrow to the knee. +injured",
				CleanText: "Took an arrow to the knee.",
				File:      "t.md", Line: 1,
			},
			{
				Text:      "+healed",
				CleanText: "",
				File:      "t.md", Line: 3,
			},
		},
	}
	with := formatEntityHover(nil, ent, "", 0, HoverStateModeLatest, true, nil)
	if !strings.Contains(with, "+injured") {
		t.Fatalf("showStateDirectives=true should include raw directives; got %q", with)
	}
	without := formatEntityHover(nil, ent, "", 0, HoverStateModeLatest, false, nil)
	if strings.Contains(without, "+injured") || strings.Contains(without, "+healed") {
		t.Fatalf("showStateDirectives=false should strip directives; got %q", without)
	}
	if !strings.Contains(without, "Took an arrow to the knee.") {
		t.Fatalf("cleaned prose should still appear; got %q", without)
	}
}

func TestHoverShowStateDirectivesFromOptions(t *testing.T) {
	if got := hoverShowStateDirectivesFromOptions(nil); got != false {
		t.Errorf("nil default: %v", got)
	}
	if got := hoverShowStateDirectivesFromOptions(map[string]any{"hoverShowStateDirectives": true}); got != true {
		t.Errorf("flat: %v", got)
	}
	nested := map[string]any{"hover": map[string]any{"showStateDirectives": true}}
	if got := hoverShowStateDirectivesFromOptions(nested); got != true {
		t.Errorf("nested: %v", got)
	}
}

func TestFormatEntityHoverColourisesNamesAndDescriptions(t *testing.T) {
	content := "Strahd (character): Vampire lord.\n\nTatyana (npc): Strahd's lost love.\n"
	s := setupTestServer(t, content)
	s.palette = []string{
		"#000001", "#000002", "#000003", "#000004", "#000005",
		"#000006", "#000007", "#000008", "#000009", "#00000A",
		"#00000B", "#00000C", "#00000D", "#00000E", "#00000F",
		"#000010", "#000011", "#000012", "#000013", "#000014",
		"#000015", "#000016", "#000017", "#000018", "#000019",
		"#00001A",
	}
	col := &colouriser{world: soleWorld(t, s), palette: s.palette}

	world := soleWorld(t, s)
	var tatyana *lore.Entity
	for i := range world.Entities {
		if world.Entities[i].Name == "Tatyana" {
			tatyana = &world.Entities[i]
		}
	}
	if tatyana == nil {
		t.Fatal("Tatyana not found")
	}

	out := formatEntityHover(world, tatyana, "", 0, HoverStateModeLatest, true, col)

	tatyanaHex := s.palette[entityColourIndex(tatyana)]
	if !strings.Contains(out, `<span style="color:`+tatyanaHex+`;">Tatyana</span>`) {
		t.Fatalf("hover did not wrap Tatyana name: %q", out)
	}

	// Description prose mentions Strahd — should be wrapped in Strahd's hex.
	var strahd *lore.Entity
	for i := range world.Entities {
		if world.Entities[i].Name == "Strahd" {
			strahd = &world.Entities[i]
		}
	}
	if strahd == nil {
		t.Fatal("Strahd not found")
	}
	strahdHex := s.palette[entityColourIndex(strahd)]
	if !strings.Contains(out, `<span style="color:`+strahdHex+`;">Strahd</span>`) {
		t.Fatalf("hover did not wrap Strahd reference in description: %q", out)
	}
}


func TestFileToURIEncodesSpecialChars(t *testing.T) {
	ps := &projectState{root: "/workspace/notes"}
	cases := []struct {
		rel  string
		want string
	}{
		{"plain.md", "file:///workspace/notes/plain.md"},
		{"what?.md", "file:///workspace/notes/what%3F.md"},
		{"with space.md", "file:///workspace/notes/with%20space.md"},
		{"hash#tag.md", "file:///workspace/notes/hash%23tag.md"},
		{"sub/dir/file?.md", "file:///workspace/notes/sub/dir/file%3F.md"},
	}
	for _, tc := range cases {
		got := ps.fileToURI(tc.rel)
		if got != tc.want {
			t.Errorf("fileToURI(%q) = %q, want %q", tc.rel, got, tc.want)
		}
		// Round-trip: VSCode-side parsing should recover the path.
		back := uriToPath(&got)
		wantPath := filepath.Join(ps.root, filepath.FromSlash(tc.rel))
		if back != wantPath {
			t.Errorf("uriToPath(%q) = %q, want %q", got, back, wantPath)
		}
	}
}

func TestColouriseEscapesHTMLActiveCharsInProse(t *testing.T) {
	// User prose passes through Wrap with `<` and `&` HTML-escaped, so
	// dangerous tags from a user's notes never enter VSCode's markdown/
	// HTML pipeline — we don't depend on its sanitiser being correct.
	// `>` is left alone so markdown blockquotes survive.
	world := lore.NewWorld()
	world.Match = nil
	col := &colouriser{world: world, palette: []string{"#FF0000"}}
	got := col.Wrap("a < b & c > d")
	want := "a &lt; b &amp; c > d"
	if got != want {
		t.Fatalf("escape mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestEscapeForHover(t *testing.T) {
	// `<` and `&` get HTML-escaped; `>` passes through. Applied to both
	// user prose and entity-name span content in hover, so user `<` can
	// never start an HTML tag and user `&` can never start an entity.
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"Tom & Jerry", "Tom &amp; Jerry"},
		{"a < b", "a &lt; b"},
		{"a > b", "a > b"},
		{"X<Y&Z", "X&lt;Y&amp;Z"},
		{"<script>alert(1)</script>", "&lt;script>alert(1)&lt;/script>"},
	}
	for _, c := range cases {
		if got := escapeForHover(c.in); got != c.want {
			t.Errorf("escapeForHover(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestColouriseEscapesDangerousTagsInProse(t *testing.T) {
	// A `<script>` smuggled into user prose can't reach VSCode's HTML
	// renderer — the leading `<` gets HTML-escaped at the server.
	world := lore.NewWorld()
	world.Match = nil
	col := &colouriser{world: world, palette: []string{"#FF0000"}}
	got := col.Wrap("before <script>alert(1)</script> after")
	if strings.Contains(got, "<script") {
		t.Fatalf("dangerous tag leaked unescaped: %q", got)
	}
}

func TestColourisePreservesBlockquoteMarker(t *testing.T) {
	// Markdown blockquotes — `>` at line start — must survive Wrap so
	// VSCode's markdown renderer can format them as <blockquote>.
	world := lore.NewWorld()
	world.Match = nil
	col := &colouriser{world: world, palette: []string{"#FF0000"}}
	got := col.Wrap("> Lots of prose.\n>\n> Spread over many lines.")
	want := "> Lots of prose.\n>\n> Spread over many lines."
	if got != want {
		t.Fatalf("blockquote mangled:\n got: %q\nwant: %q", got, want)
	}
}

func TestColourisePreservesHorizontalRule(t *testing.T) {
	world := lore.NewWorld()
	world.Match = nil
	col := &colouriser{world: world, palette: []string{"#FF0000"}}
	got := col.Wrap("before\n\n---\n\nafter")
	want := "before\n\n---\n\nafter"
	if got != want {
		t.Fatalf("horizontal rule mangled:\n got: %q\nwant: %q", got, want)
	}
}

func TestFormatEntityHoverPreservesBlockquoteAndRule(t *testing.T) {
	// End-to-end: a block-form aside body containing blockquote and HR
	// markdown reaches the hover output without `>` being entity-escaped,
	// so VSCode's markdown renderer can format both.
	ent := &lore.Entity{
		Name: "Rictavio's Journal",
		Type: "item",
		Descriptions: []lore.Description{
			{
				Text: "> Lots of prose.\n>\n> Spread over many lines.\n\n---\n\nFound in the attic.",
				File: "t.md", Line: 1,
			},
		},
	}
	out := formatEntityHover(nil, ent, "", 0, HoverStateModeLatest, true, nil)
	if strings.Contains(out, "&gt;") {
		t.Fatalf("blockquote marker entity-escaped in hover: %q", out)
	}
	if !strings.Contains(out, "> Lots of prose.") {
		t.Fatalf("blockquote missing from hover: %q", out)
	}
	if !strings.Contains(out, "\n---\n") {
		t.Fatalf("horizontal rule missing from hover: %q", out)
	}
}

// linkColouriser builds a colouriser whose world has a single entity
// "Link", so any URL containing the word "link" would naively get a
// colour span injected mid-URL. Used to verify URL constructs survive
// Wrap untouched.
func linkColouriser() *colouriser {
	world := lore.NewWorld()
	world.Entities = []lore.Entity{{Name: "Link", Type: "character"}}
	world.Match = lore.BuildMatchIndex(world)
	return &colouriser{world: world, palette: []string{"#FF0000"}}
}

func TestColouriseProtectsAutolinks(t *testing.T) {
	col := linkColouriser()
	got := col.Wrap("see <www.link.com> for more")
	want := "see <www.link.com> for more"
	if got != want {
		t.Fatalf("autolink mangled:\n got: %q\nwant: %q", got, want)
	}
}

func TestColouriseProtectsMarkdownLinkURL(t *testing.T) {
	col := linkColouriser()
	got := col.Wrap("see [Link](www.link.com) for more")
	// Label "Link" inside `[...]` may still be coloured (markdown-it
	// allows HTML inside link labels); the URL inside `(...)` must be
	// untouched even though it contains "link" — case-sensitive matching
	// already filters that out, and URL protection covers the case where
	// the URL happens to spell the entity's exact name.
	want := `see [<span style="color:#FF0000;">Link</span>](www.link.com) for more`
	if got != want {
		t.Fatalf("markdown link URL mangled:\n got: %q\nwant: %q", got, want)
	}
}

func TestColouriseProtectsBareURL(t *testing.T) {
	col := linkColouriser()
	got := col.Wrap("visit https://www.link.com/path now")
	want := "visit https://www.link.com/path now"
	if got != want {
		t.Fatalf("bare URL mangled:\n got: %q\nwant: %q", got, want)
	}
}

func TestColouriseStillWrapsOutsideURLs(t *testing.T) {
	col := linkColouriser()
	got := col.Wrap("Link is here, also www.link.com tail")
	want := `<span style="color:#FF0000;">Link</span> is here, also www.link.com tail`
	if got != want {
		t.Fatalf("non-URL link not wrapped:\n got: %q\nwant: %q", got, want)
	}
}

func TestFormatEntityHoverOmitsEmptyState(t *testing.T) {
	ent := &lore.Entity{
		Name: "Sildar",
		Type: "character",
		Descriptions: []lore.Description{
			{Text: "Fighter.", File: "t.md", Line: 1},
		},
	}
	out := formatEntityHover(nil, ent, "", 0, HoverStateModeLatest, true, nil)
	// When there's no state, no empty code block should appear.
	if strings.Contains(out, "```\n\n```") || strings.Contains(out, "```\n```") {
		t.Fatalf("hover has empty state block: %q", out)
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestSemanticTokensStableColour(t *testing.T) {
	s := setupTestServer(t, testContent)

	result1, err := s.semanticTokensFull(nil, &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result1.Data) == 0 {
		t.Fatal("expected semantic tokens")
	}

	// Request again — colours should be identical (stable hash).
	result2, err := s.semanticTokensFull(nil, &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result1.Data) != len(result2.Data) {
		t.Fatalf("token count changed across re-parse: %d vs %d", len(result1.Data), len(result2.Data))
	}
	for i := range result1.Data {
		if result1.Data[i] != result2.Data[i] {
			t.Fatalf("token data differs at index %d: %d vs %d", i, result1.Data[i], result2.Data[i])
		}
	}
}

func TestSemanticTokensHasCorrectPositions(t *testing.T) {
	content := "Strahd (character): Vampire lord.\n\nWe saw Strahd at the castle.\n"
	s := setupTestServer(t, content)

	result, err := s.semanticTokensFull(nil, &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// "Strahd" appears on line 0 col 0 and line 2 col 7.
	// Encoded as delta: [0, 0, 6, 0, mod], [2, 7, 6, 0, mod]
	if len(result.Data) < 10 {
		t.Fatalf("expected at least 2 tokens (10 values), got %d values", len(result.Data))
	}
	// First token: line 0, col 0, length 6.
	if result.Data[0] != 0 || result.Data[1] != 0 || result.Data[2] != 6 {
		t.Fatalf("first token wrong: deltaLine=%d deltaChar=%d len=%d",
			result.Data[0], result.Data[1], result.Data[2])
	}
	// Second token: deltaLine=2, col 7, length 6.
	if result.Data[5] != 2 || result.Data[6] != 7 || result.Data[7] != 6 {
		t.Fatalf("second token wrong: deltaLine=%d deltaChar=%d len=%d",
			result.Data[5], result.Data[6], result.Data[7])
	}
}

func TestSemanticTokensConvertsByteOffsetsToUTF16(t *testing.T) {
	// `’` is U+2019 (RIGHT SINGLE QUOTATION MARK) — 3 bytes UTF-8, 1 UTF-16
	// code unit. A naïve byte-offset emit would place "Strahd" at column 13
	// instead of the correct UTF-16 column 11.
	content := "Strahd (character): Vampire.\n\nShe’s with Strahd.\n"
	s := setupTestServer(t, content)

	result, err := s.semanticTokensFull(nil, &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Data) < 10 {
		t.Fatalf("expected at least 2 tokens, got %d values", len(result.Data))
	}
	// First token: header definition, line 0, col 0, length 6.
	if result.Data[0] != 0 || result.Data[1] != 0 || result.Data[2] != 6 {
		t.Fatalf("first token wrong: deltaLine=%d deltaChar=%d len=%d",
			result.Data[0], result.Data[1], result.Data[2])
	}
	// Second token: line 2, UTF-16 col 11, length 6 (still ASCII chars).
	if result.Data[5] != 2 || result.Data[6] != 11 || result.Data[7] != 6 {
		t.Fatalf("second token wrong: deltaLine=%d deltaChar=%d len=%d",
			result.Data[5], result.Data[6], result.Data[7])
	}
}

func TestSemanticTokensDisambiguatedNameSingleEmission(t *testing.T) {
	// Two entities share the name "Barovia". A mention in free text that
	// carries a `(town)` suffix must emit exactly one token, with the town
	// entity's colour modifiers — not the country's, and not both overlapping.
	content := "Barovia (town): Gloomy.\n\nBarovia (country): Cloudy.\n\nWe entered Barovia (town) from the west.\n"
	s := setupTestServer(t, content)

	result, err := s.semanticTokensFull(nil, &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Data)%5 != 0 {
		t.Fatalf("malformed semantic token stream: %d values", len(result.Data))
	}

	world := soleWorld(t, s)
	var townMods, countryMods uint32
	for i := range world.Entities {
		ent := &world.Entities[i]
		if ent.Type == "town" {
			townMods = uint32(1) << entityColourIndex(ent)
		}
		if ent.Type == "country" {
			countryMods = uint32(1) << entityColourIndex(ent)
		}
	}
	if townMods == 0 || countryMods == 0 {
		t.Fatalf("expected both entities in world; got town=%d country=%d", townMods, countryMods)
	}
	if townMods == countryMods {
		t.Fatal("test requires town and country to hash to distinct colours")
	}

	// Walk the delta-encoded stream and resolve each token's absolute (line, col).
	var prevLine, prevChar uint32
	type tok struct{ line, col, mods uint32 }
	var toks []tok
	for i := 0; i < len(result.Data); i += 5 {
		deltaLine := result.Data[i]
		deltaChar := result.Data[i+1]
		mods := result.Data[i+4]
		line := prevLine + deltaLine
		col := deltaChar
		if deltaLine == 0 {
			col = prevChar + deltaChar
		}
		toks = append(toks, tok{line, col, mods})
		prevLine = line
		prevChar = col
	}

	// Free-text mention is on line 4 ("We entered Barovia (town) from the west.")
	// at column 11. Collect tokens covering that position.
	var hits []tok
	for _, tk := range toks {
		if tk.line == 4 && tk.col == 11 {
			hits = append(hits, tk)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("expected exactly 1 token for disambiguated free-text mention, got %d: %+v", len(hits), hits)
	}
	if hits[0].mods != townMods {
		t.Fatalf("expected town colour %d at free-text mention, got %d", townMods, hits[0].mods)
	}
}

func TestEntityListReturnsTagsAndLocations(t *testing.T) {
	content := `# Session 3

Strahd (character): Lord of Barovia. +vampire +undead

Castle Ravenloft (location): Strahd's seat.
`
	s := setupTestServer(t, content)

	result, err := s.entityList(&EntityListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entities) != 2 {
		names := make([]string, len(result.Entities))
		for i, e := range result.Entities {
			names[i] = e.Name
		}
		t.Fatalf("expected 2 entities, got %d: %v", len(result.Entities), names)
	}

	byName := make(map[string]EntityListItem, len(result.Entities))
	for _, e := range result.Entities {
		byName[e.Name] = e
	}

	strahd, ok := byName["Strahd"]
	if !ok {
		t.Fatal("missing Strahd entity")
	}
	if strahd.Type != "character" {
		t.Errorf("expected type 'character', got %q", strahd.Type)
	}
	wantTags := map[string]bool{"vampire": true, "undead": true}
	if len(strahd.Tags) != len(wantTags) {
		t.Fatalf("expected tags %v, got %v", wantTags, strahd.Tags)
	}
	for _, tag := range strahd.Tags {
		if !wantTags[tag] {
			t.Errorf("unexpected tag %q", tag)
		}
	}
	if !strings.HasSuffix(strahd.Location.URI, "/test/test.md") {
		t.Errorf("expected location in test.md, got %q", strahd.Location.URI)
	}

	castle, ok := byName["Castle Ravenloft"]
	if !ok {
		t.Fatal("missing Castle Ravenloft entity")
	}
	if len(castle.Tags) != 0 {
		t.Errorf("expected no tags on Castle Ravenloft, got %v", castle.Tags)
	}
}

func TestSemanticTokensLongestMatchWinsOverlap(t *testing.T) {
	// Two entities whose names overlap: "Vallaki" is a prefix of "Vallaki
	// Cathedral". A free-text mention of the longer name must emit exactly
	// one token covering the full span, not two overlapping tokens that
	// VSCode would resolve by keeping only the shorter prefix.
	content := "Vallaki (town): A walled town.\n\nVallaki Cathedral (location): The cathedral in Vallaki.\n\nWe arrived at Vallaki Cathedral after dark.\n"
	s := setupTestServer(t, content)

	result, err := s.semanticTokensFull(nil, &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Data)%5 != 0 {
		t.Fatalf("malformed semantic token stream: %d values", len(result.Data))
	}

	world := soleWorld(t, s)
	var cathedralMods uint32
	for i := range world.Entities {
		ent := &world.Entities[i]
		if ent.Name == "Vallaki Cathedral" {
			cathedralMods = uint32(1) << entityColourIndex(ent)
		}
	}
	if cathedralMods == 0 {
		t.Fatal("expected Vallaki Cathedral entity in world")
	}

	// Walk the delta-encoded stream into absolute coordinates.
	var prevLine, prevChar uint32
	type tok struct{ line, col, length, mods uint32 }
	var toks []tok
	for i := 0; i < len(result.Data); i += 5 {
		deltaLine := result.Data[i]
		deltaChar := result.Data[i+1]
		length := result.Data[i+2]
		mods := result.Data[i+4]
		line := prevLine + deltaLine
		col := deltaChar
		if deltaLine == 0 {
			col = prevChar + deltaChar
		}
		toks = append(toks, tok{line, col, length, mods})
		prevLine = line
		prevChar = col
	}

	// Free-text mention is on line 4 ("We arrived at Vallaki Cathedral after dark.")
	// at column 14. Find tokens overlapping that position.
	var hits []tok
	for _, tk := range toks {
		if tk.line != 4 {
			continue
		}
		if tk.col <= 14 && tk.col+tk.length > 14 {
			hits = append(hits, tk)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("expected exactly 1 token covering the free-text mention, got %d: %+v", len(hits), hits)
	}
	if hits[0].length != uint32(len("Vallaki Cathedral")) {
		t.Fatalf("expected token length %d, got %d", len("Vallaki Cathedral"), hits[0].length)
	}
	if hits[0].mods != cathedralMods {
		t.Fatalf("expected cathedral colour %d, got %d", cathedralMods, hits[0].mods)
	}
}

func TestDefinitionRangesCoversHeadersAndAsides(t *testing.T) {
	content := "Strahd (character): Vampire lord\n  who rules over Barovia.\n\nWalking past, we glimpsed (Tatyana (npc): Strahd's lost love).\n"
	s := setupTestServer(t, content)

	result, err := s.definitionRanges(&DefinitionRangesParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Ranges) != 2 {
		t.Fatalf("expected 2 definition ranges, got %d: %+v", len(result.Ranges), result.Ranges)
	}

	world := soleWorld(t, s)
	colourFor := func(name string) uint32 {
		for i := range world.Entities {
			if world.Entities[i].Name == name {
				return entityColourIndex(&world.Entities[i])
			}
		}
		t.Fatalf("entity %q not found", name)
		return 0
	}

	// Header range: line 0 col 0 → line 1 col 25 (length of "  who rules over Barovia.").
	var header, aside *DefinitionRange
	for i := range result.Ranges {
		r := &result.Ranges[i]
		if r.Range.Start.Line == 0 {
			header = r
		} else {
			aside = r
		}
	}
	if header == nil || aside == nil {
		t.Fatalf("missing one of header/aside ranges: %+v", result.Ranges)
	}

	if header.Range.Start.Line != 0 || header.Range.Start.Character != 0 {
		t.Fatalf("header start wrong: %+v", header.Range.Start)
	}
	if header.Range.End.Line != 1 || header.Range.End.Character != 25 {
		t.Fatalf("header end wrong: %+v", header.Range.End)
	}
	if header.ColourIndex != colourFor("Strahd") {
		t.Fatalf("header colour wrong: got %d want %d", header.ColourIndex, colourFor("Strahd"))
	}

	// Inline aside on line 3, '(' at col 26, ')' at col 60 — End.Character = 61.
	if aside.Range.Start.Line != 3 || aside.Range.Start.Character != 26 {
		t.Fatalf("aside start wrong: %+v", aside.Range.Start)
	}
	if aside.Range.End.Line != 3 || aside.Range.End.Character != 61 {
		t.Fatalf("aside end wrong: %+v", aside.Range.End)
	}
	if aside.ColourIndex != colourFor("Tatyana") {
		t.Fatalf("aside colour wrong: got %d want %d", aside.ColourIndex, colourFor("Tatyana"))
	}
}

func TestFindEntityAtPositionDisambiguates(t *testing.T) {
	content := "Barovia (town): Gothic, dark, misty.\n\nBarovia (country): Perpetually cloudy.\n\nWe entered Barovia (town) from the west.\n"
	s := setupTestServer(t, content)

	ps := s.projects["/test"]
	// Cursor on "Barovia" in the definition of Barovia (town) — line 0.
	match := s.findEntityAtPosition(ps, "file:///test/test.md", protocol.Position{Line: 0, Character: 3})
	if match == nil {
		t.Fatal("expected match")
	}
	if match.Entity.Type != "town" {
		t.Fatalf("expected town, got %q", match.Entity.Type)
	}

	// Cursor on "Barovia" in the definition of Barovia (country) — line 2.
	match = s.findEntityAtPosition(ps, "file:///test/test.md", protocol.Position{Line: 2, Character: 3})
	if match == nil {
		t.Fatal("expected match")
	}
	if match.Entity.Type != "country" {
		t.Fatalf("expected country, got %q", match.Entity.Type)
	}

	// Cursor on "Barovia (town)" in free text — line 4.
	match = s.findEntityAtPosition(ps, "file:///test/test.md", protocol.Position{Line: 4, Character: 14})
	if match == nil {
		t.Fatal("expected match")
	}
	if match.Entity.Type != "town" {
		t.Fatalf("expected town from disambiguated reference, got %q", match.Entity.Type)
	}
}

func TestFindEntityAtPositionLongestMatch(t *testing.T) {
	content := "Count Strahd von Zarovich (character) | Strahd: Vampire lord.\n"
	s := setupTestServer(t, content)

	ps := s.projects["/test"]
	// Cursor on "Strahd" which is part of "Count Strahd von Zarovich" — should match the longer name.
	match := s.findEntityAtPosition(ps, "file:///test/test.md", protocol.Position{Line: 0, Character: 8})
	if match == nil {
		t.Fatal("expected match")
	}
	if match.Entity.Name != "Count Strahd von Zarovich" {
		t.Fatalf("expected longest match, got %q", match.Entity.Name)
	}
}

func TestCompletionSuggestsTagsAfterPlus(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"a.md": "Sildar (character): Fighter. +injured +bleeding\n",
		"b.md": "Gundren (character): A dwarf.\n",
	})
	uri := uriFor("cursor.md")
	openDoc(t, s, uri, "+\n")

	result, err := s.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	list, ok := result.(*protocol.CompletionList)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	var labels []string
	for _, item := range list.Items {
		labels = append(labels, item.Label)
	}
	// Both tags must be suggested.
	want := map[string]bool{"injured": true, "bleeding": true}
	found := 0
	for _, l := range labels {
		if want[l] {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("expected both tags in %v", labels)
	}
}

func TestCompletionSuggestsActiveTagsAfterMinus(t *testing.T) {
	body := "Sildar (character): Fighter. +injured +bleeding -bleeding\n  -\n"
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"a.md": body,
	})
	// After the resolution pass, Sildar has only 'injured' active. Position
	// the cursor on the second line of Sildar's description so the `-` sits
	// inside the entity's owning block.
	uri := uriFor("a.md")
	openDoc(t, s, uri, body)

	result, err := s.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 1, Character: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	list := result.(*protocol.CompletionList)
	labels := completionLabels(list)
	if !labelSetContains(labels, "injured") {
		t.Fatalf("expected 'injured' (currently active) in %v", labels)
	}
	if labelSetContains(labels, "bleeding") {
		t.Fatalf("did not expect 'bleeding' (not currently active) in %v", labels)
	}
}

func TestCompletionMinusScopesToOwningEntity(t *testing.T) {
	// Sildar has 'injured', Cragmaw has 'infested'. Inside Sildar's body, a
	// `-` should only offer 'injured' — not 'infested', which belongs to a
	// different entity.
	body := "Sildar (character): Fighter. +injured\n  -\n\nCragmaw (location): +infested\n"
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"a.md": body,
	})
	uri := uriFor("a.md")
	openDoc(t, s, uri, body)

	result, err := s.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 1, Character: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	labels := completionLabels(result.(*protocol.CompletionList))
	if !labelSetContains(labels, "injured") {
		t.Fatalf("expected 'injured' on Sildar in %v", labels)
	}
	if labelSetContains(labels, "infested") {
		t.Fatalf("did not expect 'infested' (Cragmaw's tag) in %v", labels)
	}
}

func TestCompletionFallsBackToEntitiesWithoutSigil(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"a.md": "Sildar (character): Fighter.\n",
	})
	uri := uriFor("cursor.md")
	openDoc(t, s, uri, "Sil\n")

	result, err := s.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	list := result.(*protocol.CompletionList)
	// The existing entity completion should include "Sildar".
	found := false
	for _, item := range list.Items {
		if item.Label == "Sildar" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected Sildar in entity completions")
	}
}

func TestCompletionListRemoveSuggestsActiveItems(t *testing.T) {
	body := "Sildar (character): inventory = sword, helm, bow.\n  inventory -= \n"
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"a.md": body,
	})
	uri := uriFor("a.md")
	openDoc(t, s, uri, body)

	// Cursor after the trailing space on the `inventory -= ` line.
	line := "  inventory -= "
	res, err := s.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 1, Character: uint32(len(line))},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	list := res.(*protocol.CompletionList)
	got := completionLabels(list)
	want := map[string]bool{"sword": true, "helm": true, "bow": true}
	for w := range want {
		if !labelSetContains(got, w) {
			t.Fatalf("expected %q in active list completions, got %v", w, got)
		}
	}
	for _, label := range got {
		if !want[label] {
			t.Fatalf("unexpected label %q in %v", label, got)
		}
	}
}

func TestCompletionListAppendSuggestsKnownItems(t *testing.T) {
	body := "Sildar (character): inventory = sword, helm; inventory -= helm.\n  inventory += \n"
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"a.md": body,
	})
	uri := uriFor("a.md")
	openDoc(t, s, uri, body)

	line := "  inventory += "
	res, err := s.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 1, Character: uint32(len(line))},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	list := res.(*protocol.CompletionList)
	got := completionLabels(list)
	// Both `helm` (removed earlier) and `sword` should still be suggested as
	// known items for `inventory`, since `+=` looks at history rather than
	// the active set.
	for _, want := range []string{"helm", "sword"} {
		if !labelSetContains(got, want) {
			t.Fatalf("expected %q in known list completions, got %v", want, got)
		}
	}
}

func TestCompletionTagSigilTriggersImmediately(t *testing.T) {
	body := "Sildar (character): Fighter. +injured\n  -\n"
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"a.md": body,
	})
	uri := uriFor("a.md")
	openDoc(t, s, uri, body)

	res, err := s.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 1, Character: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	list := res.(*protocol.CompletionList)
	if !labelSetContains(completionLabels(list), "injured") {
		t.Fatalf("expected 'injured' active tag suggestion right after '-', got %v", completionLabels(list))
	}
}

func completionLabels(list *protocol.CompletionList) []string {
	out := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		out = append(out, item.Label)
	}
	return out
}

func labelSetContains(labels []string, want string) bool {
	return slices.Contains(labels, want)
}

func TestPaletteFromOptionsValidatesHex(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{
			name: "valid 6-digit hex",
			in:   map[string]any{"palette": []any{"#FF0000", "#00ff00", "#abcDEF"}},
			want: []string{"#FF0000", "#00ff00", "#abcDEF"},
		},
		{
			name: "valid 3, 4, 6, 8 digit forms",
			in:   map[string]any{"palette": []any{"#fff", "#abcd", "#aabbcc", "#aabbccdd"}},
			want: []string{"#fff", "#abcd", "#aabbcc", "#aabbccdd"},
		},
		{
			name: "missing leading hash",
			in:   map[string]any{"palette": []any{"FF0000"}},
			want: nil,
		},
		{
			name: "non-hex digit",
			in:   map[string]any{"palette": []any{"#FF00ZZ"}},
			want: nil,
		},
		{
			name: "injection attempt with quote/semicolon",
			in:   map[string]any{"palette": []any{`#fff;"></span><script>alert(1)</script>`}},
			want: nil,
		},
		{
			name: "css colour name",
			in:   map[string]any{"palette": []any{"red"}},
			want: nil,
		},
		{
			name: "non-string entry",
			in:   map[string]any{"palette": []any{"#FF0000", 42}},
			want: nil,
		},
		{
			name: "single bad entry poisons whole palette",
			in:   map[string]any{"palette": []any{"#FF0000", "#nope"}},
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := paletteFromOptions(tc.in)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("paletteFromOptions = %v, want %v", got, tc.want)
			}
		})
	}
}
