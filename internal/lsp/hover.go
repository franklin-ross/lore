package lsp

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) hover(_ *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	ps, rel := s.projectForURI(params.TextDocument.URI)
	if ps == nil {
		return nil, nil
	}
	match := s.findEntityAtPosition(ps, params.TextDocument.URI, params.Position)
	if match == nil {
		return nil, nil
	}

	cursorLine := int(params.Position.Line) + 1 // LSP 0-based → lore 1-based
	col := &colouriser{world: ps.world(), palette: s.palette}
	content := formatEntityHover(match.Entity, rel, cursorLine, s.hoverStateMode, s.hoverShowStateDirectives, col)
	line := params.Position.Line
	lineText := s.getLine(params.TextDocument.URI, line)
	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: content,
		},
		Range: &protocol.Range{
			Start: protocol.Position{Line: line, Character: utf16UnitsForBytes(lineText, match.Start)},
			End:   protocol.Position{Line: line, Character: utf16UnitsForBytes(lineText, match.End)},
		},
	}, nil
}
