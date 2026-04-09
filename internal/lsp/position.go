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

// findEntityAtPosition identifies which entity the cursor is on, preferring
// disambiguated matches ("Name (type)") over bare name matches, and longer
// matches over shorter ones.
func (s *Server) findEntityAtPosition(uri string, pos protocol.Position) *entityMatch {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.world == nil {
		return nil
	}

	line := s.getLine(uri, pos.Line)
	col := int(pos.Character)

	var best *entityMatch

	for i := range s.world.Entities {
		ent := &s.world.Entities[i]
		names := allNames(ent)
		for _, name := range names {
			for _, start := range findAllIgnoreCase(line, name) {
				end := start + len(name)
				if col < start || col > end {
					continue
				}

				matchEnt := ent
				matchEnd := end

				// Check if this is a disambiguated reference: "Name (type)".
				if ent.Type != "" {
					disambig := name + " (" + ent.Type + ")"
					if start+len(disambig) <= len(line) &&
						strings.EqualFold(line[start:start+len(disambig)], disambig) {
						matchEnd = start + len(disambig)
					}
				}

				// Also check if the text has a (type) suffix that resolves to a
				// *different* entity than the one we matched by bare name.
				if disambigEnd := findDisambigEnd(line, start, name); disambigEnd > end {
					disambigText := line[start:disambigEnd]
					if resolved, err := s.world.FindEntity(disambigText); err == nil {
						matchEnt = resolved
						matchEnd = disambigEnd
					}
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

// findDisambigEnd checks whether line[start:] begins with "name (word)" and
// returns the end index (after the closing paren), or -1 if not.
func findDisambigEnd(line string, start int, name string) int {
	after := start + len(name)
	rest := line[after:]
	if !strings.HasPrefix(rest, " (") {
		return -1
	}
	close := strings.Index(rest, ")")
	if close < 0 {
		return -1
	}
	return after + close + 1
}

// allNames returns the canonical name followed by all aliases.
func allNames(ent *lore.Entity) []string {
	names := make([]string, 0, 1+len(ent.Aliases))
	names = append(names, ent.Name)
	names = append(names, ent.Aliases...)
	return names
}

// findAllIgnoreCase returns all start positions where needle appears in haystack (case-insensitive).
func findAllIgnoreCase(haystack, needle string) []int {
	lower := strings.ToLower(haystack)
	target := strings.ToLower(needle)
	var positions []int
	start := 0
	for {
		idx := strings.Index(lower[start:], target)
		if idx < 0 {
			break
		}
		positions = append(positions, start+idx)
		start += idx + 1
	}
	return positions
}
