package lsp

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) references(_ *glsp.Context, params *protocol.ReferenceParams) ([]protocol.Location, error) {
	ps, _ := s.projectForURI(params.TextDocument.URI)
	if ps == nil {
		return nil, nil
	}
	match := s.findEntityAtPosition(ps, params.TextDocument.URI, params.Position)
	if match == nil {
		return nil, nil
	}

	var locations []protocol.Location

	if params.Context.IncludeDeclaration {
		for _, desc := range match.Entity.Descriptions {
			locations = append(locations, protocol.Location{
				URI:   ps.fileToURI(desc.File),
				Range: lineRange(desc.Line),
			})
		}
	}

	refs := ps.world().GetReferences(match.Entity.Name)
	for _, ref := range refs {
		locations = append(locations, protocol.Location{
			URI:   ps.fileToURI(ref.File),
			Range: lineRange(ref.Line),
		})
	}

	return locations, nil
}
