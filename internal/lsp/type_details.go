package lsp

import (
	"encoding/json"
	"sort"
	"strings"

	"lore/internal/lore"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// MethodLoreTypeDetails is the LSP request the wiki "type page" invokes
// to fetch every entity of a given type along with each entity's inbound
// references. The shape mirrors the per-entity references section so the
// webview can reuse its row layout.
const MethodLoreTypeDetails = "lore/typeDetails"

// TypeDetailsParams identifies the type to list. TextDocument scopes the
// lookup to one project; without it the first project is used so palette
// invocation works before any markdown is open.
type TypeDetailsParams struct {
	Type         string                           `json:"type"`
	TextDocument *protocol.TextDocumentIdentifier `json:"textDocument,omitempty"`
}

// TypeEntityEntry is one entity belonging to the queried type. Definitions
// is every authored description block for the entity, already colourised so
// entity-name mentions inside the prose paint in palette colours. The wiki
// renders these as clickable rows so the type page reads as a directory of
// where each entity is defined, not a wall of references.
type TypeEntityEntry struct {
	Name        string                   `json:"name"`
	ColourIndex uint32                   `json:"colourIndex"`
	Aliases     []string                 `json:"aliases,omitempty"`
	Tags        []string                 `json:"tags,omitempty"`
	Location    protocol.Location        `json:"location"`
	Definitions []EntityDescriptionBlock `json:"definitions,omitempty"`
}

// TypeDetailsResult is the response body. Found is false when no entity in
// scope claims the requested type.
type TypeDetailsResult struct {
	Found    bool              `json:"found"`
	Type     string            `json:"type,omitempty"`
	Entities []TypeEntityEntry `json:"entities,omitempty"`
}

// typeDetails resolves the requested type in scope and returns every
// matching entity with its inbound refs. Type matching is case-insensitive
// to match the rest of the matcher.
func (s *Server) typeDetails(p *TypeDetailsParams) (*TypeDetailsResult, error) {
	ps := s.entityDetailsScope(&EntityDetailsParams{TextDocument: p.TextDocument})
	if ps == nil || p == nil || strings.TrimSpace(p.Type) == "" {
		return &TypeDetailsResult{}, nil
	}
	world := ps.world()
	want := strings.ToLower(strings.TrimSpace(p.Type))
	var entries []TypeEntityEntry
	canonicalType := ""
	for i := range world.Entities {
		ent := &world.Entities[i]
		if !strings.EqualFold(ent.Type, want) {
			continue
		}
		if canonicalType == "" {
			canonicalType = ent.Type
		}
		entries = append(entries, buildTypeEntityEntry(ps, world, ent))
	}
	if len(entries) == 0 {
		return &TypeDetailsResult{}, nil
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return &TypeDetailsResult{
		Found:    true,
		Type:     canonicalType,
		Entities: entries,
	}, nil
}

func buildTypeEntityEntry(ps *projectState, world *lore.World, ent *lore.Entity) TypeEntityEntry {
	canonical := canonicalDescription(ent)
	loc := protocol.Location{}
	if canonical != nil {
		loc = ps.locAtLine(canonical.File, canonical.Line)
	}
	return TypeEntityEntry{
		Name:        ent.Name,
		ColourIndex: entityColourIndex(ent),
		Aliases:     append([]string(nil), ent.Aliases...),
		Tags:        activeTags(ent.Tags),
		Location:    loc,
		Definitions: buildDescriptionBlocks(ps, world, ent),
	}
}

// decodeTypeDetails unmarshals the request payload. Used by loreHandler.
func decodeTypeDetails(raw json.RawMessage) (*TypeDetailsParams, error) {
	if len(raw) == 0 {
		return &TypeDetailsParams{}, nil
	}
	p := &TypeDetailsParams{}
	if err := json.Unmarshal(raw, p); err != nil {
		return nil, err
	}
	return p, nil
}
