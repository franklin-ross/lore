package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lore/internal/lore"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// setupLifecycleServer writes files into a fresh temp directory, loads a
// Server rooted at it, and returns the server plus a helper that builds a
// file:// URI for a project-relative path. A lore.toml is created
// automatically unless the test supplies one.
func setupLifecycleServer(t *testing.T, files map[string]string) (*Server, func(string) string) {
	t.Helper()

	dir := t.TempDir()
	if _, ok := files["lore.toml"]; !ok {
		if err := os.WriteFile(filepath.Join(dir, "lore.toml"), []byte(`files = ["**/*.md"]`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range files {
		abs := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	s := NewServer()
	s.root = dir
	s.discoverAllProjects()

	uriFor := func(rel string) string {
		return "file://" + filepath.Join(dir, rel)
	}
	return s, uriFor
}

// openDoc simulates a textDocument/didOpen notification.
func openDoc(t *testing.T, s *Server, uri, content string) {
	t.Helper()
	if err := s.didOpen(nil, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: uri, Text: content},
	}); err != nil {
		t.Fatal(err)
	}
}

// changeDoc simulates a textDocument/didChange with a full-document update.
func changeDoc(t *testing.T, s *Server, uri, content string) {
	t.Helper()
	if err := s.didChange(nil, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uri},
		},
		ContentChanges: []any{
			protocol.TextDocumentContentChangeEventWhole{Text: content},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

// closeDoc simulates a textDocument/didClose notification.
func closeDoc(t *testing.T, s *Server, uri string) {
	t.Helper()
	if err := s.didClose(nil, &protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	}); err != nil {
		t.Fatal(err)
	}
}

// mustFindEntity fails the test if the given name doesn't resolve in the
// server's current merged world.
func mustFindEntity(t *testing.T, s *Server, name string) *lore.Entity {
	t.Helper()
	ent, err := soleWorld(t, s).FindEntity(name)
	if err != nil {
		t.Fatalf("expected entity %q, got error: %v", name, err)
	}
	return ent
}

func assertNoEntity(t *testing.T, s *Server, name string) {
	t.Helper()
	if _, err := soleWorld(t, s).FindEntity(name); err == nil {
		t.Fatalf("did not expect entity %q to resolve", name)
	}
}

func TestLifecycleLoadProjectReadsDiskFiles(t *testing.T) {
	s, _ := setupLifecycleServer(t, map[string]string{
		"glossary.md": "Sildar (character): Fighter.\n",
	})

	sildar := mustFindEntity(t, s, "Sildar")
	if sildar.Type != "character" {
		t.Fatalf("type = %q", sildar.Type)
	}
}

func TestLifecycleDidChangeRegistersNewEntityImmediately(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"session.md": "Sildar (character): Fighter.\n",
	})

	uri := uriFor("session.md")
	openDoc(t, s, uri, "Sildar (character): Fighter.\n")

	// User types a brand-new entity. Before the fix this wouldn't show up
	// until save — now it must be visible on the very next query.
	changeDoc(t, s, uri, "Sildar (character): Fighter.\n\nKlarg (character): Goblin chief.\n")

	mustFindEntity(t, s, "Klarg")
}

func TestLifecycleDidChangeRemovesDeletedEntities(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"session.md": "Sildar (character): Fighter.\n\nKlarg (character): Goblin chief.\n",
	})

	uri := uriFor("session.md")
	openDoc(t, s, uri, "Sildar (character): Fighter.\n\nKlarg (character): Goblin chief.\n")
	mustFindEntity(t, s, "Klarg")

	// Delete Klarg from the buffer.
	changeDoc(t, s, uri, "Sildar (character): Fighter.\n")

	assertNoEntity(t, s, "Klarg")
}

func TestLifecycleDidCloseRestoresOnDiskContent(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"session.md": "Sildar (character): Fighter.\n",
	})

	uri := uriFor("session.md")
	openDoc(t, s, uri, "Sildar (character): Fighter.\n")

	// Unsaved edit adds Klarg in the buffer only.
	changeDoc(t, s, uri, "Sildar (character): Fighter.\n\nKlarg (character): Goblin chief.\n")
	mustFindEntity(t, s, "Klarg")

	// Closing the buffer should drop the unsaved edit and revert to disk,
	// which still only knows about Sildar.
	closeDoc(t, s, uri)
	assertNoEntity(t, s, "Klarg")
	mustFindEntity(t, s, "Sildar")
}

func TestLifecycleCrossFileReferencesUpdateOnChange(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"glossary/characters.md": "Sildar (character): Fighter.\n",
		"sessions/01.md":         "Nothing happened here.\n",
	})

	// Initially Sildar has no references (its own definition is filtered).
	if refs := soleWorld(t, s).GetReferences("Sildar"); len(refs) != 0 {
		t.Fatalf("expected 0 initial refs to Sildar, got %d", len(refs))
	}

	sessionURI := uriFor("sessions/01.md")
	openDoc(t, s, sessionURI, "Nothing happened here.\n")

	// Mention Sildar in the session buffer.
	changeDoc(t, s, sessionURI, "We rescued Sildar from the goblins.\n")

	refs := soleWorld(t, s).GetReferences("Sildar")
	if len(refs) == 0 {
		t.Fatal("expected a reference to Sildar after editing sessions/01.md")
	}
	found := false
	for _, ref := range refs {
		if ref.File == "sessions/01.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected reference from sessions/01.md, got %+v", refs)
	}
}

func TestLifecycleHoverSeesBufferedChanges(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"session.md": "Sildar (character): Fighter.\n",
	})

	uri := uriFor("session.md")
	openDoc(t, s, uri, "Sildar (character): Fighter.\n")

	// Add Klarg, then hover directly on the name.
	changeDoc(t, s, uri, "Sildar (character): Fighter.\n\nKlarg (character): Goblin chief.\n")

	hover, err := s.hover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 2, Character: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hover == nil {
		t.Fatal("expected hover result for Klarg after didChange")
	}
	mc, ok := hover.Contents.(protocol.MarkupContent)
	if !ok {
		t.Fatalf("unexpected hover contents type %T", hover.Contents)
	}
	if !lore.ContainsIgnoreCase(mc.Value, "Klarg") {
		t.Fatalf("hover content should mention Klarg, got %q", mc.Value)
	}
}

func TestPublishDiagnosticsForStateIssues(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, nil)

	// Capture published diagnostics.
	type published struct {
		uri         string
		diagnostics []protocol.Diagnostic
	}
	var got []published
	s.notify = func(method string, params any) {
		if method != "textDocument/publishDiagnostics" {
			return
		}
		p, ok := params.(*protocol.PublishDiagnosticsParams)
		if !ok {
			t.Fatalf("unexpected params type: %T", params)
		}
		got = append(got, published{uri: p.URI, diagnostics: p.Diagnostics})
	}

	openDoc(t, s, uriFor("sildar.md"), "Sildar (character): Fighter. -injured\n")

	if len(got) == 0 {
		t.Fatal("expected at least one publishDiagnostics notification")
	}
	last := got[len(got)-1]
	if len(last.diagnostics) != 1 {
		t.Fatalf("diagnostics: %+v", last.diagnostics)
	}
	if !strings.Contains(last.diagnostics[0].Message, "injured") {
		t.Fatalf("message: %q", last.diagnostics[0].Message)
	}
}
