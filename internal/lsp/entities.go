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

// EntityListParams optionally scopes the result set to a single project. If
// TextDocument is set, only entities from the project owning that URI are
// returned, matching VSCode's "active editor" tree-view scope. When unset,
// entities from every project are merged so the tree still shows something
// before any markdown editor is focused.
type EntityListParams struct {
	TextDocument *protocol.TextDocumentIdentifier `json:"textDocument,omitempty"`
}

// EntityListItem mirrors enough of lore.Entity for the tree to render an
// entry and jump to the canonical definition span. Tags are the resolved
// state from Merge — already filtered to those currently set.
type EntityListItem struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Aliases  []string          `json:"aliases,omitempty"`
	Tags     []string          `json:"tags,omitempty"`
	Location protocol.Location `json:"location"`
}

type EntityListResult struct {
	Entities []EntityListItem `json:"entities"`
	// Message, when set, signals the workspace has no scope to list from
	// (no lore.toml found). Clients should display it in place of an
	// empty list rather than render a functional-looking empty view.
	Message string `json:"message,omitempty"`
}

func (s *Server) entityList(params *EntityListParams) (*EntityListResult, error) {
	out := EntityListResult{Entities: []EntityListItem{}}
	if len(s.projects) == 0 {
		out.Message = "No lore.toml found in this workspace."
		return &out, nil
	}

	scope := s.entityListScope(params)
	for _, ps := range scope {
		appendProjectEntities(&out.Entities, ps)
	}
	return &out, nil
}

// entityListScope returns the set of projects to include in the response.
// When the request carries an active textDocument URI, scope is the single
// owning project; otherwise every project is returned so the tree has
// content even before a markdown buffer is focused.
func (s *Server) entityListScope(params *EntityListParams) []*projectState {
	if params != nil && params.TextDocument != nil && params.TextDocument.URI != "" {
		if ps, _ := s.projectForURI(params.TextDocument.URI); ps != nil {
			return []*projectState{ps}
		}
		// URI was supplied but no project owns it (e.g. file outside any
		// lore.toml subtree). Return empty so the tree clearly reflects
		// "this file isn't part of any campaign".
		return nil
	}
	out := make([]*projectState, 0, len(s.projects))
	for _, ps := range s.projects {
		out = append(out, ps)
	}
	return out
}

func appendProjectEntities(out *[]EntityListItem, ps *projectState) {
	world := ps.world()
	for i := range world.Entities {
		ent := &world.Entities[i]
		desc := canonicalDescription(ent)
		if desc == nil {
			continue
		}
		tags := activeTags(ent.Tags)
		*out = append(*out, EntityListItem{
			Name:    ent.Name,
			Type:    ent.Type,
			Aliases: append([]string(nil), ent.Aliases...),
			Tags:    tags,
			Location: protocol.Location{
				URI: ps.fileToURI(desc.File),
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
