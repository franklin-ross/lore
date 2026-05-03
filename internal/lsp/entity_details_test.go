package lsp

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestEntityDetailsBasic(t *testing.T) {
	s := setupTestServer(t, testContent)
	got, err := s.entityDetails(&EntityDetailsParams{
		Entity:       "Sildar",
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found {
		t.Fatalf("expected Found=true, got %+v", got)
	}
	if got.Name != "Sildar Hallwinter" {
		t.Errorf("Name = %q, want %q", got.Name, "Sildar Hallwinter")
	}
	if got.Type != "character" {
		t.Errorf("Type = %q, want character", got.Type)
	}
	if !slices.Contains(got.Aliases, "Sildar") {
		t.Errorf("Aliases missing Sildar: %v", got.Aliases)
	}
	if len(got.Descriptions) == 0 {
		t.Fatal("expected at least one description block")
	}
	desc := got.Descriptions[0]
	if len(desc.Segments) == 0 {
		t.Fatal("description has no segments")
	}
	var joined strings.Builder
	for _, seg := range desc.Segments {
		joined.WriteString(seg.Text)
	}
	if !strings.Contains(joined.String(), "Fighter") {
		t.Errorf("description segments missing prose: %q", joined.String())
	}
	if desc.Location.URI == "" {
		t.Error("description location URI empty")
	}
}

func TestEntityDetailsBodyHighlightsEntities(t *testing.T) {
	s := setupTestServer(t, testContent)
	got, err := s.entityDetails(&EntityDetailsParams{
		Entity:       "Cragmaw Hideout",
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Descriptions) == 0 {
		t.Fatal("expected description for Cragmaw Hideout")
	}
	// Cragmaw's description mentions "Sildar" — that segment should
	// arrive with a non-default colour index so the webview can paint
	// it in Sildar's palette colour.
	var sawColouredSildar bool
	for _, seg := range got.Descriptions[0].Segments {
		if strings.Contains(seg.Text, "Sildar") && seg.ColourIndex >= 0 {
			sawColouredSildar = true
		}
	}
	if !sawColouredSildar {
		t.Errorf("expected coloured Sildar segment in description; got %+v", got.Descriptions[0].Segments)
	}
}

func TestEntityDetailsMissingEntity(t *testing.T) {
	s := setupTestServer(t, testContent)
	got, err := s.entityDetails(&EntityDetailsParams{
		Entity:       "Nonexistent",
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Found {
		t.Fatalf("expected Found=false for missing entity, got %+v", got)
	}
}

func TestEntityDetailsInboundRefsGrouped(t *testing.T) {
	s := setupTestServer(t, testContent)
	got, err := s.entityDetails(&EntityDetailsParams{
		Entity:       "Sildar",
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.InboundRefs) == 0 {
		t.Fatal("expected inbound refs from Cragmaw + free text mention")
	}
	// One group should be free text (Source ""), one should be from
	// Cragmaw Hideout's description prose ("Sildar was captured here.").
	var sawFreeText, sawCragmaw bool
	for _, g := range got.InboundRefs {
		switch g.Source {
		case "":
			sawFreeText = true
		case "Cragmaw Hideout":
			sawCragmaw = true
		}
		if len(g.Refs) == 0 {
			t.Errorf("group %q has no refs", g.Source)
		}
	}
	if !sawFreeText {
		t.Errorf("missing free-text inbound group; got %+v", got.InboundRefs)
	}
	if !sawCragmaw {
		t.Errorf("missing Cragmaw Hideout inbound group; got %+v", got.InboundRefs)
	}
}

// An entity's own description names the entity (and its aliases) inside
// itself; those are self-references and shouldn't surface in the wiki's
// "Mentions" list.
func TestEntityDetailsOutboundExcludesSelfRefs(t *testing.T) {
	const content = `Captain Casimir (npc) | Casimir: Dusk elf with ties to the Vistani.

Vistani (faction): Wandering folk.
`
	s := setupTestServer(t, content)
	got, err := s.entityDetails(&EntityDetailsParams{
		Entity:       "Captain Casimir",
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range got.OutboundRefs {
		if g.Source == "Captain Casimir" {
			t.Errorf("outbound included self-ref group: %+v", g)
		}
	}
}

func TestEntityDetailsOutboundRefs(t *testing.T) {
	s := setupTestServer(t, testContent)
	got, err := s.entityDetails(&EntityDetailsParams{
		Entity:       "Cragmaw Hideout",
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.OutboundRefs) == 0 {
		t.Fatal("expected outbound refs to Sildar")
	}
	var sawSildar bool
	for _, g := range got.OutboundRefs {
		if g.Source == "Sildar Hallwinter" {
			sawSildar = true
		}
	}
	if !sawSildar {
		names := make([]string, 0, len(got.OutboundRefs))
		for _, g := range got.OutboundRefs {
			names = append(names, g.Source)
		}
		t.Errorf("outbound groups want Sildar Hallwinter, got %v", names)
	}
}

// Long sentences containing multiple references would otherwise render
// as identical wiki rows. The build pipeline crops each ref preview
// around its match.
func TestEntityDetailsRefPreviewTrimsToMatch(t *testing.T) {
	const content = `Vallaki (location): Walled town.

Casimir (npc): Yltry meets the dusk elf at the gates of Vallaki on a misty morning.
`
	s := setupTestServer(t, content)
	got, err := s.entityDetails(&EntityDetailsParams{
		Entity:       "Vallaki",
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var preview string
	for _, g := range got.InboundRefs {
		if g.Source != "Casimir" {
			continue
		}
		for _, r := range g.Refs {
			var b strings.Builder
			for _, seg := range r.Segments {
				b.WriteString(seg.Text)
			}
			preview = b.String()
		}
	}
	if preview == "" {
		t.Fatal("expected ref to Vallaki from Casimir")
	}
	if !strings.HasPrefix(preview, "… ") {
		t.Errorf("expected leading ellipsis, got %q", preview)
	}
	if !strings.Contains(preview, "Vallaki") {
		t.Errorf("trimmed preview missing match: %q", preview)
	}
}

// Plain (uncoloured) ContextSegments should marshal without a
// "colourIndex" field; coloured segments include it. Keeps wire payload
// small and the JSON shape obvious to webview readers.
func TestContextSegmentJSONOmitsSentinel(t *testing.T) {
	plain, err := json.Marshal(ContextSegment{Text: "hello", ColourIndex: -1})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(plain); got != `{"text":"hello"}` {
		t.Errorf("plain segment marshalled %q", got)
	}
	coloured, err := json.Marshal(ContextSegment{Text: "Sildar", ColourIndex: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(coloured); got != `{"text":"Sildar","colourIndex":3}` {
		t.Errorf("coloured segment marshalled %q", got)
	}
}

func TestEntityRefGroupJSONOmitsSentinel(t *testing.T) {
	free, err := json.Marshal(EntityRefGroup{Source: "", ColourIndex: -1, Refs: nil})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(free); got != `{"source":"","refs":null}` {
		t.Errorf("free-text group marshalled %q", got)
	}
	coloured, err := json.Marshal(EntityRefGroup{Source: "Sildar", ColourIndex: 1, Refs: nil})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(coloured); got != `{"source":"Sildar","colourIndex":1,"refs":null}` {
		t.Errorf("coloured group marshalled %q", got)
	}
}
