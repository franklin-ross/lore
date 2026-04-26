package lsp

import (
	"encoding/json"
	"sort"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// MethodLoreEntityList is a custom LSP request the tree-view client invokes
// to list every entity with the metadata workspace/symbol can't carry —
// notably resolved tags. Workspace symbols stay flat for Cmd-T; this stream
// is richer and only paid for when the tree view asks for it.
const MethodLoreEntityList = "lore/entityList"

type EntityListParams struct{}

// EntityListItem mirrors enough of lore.Entity for the tree to render an
// entry and jump to the canonical definition span. Tags are the resolved
// state from Merge — already filtered to those currently set.
type EntityListItem struct {
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Aliases  []string       `json:"aliases,omitempty"`
	Tags     []string       `json:"tags,omitempty"`
	Location protocol.Location `json:"location"`
}

type EntityListResult struct {
	Entities []EntityListItem `json:"entities"`
}

func (s *Server) entityList(_ *EntityListParams) (*EntityListResult, error) {
	world := s.world()
	if world == nil {
		return &EntityListResult{}, nil
	}

	out := EntityListResult{Entities: make([]EntityListItem, 0, len(world.Entities))}
	for i := range world.Entities {
		ent := &world.Entities[i]
		desc := canonicalDescription(ent)
		if desc == nil {
			continue
		}
		tags := activeTags(ent.Tags)
		out.Entities = append(out.Entities, EntityListItem{
			Name:    ent.Name,
			Type:    ent.Type,
			Aliases: append([]string(nil), ent.Aliases...),
			Tags:    tags,
			Location: protocol.Location{
				URI: s.fileToURI(desc.File),
				Range: protocol.Range{
					Start: protocol.Position{
						Line:      uint32(desc.Line - 1),
						Character: uint32(desc.StartColumn),
					},
					End: protocol.Position{
						Line:      uint32(desc.EndLine - 1),
						Character: uint32(desc.EndColumn),
					},
				},
			},
		})
	}
	return &out, nil
}

// activeTags returns the set tags from the resolved tag map, sorted for
// stable display order in the client.
func activeTags(tags map[string]bool) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	for tag, set := range tags {
		if set {
			out = append(out, tag)
		}
	}
	sort.Strings(out)
	return out
}

// decodeEntityList unmarshals the request payload. Used by loreHandler.
func decodeEntityList(raw json.RawMessage) (*EntityListParams, error) {
	if len(raw) == 0 {
		return &EntityListParams{}, nil
	}
	p := &EntityListParams{}
	if err := json.Unmarshal(raw, p); err != nil {
		return nil, err
	}
	return p, nil
}
