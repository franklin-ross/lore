package lsp

import (
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
	s.project = project
	if err := s.index.LoadProject(project); err != nil {
		t.Fatal(err)
	}
	// Editor buffer test URIs use /test as the root; mirror the content there.
	s.index.SetFile("test.md", content)
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

func TestCompletionReturnsAllEntitiesAndAliases(t *testing.T) {
	s := setupTestServer(t, testContent)

	result, err := s.completion(nil, &protocol.CompletionParams{})
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

	result, err := s.completion(nil, &protocol.CompletionParams{})
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
	out := formatEntityHover(ent, "", 0, HoverStateModeLatest, true)
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
	out := formatEntityHover(ent, "t.md", 99, HoverStateModeBoth, true)
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
	out := formatEntityHover(ent, "t.md", 10, HoverStateModeBoth, true)
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
	out := formatEntityHover(ent, "t.md", 10, HoverStateModeBoth, true)
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
	out := formatEntityHover(ent, "t.md", 10, HoverStateModeAtCursor, true)
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
	out := formatEntityHover(ent, "t.md", 10, HoverStateModeLatest, true)
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
	out := formatEntityHover(ent, "t.md", 5, HoverStateModeBoth, true)
	if !strings.Contains(out, "(none)  (latest: +injured)") {
		t.Fatalf("expected tag line with (none)  (latest: +injured); got %q", out)
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
	with := formatEntityHover(ent, "", 0, HoverStateModeLatest, true)
	if !strings.Contains(with, "+injured") {
		t.Fatalf("showStateDirectives=true should include raw directives; got %q", with)
	}
	without := formatEntityHover(ent, "", 0, HoverStateModeLatest, false)
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

func TestFormatEntityHoverOmitsEmptyState(t *testing.T) {
	ent := &lore.Entity{
		Name: "Sildar",
		Type: "character",
		Descriptions: []lore.Description{
			{Text: "Fighter.", File: "t.md", Line: 1},
		},
	}
	out := formatEntityHover(ent, "", 0, HoverStateModeLatest, true)
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

func TestFindEntityAtPositionDisambiguates(t *testing.T) {
	content := "Barovia (town): Gothic, dark, misty.\n\nBarovia (country): Perpetually cloudy.\n\nWe entered Barovia (town) from the west.\n"
	s := setupTestServer(t, content)

	// Cursor on "Barovia" in the definition of Barovia (town) — line 0.
	match := s.findEntityAtPosition("file:///test/test.md", protocol.Position{Line: 0, Character: 3})
	if match == nil {
		t.Fatal("expected match")
	}
	if match.Entity.Type != "town" {
		t.Fatalf("expected town, got %q", match.Entity.Type)
	}

	// Cursor on "Barovia" in the definition of Barovia (country) — line 2.
	match = s.findEntityAtPosition("file:///test/test.md", protocol.Position{Line: 2, Character: 3})
	if match == nil {
		t.Fatal("expected match")
	}
	if match.Entity.Type != "country" {
		t.Fatalf("expected country, got %q", match.Entity.Type)
	}

	// Cursor on "Barovia (town)" in free text — line 4.
	match = s.findEntityAtPosition("file:///test/test.md", protocol.Position{Line: 4, Character: 14})
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

	// Cursor on "Strahd" which is part of "Count Strahd von Zarovich" — should match the longer name.
	match := s.findEntityAtPosition("file:///test/test.md", protocol.Position{Line: 0, Character: 8})
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
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"a.md": "Sildar (character): Fighter. +injured +bleeding -bleeding\n",
	})
	// After the resolution pass, Sildar has only 'injured' active.
	uri := uriFor("cursor.md")
	openDoc(t, s, uri, "-\n")

	result, err := s.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	list := result.(*protocol.CompletionList)
	var labels []string
	for _, item := range list.Items {
		labels = append(labels, item.Label)
	}
	hasInjured := false
	hasBleeding := false
	for _, l := range labels {
		if l == "injured" {
			hasInjured = true
		}
		if l == "bleeding" {
			hasBleeding = true
		}
	}
	if !hasInjured {
		t.Fatalf("expected 'injured' (currently active) in %v", labels)
	}
	if hasBleeding {
		t.Fatalf("did not expect 'bleeding' (not currently active) in %v", labels)
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
