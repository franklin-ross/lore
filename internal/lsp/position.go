package lsp

import (
	"strings"

	"lore/internal/lore"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// entityMatch holds a matched entity and the column span where it was found.
type entityMatch struct {
	Entity *lore.Entity
	Start  int
	End    int
}

// findEntityAtPosition identifies which entity the cursor is on within the
// owning project ps, preferring disambiguated matches ("Name (type)") over
// bare name matches, and longer matches over shorter ones.
func (s *Server) findEntityAtPosition(ps *projectState, uri string, pos protocol.Position) *entityMatch {
	world := ps.world()
	line := s.getLine(uri, pos.Line)
	lowerLine := strings.ToLower(line)
	// pos.Character arrives as a UTF-16 code unit count; lore matches
	// against byte offsets, so translate up front.
	col := bytesForUTF16Units(line, pos.Character)

	var best *entityMatch

	for i := range world.Entities {
		ent := &world.Entities[i]
		names := allNames(ent)
		for _, name := range names {
			lowerName := strings.ToLower(name)
			for _, start := range lore.FindWordMatches(lowerLine, lowerName) {
				end := start + len(name)
				if col < start || col > end {
					continue
				}

				matchEnt := ent
				matchEnd := end

				// If the text has a "(type)" suffix after any number of
				// spaces, extend the match and, if the suffixed type picks
				// a *different* entity than the bare name would, resolve to
				// that one instead.
				if disambigEnd := findDisambigEnd(line, end); disambigEnd > end {
					parenStart := lore.SkipSpaces(line, end)
					disambigText := name + " (" + line[parenStart+1:disambigEnd-1] + ")"
					if resolved, err := world.FindEntity(disambigText); err == nil {
						matchEnt = resolved
					}
					matchEnd = disambigEnd
				}

				span := matchEnd - start
				if best == nil || span > (best.End-best.Start) {
					best = &entityMatch{Entity: matchEnt, Start: start, End: matchEnd}
				}
			}
		}
	}

	return best
}

// findDisambigEnd checks whether line[afterName:] begins with optional
// spaces, "(", some word, ")" and returns the index after the closing
// paren, or -1 if the pattern doesn't match.
func findDisambigEnd(line string, afterName int) int {
	paren := lore.SkipSpaces(line, afterName)
	if paren >= len(line) || line[paren] != '(' {
		return -1
	}
	close := strings.Index(line[paren:], ")")
	if close < 0 {
		return -1
	}
	return paren + close + 1
}

// allNames returns the canonical name followed by all aliases.
func allNames(ent *lore.Entity) []string {
	names := make([]string, 0, 1+len(ent.Aliases))
	names = append(names, ent.Name)
	names = append(names, ent.Aliases...)
	return names
}
