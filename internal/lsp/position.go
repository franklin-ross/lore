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

// findEntityAtPosition identifies which entity the cursor is on, preferring the longest match.
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
				if col >= start && col <= end {
					if best == nil || (end-start) > (best.End-best.Start) {
						best = &entityMatch{Entity: ent, Start: start, End: end}
					}
				}
			}
		}
	}

	return best
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
