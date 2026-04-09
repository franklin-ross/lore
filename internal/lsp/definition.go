package lsp

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) definition(_ *glsp.Context, params *protocol.DefinitionParams) (any, error) {
	match := s.findEntityAtPosition(params.TextDocument.URI, params.Position)
	if match == nil || len(match.Entity.Descriptions) == 0 {
		return nil, nil
	}

	desc := match.Entity.Descriptions[0]
	return protocol.Location{
		URI:   s.fileToURI(desc.File),
		Range: lineRange(desc.Line),
	}, nil
}
