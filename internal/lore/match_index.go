package lore

import "strings"

// MatchIndex is the pre-lowered lookup table built alongside a World during
// Merge. It exists so reference scanning, semantic token rendering, and
// cursor-position matching can run their hot loops against plain byte
// comparisons instead of re-lowercasing on every iteration.
//
// Entities is indexed in parallel with World.Entities — MatchIndex.Entities[i]
// describes World.Entities[i]. Types holds the deduplicated lower-cased set
// of known entity types, used by the reference scanner to decide whether a
// `(candidate)` suffix counts as a real disambiguator.
type MatchIndex struct {
	Entities []EntityMatch
	Types    map[string]struct{}
}

// EntityMatch is the per-entity lookup row: every field is already lowercased.
type EntityMatch struct {
	LowerName    string
	LowerAliases []string
	LowerType    string // empty when the entity has no type
}

// buildMatchIndex produces a fresh MatchIndex for the given world.
func buildMatchIndex(world *World) *MatchIndex {
	mi := &MatchIndex{
		Entities: make([]EntityMatch, len(world.Entities)),
		Types:    make(map[string]struct{}),
	}
	for i := range world.Entities {
		ent := &world.Entities[i]
		lowerName := strings.ToLower(ent.Name)
		mi.Entities[i].LowerName = lowerName

		if len(ent.Aliases) > 0 {
			aliases := make([]string, len(ent.Aliases))
			for j, a := range ent.Aliases {
				aliases[j] = strings.ToLower(a)
			}
			mi.Entities[i].LowerAliases = aliases
		}

		if ent.Type != "" {
			lowerType := strings.ToLower(ent.Type)
			mi.Entities[i].LowerType = lowerType
			mi.Types[lowerType] = struct{}{}
		}
	}
	return mi
}
