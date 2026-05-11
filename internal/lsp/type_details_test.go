package lsp

import (
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestTypeDetailsListsEntities(t *testing.T) {
	s := setupTestServer(t, testContent)
	got, err := s.typeDetails(&TypeDetailsParams{
		Type:         "character",
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found {
		t.Fatalf("expected Found=true for type 'character', got %+v", got)
	}
	if got.Type != "character" {
		t.Errorf("Type = %q, want character", got.Type)
	}
	if len(got.Entities) != 1 {
		t.Fatalf("expected one character entity, got %d", len(got.Entities))
	}
	ent := got.Entities[0]
	if ent.Name != "Sildar Hallwinter" {
		t.Errorf("entity name = %q", ent.Name)
	}
	if len(ent.Definitions) == 0 {
		t.Errorf("expected at least one definition for %s", ent.Name)
	}
	if len(ent.Definitions) > 0 && len(flattenContentSegments(ent.Definitions[0].Content)) == 0 {
		t.Errorf("first definition has no segments for %s", ent.Name)
	}
}

func TestTypeDetailsCaseInsensitive(t *testing.T) {
	s := setupTestServer(t, testContent)
	got, err := s.typeDetails(&TypeDetailsParams{
		Type:         "LOCATION",
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found {
		t.Fatalf("expected Found=true for type 'LOCATION', got %+v", got)
	}
	if len(got.Entities) != 1 || got.Entities[0].Name != "Cragmaw Hideout" {
		t.Errorf("expected only Cragmaw Hideout, got %+v", got.Entities)
	}
}

func TestTypeDetailsMissingType(t *testing.T) {
	s := setupTestServer(t, testContent)
	got, err := s.typeDetails(&TypeDetailsParams{
		Type:         "deity",
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Found {
		t.Fatalf("expected Found=false for missing type, got %+v", got)
	}
}
