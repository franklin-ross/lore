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

	cursorFile := s.uriToRelPath(params.TextDocument.URI)
	cursorLine := int(params.Position.Line) + 1 // LSP 0-based → lore 1-based
	col := &colouriser{world: s.world(), palette: s.palette}
	content := formatEntityHover(match.Entity, cursorFile, cursorLine, s.hoverStateMode, s.hoverShowStateDirectives, col)
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
