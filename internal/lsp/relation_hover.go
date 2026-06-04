package lsp

import (
	"fmt"
	"strings"

	"lore/internal/lore"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// relationLabelHover returns a hover for a relation label under the cursor — the
// `father` in `Sarah: father -> Doug` — describing the canonical relation it
// stands in for and the reverse-side label. Returns nil when the cursor isn't
// on a relation label, so the caller can fall through to no hover.
func (s *Server) relationLabelHover(ps *projectState, params *protocol.HoverParams) *protocol.Hover {
	world := ps.world()
	if world.Vocab == nil {
		return nil
	}
	line := s.getLine(params.TextDocument.URI, params.Position.Line)
	col := bytesForUTF16Units(line, params.Position.Character)
	start, end, ok := relationLabelAt(line, col)
	if !ok {
		return nil
	}
	content := relationLabelHoverMarkdown(world.Vocab, line[start:end])
	return &protocol.Hover{
		Contents: protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: content},
		Range: &protocol.Range{
			Start: protocol.Position{Line: params.Position.Line, Character: utf16UnitsForBytes(line, start)},
			End:   protocol.Position{Line: params.Position.Line, Character: utf16UnitsForBytes(line, end)},
		},
	}
}

// relationLabelAt returns the byte span of the relation label at col, if the
// cursor sits on one. A label is a run of label characters immediately followed
// (past spaces) by a relation arrow `->` or `-/>`, and preceded (past spaces) by
// a directive boundary — the line start or a `:`/`;`. That pins it to the label
// slot of a directive and rejects target words and ordinary prose.
func relationLabelAt(line string, col int) (start, end int, ok bool) {
	if col < 0 {
		col = 0
	}
	if col > len(line) {
		col = len(line)
	}
	start, end = col, col
	for start > 0 && isLabelChar(line[start-1]) {
		start--
	}
	for end < len(line) && isLabelChar(line[end]) {
		end++
	}
	if start == end {
		return 0, 0, false
	}
	// Followed by a relation arrow (targets never precede one). Spaces around the
	// arrow are optional, so a tight `father->Doug` or `daughter-of->X` leaves the
	// forward scan having swallowed the arrow's leading `-` into the label (it's a
	// label char). Peel trailing hyphens back until an arrow is exposed.
	for {
		after := lore.SkipSpaces(line, end)
		if strings.HasPrefix(line[after:], "->") || strings.HasPrefix(line[after:], "-/>") {
			break
		}
		if end > start && line[end-1] == '-' {
			end--
			continue
		}
		return 0, 0, false
	}
	if start == end {
		return 0, 0, false // the run was only the arrow's hyphen(s)
	}
	// Preceded by a directive boundary: line start, `:`, or `;`.
	before := start
	for before > 0 && line[before-1] == ' ' {
		before--
	}
	if before > 0 && line[before-1] != ':' && line[before-1] != ';' {
		return 0, 0, false
	}
	return start, end, true
}

// isLabelChar reports whether b can appear in a relation label (letters and the
// hyphen of compounds/genitives like `step-mother`, `daughter-of`).
func isLabelChar(b byte) bool {
	return b == '-' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// relationLabelHoverMarkdown renders the hover for a relation label: the label
// as the heading, then spaced "Field: value" lines — the canonical it normalises
// to, its other surface synonyms, and the reciprocal (the far-side label, which
// is the cue for reading the edge's direction).
func relationLabelHoverMarkdown(vocab *lore.RelationVocab, label string) string {
	canon, known := vocab.Resolve(label)
	if !known {
		return fmt.Sprintf("### %s (generic relation)\n\nNot in the vocabulary — no reciprocal; renders as a named incoming edge on the target.", label)
	}

	var lines []string
	// The canonical, only when the hovered label is an alias of it.
	if !strings.EqualFold(canon, label) {
		lines = append(lines, "**Canonical:** `"+vocab.Display(canon)+"`")
	}
	// The canonical's other surface synonyms, excluding the hovered label.
	var aliases []string
	for _, a := range vocab.SurfaceAliases(canon) {
		if !strings.EqualFold(a, label) {
			aliases = append(aliases, "`"+a+"`")
		}
	}
	if len(aliases) > 0 {
		lines = append(lines, "**Aliases:** "+strings.Join(aliases, ", "))
	}
	// The reciprocal — the far-side label.
	switch recip := vocab.Reciprocal(canon); {
	case recip == "":
		lines = append(lines, "**Reciprocal:** none")
	case strings.EqualFold(recip, canon):
		lines = append(lines, "**Reciprocal:** `"+vocab.Display(recip)+"` (symmetric)")
	default:
		lines = append(lines, "**Reciprocal:** `"+vocab.Display(recip)+"`")
	}

	return fmt.Sprintf("### %s (relation)\n\n%s", label, strings.Join(lines, "\n\n"))
}

// relationReciprocal describes the far-endpoint label of a canonical relation
// for inline use: "reciprocal: child", or "symmetric"/"no reciprocal".
func relationReciprocal(vocab *lore.RelationVocab, canon string) string {
	switch recip := vocab.Reciprocal(canon); {
	case recip == "":
		return "no reciprocal"
	case strings.EqualFold(recip, canon):
		return "symmetric"
	default:
		return "reciprocal: " + vocab.Display(recip)
	}
}
