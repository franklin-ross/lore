package lsp

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) definition(_ *glsp.Context, params *protocol.DefinitionParams) (any, error) {
	ps, _ := s.projectForURI(params.TextDocument.URI)
	if ps == nil {
		return nil, nil
	}
	match := s.findEntityAtPosition(ps, params.TextDocument.URI, params.Position)
	if match == nil || len(match.Entity.Descriptions) == 0 {
		return nil, nil
	}

	desc := match.Entity.Descriptions[0]
	return protocol.Location{
		URI:   ps.fileToURI(desc.File),
		Range: lineRange(desc.Line),
	}, nil
}
