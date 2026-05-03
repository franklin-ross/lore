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

// Inbound refs should point at the matched substring on the source line
// — not a whole-line range — so the wiki click selects exactly the entity
// name span in the editor.
func TestEntityDetailsRefLocationIsPreciseSpan(t *testing.T) {
	const content = `Sildar Hallwinter (character) | Sildar: Fighter.

Cragmaw Hideout (location): North of Triboar Trail. Sildar was captured here.
`
	s := setupTestServer(t, content)
	got, err := s.entityDetails(&EntityDetailsParams{
		Entity:       "Sildar",
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, g := range got.InboundRefs {
		if g.Source != "Cragmaw Hideout" {
			continue
		}
		for _, r := range g.Refs {
			rng := r.Location.Range
			// Sub-line range — start.character > 0 and end.character > start.character.
			if rng.Start.Line != rng.End.Line {
				t.Errorf("expected single-line range, got %+v", rng)
			}
			if rng.Start.Character == rng.End.Character {
				t.Errorf("expected non-empty range, got %+v", rng)
			}
			if rng.Start.Character == 0 && rng.End.Character == 0 {
				t.Errorf("expected sub-line span, got whole-line %+v", rng)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no Sildar ref from Cragmaw Hideout")
	}
}

// Description blocks should locate the entity name on the definition line
// with the `(type)` suffix included so the wiki type-page row click
// selects the disambiguator alongside the name.
func TestEntityDetailsDescriptionLocationCoversNameAndType(t *testing.T) {
	const content = `Sildar Hallwinter (character) | Sildar: Fighter.
`
	s := setupTestServer(t, content)
	got, err := s.entityDetails(&EntityDetailsParams{
		Entity:       "Sildar Hallwinter",
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Descriptions) == 0 {
		t.Fatal("expected one description")
	}
	rng := got.Descriptions[0].Location.Range
	// Span = "Sildar Hallwinter (character)" on line 0, columns 0..29.
	want := "Sildar Hallwinter (character)"
	if rng.Start.Character != 0 || rng.End.Character != uint32(len(want)) {
		t.Errorf("range = %+v, want 0..%d (the %q span)", rng, len(want), want)
	}
}

// Inline asides start with an opening paren; the description location must
// skip it so the selection lands on the first letter of the name.
func TestEntityDetailsAsideDescriptionLocationSkipsParen(t *testing.T) {
	const content = `Vallaki (location): Walled town.

Session intro. (Vallaki: Wolf heads on the gates).
`
	s := setupTestServer(t, content)
	got, err := s.entityDetails(&EntityDetailsParams{
		Entity:       "Vallaki",
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Descriptions) < 2 {
		t.Fatal("expected header + aside descriptions")
	}
	// Find the aside (Line 3, the inline `(Vallaki: ...)`).
	var aside *EntityDescriptionBlock
	for i := range got.Descriptions {
		if got.Descriptions[i].StartLine == 3 {
			aside = &got.Descriptions[i]
			break
		}
	}
	if aside == nil {
		t.Fatal("expected aside on line 3")
	}
	asideLineStart := strings.Index(content, "Session intro.")
	asideOpen := strings.Index(content[asideLineStart:], "(") + asideLineStart
	wantStart := uint32(asideOpen - asideLineStart + 1) // skip `(`
	wantEnd := wantStart + uint32(len("Vallaki"))
	if aside.Location.Range.Start.Character != wantStart || aside.Location.Range.End.Character != wantEnd {
		t.Errorf("range = %+v, want %d..%d", aside.Location.Range, wantStart, wantEnd)
	}
}

// State history rows should jump to the directive's exact span on its
// source line, not to a whole-line range.
func TestEntityDetailsStateHistoryLocationIsPreciseSpan(t *testing.T) {
	const content = `Sildar Hallwinter (character): Fighter. +injured
`
	s := setupTestServer(t, content)
	got, err := s.entityDetails(&EntityDetailsParams{
		Entity:       "Sildar Hallwinter",
		TextDocument: &protocol.TextDocumentIdentifier{URI: "file:///test/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.StateHistory) == 0 {
		t.Fatal("expected one state event")
	}
	ev := got.StateHistory[0]
	if ev.Op != "add" || ev.Target != "injured" {
		t.Fatalf("unexpected event %+v", ev)
	}
	rng := ev.Location.Range
	wantStart := uint32(strings.Index(content, "+injured"))
	wantEnd := wantStart + uint32(len("+injured"))
	if rng.Start.Line != rng.End.Line {
		t.Errorf("expected single-line range, got %+v", rng)
	}
	if rng.Start.Character != wantStart || rng.End.Character != wantEnd {
		t.Errorf("range = %+v, want %d..%d", rng, wantStart, wantEnd)
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
