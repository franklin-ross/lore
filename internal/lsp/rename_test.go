package lsp

import (
	"sort"
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

const renameContent = `# Session 1

Sildar Hallwinter (character) | Sildar: Fighter. Rescued from
  Cragmaw Hideout.

Cragmaw Hideout (location): North of Triboar Trail. Sildar was
  captured here.

We followed the trail and found Sildar Hallwinter inside.
`

func collectRenameEditTexts(t *testing.T, s *Server, uri string, line, char uint32, newName string) map[string][]protocol.TextEdit {
	t.Helper()
	edit, err := s.rename(nil, &protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: line, Character: char},
		},
		NewName: newName,
	})
	if err != nil {
		t.Fatalf("rename returned error: %v", err)
	}
	if edit == nil {
		t.Fatal("expected workspace edit, got nil")
	}
	out := make(map[string][]protocol.TextEdit, len(edit.Changes))
	for u, edits := range edit.Changes {
		sorted := append([]protocol.TextEdit(nil), edits...)
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].Range.Start.Line != sorted[j].Range.Start.Line {
				return sorted[i].Range.Start.Line < sorted[j].Range.Start.Line
			}
			return sorted[i].Range.Start.Character < sorted[j].Range.Start.Character
		})
		out[string(u)] = sorted
	}
	return out
}

func TestPrepareRenameOnAlias(t *testing.T) {
	s := setupTestServer(t, renameContent)

	// "Sildar" alias appears on line 2 (0-based) at column 32 within the header.
	result, err := s.prepareRename(nil, &protocol.PrepareRenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
			Position:     protocol.Position{Line: 2, Character: 33},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rwp, ok := result.(protocol.RangeWithPlaceholder)
	if !ok {
		t.Fatalf("expected RangeWithPlaceholder, got %T", result)
	}
	if rwp.Placeholder != "Sildar" {
		t.Fatalf("placeholder = %q, want Sildar", rwp.Placeholder)
	}
	if rwp.Range.End.Character-rwp.Range.Start.Character != 6 {
		t.Fatalf("range width = %d, want 6 (Sildar)", rwp.Range.End.Character-rwp.Range.Start.Character)
	}
}

func TestRenameAliasOnlyTouchesAlias(t *testing.T) {
	s := setupTestServer(t, renameContent)

	// Cursor on the prose mention of "Sildar" on line 5 (0-based) col 53.
	changes := collectRenameEditTexts(t, s, "file:///test/test.md", 5, 53, "Sil")
	uri := "file:///test/test.md"
	edits, ok := changes[uri]
	if !ok {
		t.Fatalf("expected edits for %q, got %v", uri, changes)
	}

	// Expected spans (0-based line, byte columns):
	//   line 2 col 32-38 — alias slot in header
	//   line 5 col 52-58 — prose mention in `Cragmaw Hideout` definition body
	// The canonical name `Sildar Hallwinter` (lines 2 and 8) must be left
	// alone — only the bare-Sildar spelling renames.
	wantRanges := []struct {
		line, startChar, endChar uint32
	}{
		{2, 32, 38},
		{5, 52, 58},
	}
	if len(edits) != len(wantRanges) {
		t.Fatalf("got %d edits, want %d: %+v", len(edits), len(wantRanges), edits)
	}
	for i, want := range wantRanges {
		got := edits[i].Range
		if got.Start.Line != want.line || got.Start.Character != want.startChar || got.End.Character != want.endChar {
			t.Errorf("edit %d range = %+v, want line=%d [%d,%d)", i, got, want.line, want.startChar, want.endChar)
		}
		if edits[i].NewText != "Sil" {
			t.Errorf("edit %d NewText = %q, want Sil", i, edits[i].NewText)
		}
	}
}

func TestRenameCanonicalNameTouchesCanonicalOnly(t *testing.T) {
	s := setupTestServer(t, renameContent)

	// Cursor on `Sildar Hallwinter` on line 8 (0-based) at col 36.
	changes := collectRenameEditTexts(t, s, "file:///test/test.md", 8, 36, "Sildar Bravearm")
	edits := changes["file:///test/test.md"]
	if len(edits) != 2 {
		t.Fatalf("got %d edits, want 2: %+v", len(edits), edits)
	}
	for _, e := range edits {
		if e.NewText != "Sildar Bravearm" {
			t.Errorf("NewText = %q, want Sildar Bravearm", e.NewText)
		}
	}
	// Header occurrence on line 2 col 0-17, prose mention on line 8 col 32-49.
	if edits[0].Range.Start.Line != 2 || edits[0].Range.Start.Character != 0 || edits[0].Range.End.Character != 17 {
		t.Errorf("header edit = %+v", edits[0].Range)
	}
	if edits[1].Range.Start.Line != 8 || edits[1].Range.Start.Character != 32 || edits[1].Range.End.Character != 49 {
		t.Errorf("prose edit = %+v", edits[1].Range)
	}
}

func TestRenameRejectsInvalidName(t *testing.T) {
	s := setupTestServer(t, renameContent)

	for _, bad := range []string{"", "  ", "Foo:Bar", "Foo|Bar", "Foo (x)"} {
		_, err := s.rename(nil, &protocol.RenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
				Position:     protocol.Position{Line: 2, Character: 33},
			},
			NewName: bad,
		})
		if err == nil {
			t.Errorf("rename(%q) = nil error, want validation failure", bad)
		}
	}
}

func TestPrepareRenameMissesNonEntity(t *testing.T) {
	s := setupTestServer(t, renameContent)
	// Column inside `# Session 1` — not an entity.
	result, err := s.prepareRename(nil, &protocol.PrepareRenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
			Position:     protocol.Position{Line: 0, Character: 4},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatalf("expected nil prepareRename, got %v", result)
	}
}

// Regression for header-locator: locating an alias must not return the
// canonical-name segment, and locating the canonical name must not return
// the alias segment.
func TestRenameAliasInsideHeaderIsolated(t *testing.T) {
	s := setupTestServer(t, "Sildar Hallwinter (character) | Sildar: Fighter.\n")
	uri := "file:///test/test.md"
	// Place cursor on alias `Sildar` (cols 32-38).
	changes := collectRenameEditTexts(t, s, uri, 0, 33, "Sil")
	edits := changes[uri]
	if len(edits) != 1 {
		t.Fatalf("got %d edits, want 1: %+v", len(edits), edits)
	}
	if edits[0].Range.Start.Character != 32 || edits[0].Range.End.Character != 38 {
		t.Errorf("alias edit range = %+v, want [32,38)", edits[0].Range)
	}

	// Cursor on canonical `Sildar Hallwinter`.
	changes = collectRenameEditTexts(t, s, uri, 0, 5, "Steve")
	edits = changes[uri]
	if len(edits) != 1 {
		t.Fatalf("got %d edits, want 1: %+v", len(edits), edits)
	}
	if edits[0].Range.Start.Character != 0 || edits[0].Range.End.Character != 17 {
		t.Errorf("canonical edit range = %+v, want [0,17)", edits[0].Range)
	}
	if !strings.Contains(uri, "test.md") {
		t.Fatal("sanity check failed")
	}
}
