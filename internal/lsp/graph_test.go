package lsp

import (
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestGraphBuildsDefEdges(t *testing.T) {
	s := setupTestServer(t, testContent)
	got, err := s.graph(&GraphParams{
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Nodes) != 2 {
		t.Fatalf("Nodes len = %d, want 2: %+v", len(got.Nodes), got.Nodes)
	}
	want := map[string]bool{"Sildar Hallwinter": true, "Cragmaw Hideout": true}
	for _, n := range got.Nodes {
		if !want[n.Label] {
			t.Errorf("unexpected node %q", n.Label)
		}
	}

	// testContent has Sildar's body mentioning Cragmaw and Cragmaw's body
	// mentioning Sildar — expect both directions.
	if !hasDefEdge(got.DefEdges, "Sildar Hallwinter", "Cragmaw Hideout") {
		t.Errorf("missing Sildar → Cragmaw def edge: %+v", got.DefEdges)
	}
	if !hasDefEdge(got.DefEdges, "Cragmaw Hideout", "Sildar Hallwinter") {
		t.Errorf("missing Cragmaw → Sildar def edge: %+v", got.DefEdges)
	}
}

func TestGraphFiltersSelfDefEdges(t *testing.T) {
	const content = `# Story

Hero (character): A brave Hero with a sword. Hero defeats monsters.
`
	s := setupTestServer(t, content)
	got, err := s.graph(&GraphParams{
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range got.DefEdges {
		if e.From == e.To {
			t.Errorf("self def edge leaked through: %+v", e)
		}
	}
}

func TestGraphMergedWorldFallback(t *testing.T) {
	s := setupTestServer(t, testContent)
	// No URI scope — should still produce graph from merged workspace.
	got, err := s.graph(&GraphParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Nodes) != 2 {
		t.Errorf("merged Nodes len = %d, want 2", len(got.Nodes))
	}
}

func TestGraphUnknownProjectURIReturnsEmpty(t *testing.T) {
	s := setupTestServer(t, testContent)
	got, err := s.graph(&GraphParams{
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///not/in/any/project.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Nodes) != 0 || len(got.DefEdges) != 0 {
		t.Errorf("expected empty result for orphan URI, got %+v", got)
	}
}

func hasDefEdge(edges []GraphDefEdge, from, to string) bool {
	for _, e := range edges {
		if e.From == from && e.To == to {
			return true
		}
	}
	return false
}
