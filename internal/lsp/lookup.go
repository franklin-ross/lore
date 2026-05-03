package lsp

import (
	"encoding/json"
	"strings"

	"lore/internal/lore"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// MethodLoreLookup is the LSP request the wiki F12 handler invokes when it
// has a word but doesn't yet know whether it names an entity or a type.
// The server probes the scoped world and returns whichever it found, so the
// client opens the right wiki page without bouncing through a not-found
// response first.
const MethodLoreLookup = "lore/lookup"

// LookupParams is the request payload. TextDocument scopes the lookup to
// one project; without it the first project is used so palette invocation
// works before any markdown is open.
type LookupParams struct {
	Name         string                           `json:"name"`
	TextDocument *protocol.TextDocumentIdentifier `json:"textDocument,omitempty"`
}

// LookupResult names the page kind the client should navigate to. Kind is
// "entity", "type", or "none". Value is the canonical entity name or type
// (or empty when Kind is "none").
type LookupResult struct {
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
}

// lookup classifies the supplied name as an entity, a type, or a miss.
// The input is treated as free text, not as a strict identifier: the
// scanner walks it for any known entity name (covers the common F12 case
// where the editor's word range over-matches and grabs surrounding prose,
// e.g. "Sildar arrived at the fortress"). When that finds nothing, falls
// back to an exact type match. Entity wins on collision since name
// matches are far more common than colliding type tokens.
func (s *Server) lookup(p *LookupParams) (*LookupResult, error) {
	if p == nil || strings.TrimSpace(p.Name) == "" {
		return &LookupResult{Kind: "none"}, nil
	}
	ps := s.entityDetailsScope(&EntityDetailsParams{TextDocument: p.TextDocument})
	if ps == nil {
		return &LookupResult{Kind: "none"}, nil
	}
	world := ps.world()
	name := strings.TrimSpace(p.Name)

	// First try the input verbatim — fast path for F12 on a clean name or
	// a `Name (type)` disambiguator that the scanner would otherwise miss
	// because of the parens.
	if ent, err := world.FindEntity(name); err == nil && ent != nil {
		return &LookupResult{Kind: "entity", Value: entityLabel(world, ent.Name, ent.Type)}, nil
	} else {
		var amb *lore.AmbiguousError
		if asAmbiguous(err, &amb) && len(amb.Matches) > 0 {
			first := amb.Matches[0]
			return &LookupResult{Kind: "entity", Value: entityLabel(world, first.Name, first.Type)}, nil
		}
	}

	// Scan the input as free text — picks up an entity name embedded in a
	// longer word range, e.g. "Sildar arrived at the fortress" → Sildar.
	if matches := lore.ScanEntities(world, name, false); len(matches) > 0 {
		ent := &world.Entities[matches[0].EntityIdx]
		return &LookupResult{Kind: "entity", Value: entityLabel(world, ent.Name, ent.Type)}, nil
	}

	for i := range world.Entities {
		if strings.EqualFold(world.Entities[i].Type, name) {
			return &LookupResult{Kind: "type", Value: world.Entities[i].Type}, nil
		}
	}
	return &LookupResult{Kind: "none"}, nil
}

// decodeLookup unmarshals the request payload. Used by loreHandler.
func decodeLookup(raw json.RawMessage) (*LookupParams, error) {
	if len(raw) == 0 {
		return &LookupParams{}, nil
	}
	p := &LookupParams{}
	if err := json.Unmarshal(raw, p); err != nil {
		return nil, err
	}
	return p, nil
}
