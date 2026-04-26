package lsp

import (
	"strings"

	"lore/internal/lore"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// workspaceSymbol responds to workspace/symbol — VSCode's Cmd-T / Ctrl-T
// "Go to symbol in workspace" picker. Returns one entry per entity across
// every project in the workspace, located at the first description's span.
// The free-text entity type is exposed via containerName so it renders as a
// qualifier next to the name in the picker.
//
// Server-side filtering is loose: a case-insensitive substring match against
// the canonical name and any alias. VSCode refilters fuzzily on the client,
// so this only needs to be permissive, not precise.
func (s *Server) workspaceSymbol(_ *glsp.Context, params *protocol.WorkspaceSymbolParams) ([]protocol.SymbolInformation, error) {
	query := strings.ToLower(strings.TrimSpace(params.Query))

	var out []protocol.SymbolInformation
	for _, ps := range s.projects {
		world := ps.world()
		for i := range world.Entities {
			ent := &world.Entities[i]
			if !entityMatchesQuery(ent, query) {
				continue
			}
			desc := canonicalDescription(ent)
			if desc == nil {
				continue
			}
			container := ent.Type
			out = append(out, protocol.SymbolInformation{
				Name: ent.Name,
				Kind: protocol.SymbolKindObject,
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
				ContainerName: &container,
			})
		}
	}
	return out, nil
}

// entityMatchesQuery reports whether the lowercased query is a substring of
// the entity's canonical name or any of its aliases. An empty query matches
// everything.
func entityMatchesQuery(ent *lore.Entity, query string) bool {
	if query == "" {
		return true
	}
	if strings.Contains(strings.ToLower(ent.Name), query) {
		return true
	}
	for _, alias := range ent.Aliases {
		if strings.Contains(strings.ToLower(alias), query) {
			return true
		}
	}
	return false
}

// canonicalDescription picks the description that should anchor the symbol's
// jump target. Prefers the first description in source order, which is the
// lore.toml header definition when present, otherwise the earliest narrative
// aside.
func canonicalDescription(ent *lore.Entity) *lore.Description {
	if len(ent.Descriptions) == 0 {
		return nil
	}
	return &ent.Descriptions[0]
}
