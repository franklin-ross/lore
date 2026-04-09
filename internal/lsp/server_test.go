package lsp

import (
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

	paths, err := lore.CollectFiles(fsys, lore.Config{Files: []string{"**/*.md"}})
	if err != nil {
		t.Fatal(err)
	}
	project := &lore.Project{FS: fsys, Config: lore.Config{Files: []string{"**/*.md"}}, FilePaths: paths}

	world, err := lore.Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	s := NewServer()
	s.root = "/test"
	s.project = project
	s.world = world
	s.docs["file:///test/test.md"] = content
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
