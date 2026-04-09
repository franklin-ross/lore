package lsp

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) hover(_ *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	match := s.findEntityAtPosition(params.TextDocument.URI, params.Position)
	if match == nil {
		return nil, nil
	}

	content := formatEntityHover(match.Entity)
	line := params.Position.Line
	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: content,
		},
		Range: &protocol.Range{
			Start: protocol.Position{Line: line, Character: uint32(match.Start)},
			End:   protocol.Position{Line: line, Character: uint32(match.End)},
		},
	}, nil
}
