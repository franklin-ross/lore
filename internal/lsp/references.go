package lsp

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) references(_ *glsp.Context, params *protocol.ReferenceParams) ([]protocol.Location, error) {
	match := s.findEntityAtPosition(params.TextDocument.URI, params.Position)
	if match == nil {
		return nil, nil
	}

	var locations []protocol.Location

	if params.Context.IncludeDeclaration {
		for _, desc := range match.Entity.Descriptions {
			locations = append(locations, protocol.Location{
				URI:   s.fileToURI(desc.File),
				Range: lineRange(desc.Line),
			})
		}
	}

	refs := s.world().GetReferences(match.Entity.Name)
	for _, ref := range refs {
		locations = append(locations, protocol.Location{
			URI:   s.fileToURI(ref.File),
			Range: lineRange(ref.Line),
		})
	}

	return locations, nil
}
