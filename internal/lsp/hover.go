package lsp

import (
	"lore/internal/lore"

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
	// A definition block reads as one beat, so resolve "at cursor" state as of
	// the end of the block the cursor sits in — directives later in the same
	// block count, rather than being cut off at the exact hovered line.
	cutoffLine := blockEndLine(ps.world(), rel, cursorLine)
	col := &colouriser{world: ps.world(), palette: s.palette}
	content := formatEntityHover(ps.world(), match.Entity, rel, cutoffLine, s.hoverStateMode, s.hoverShowStateDirectives, col)
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

// blockEndLine returns the last line of the definition block that contains
// cursorLine in file, so hover resolves "at cursor" state as of the end of the
// whole block — a multi-line entity definition reads as a single beat, not a
// per-line timeline. This covers both header definitions and paren-wrapped
// asides: a block-form aside `(Name: …)` can span many paragraphs, and its
// EndLine is the close-paren line, so hovering inside it snaps the cutoff to
// the aside's end. An aside nested inside a header definition is also fine —
// the enclosing definition's span already contains the cursor, and the larger
// EndLine wins. Returns cursorLine unchanged when the cursor isn't inside any
// definition.
func blockEndLine(world *lore.World, file string, cursorLine int) int {
	end := cursorLine
	for i := range world.Entities {
		for _, d := range world.Entities[i].Descriptions {
			if d.File != file {
				continue
			}
			if d.Line <= cursorLine && cursorLine <= d.EndLine && d.EndLine > end {
				end = d.EndLine
			}
		}
	}
	return end
}
