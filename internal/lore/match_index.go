package lore

// MatchIndex is the lookup table built alongside a World during Merge. It
// flattens each entity's name, aliases, and type into a parallel-indexed
// EntityMatch row so reference scanning, semantic token rendering, and
// cursor-position matching can run their hot loops against plain byte
// comparisons without re-walking World.Entities.
//
// Entities is indexed in parallel with World.Entities — MatchIndex.Entities[i]
// describes World.Entities[i]. Types holds the set of known entity types,
// used by the reference scanner to decide whether a `(candidate)` suffix
// counts as a real disambiguator.
//
// All matching is byte-exact and case-sensitive: a prose mention must match
// the entity definition's exact casing. Authors should rely on completion to
// fill names correctly; treating proper nouns case-sensitively keeps the
// scanner cheap (no per-line ToLower) and avoids spurious matches like
// "polish" in "polish the silver" pointing at an entity named "Polish".
type MatchIndex struct {
	Entities []EntityMatch
	Types    map[string]struct{}
}

// EntityMatch is the per-entity lookup row.
type EntityMatch struct {
	Name    string
	Aliases []string
	Type    string // empty when the entity has no type
}

// buildMatchIndex produces a fresh MatchIndex for the given world.
func buildMatchIndex(world *World) *MatchIndex {
	mi := &MatchIndex{
		Entities: make([]EntityMatch, len(world.Entities)),
		Types:    make(map[string]struct{}),
	}
	for i := range world.Entities {
		ent := &world.Entities[i]
		mi.Entities[i].Name = ent.Name

		if len(ent.Aliases) > 0 {
			aliases := make([]string, len(ent.Aliases))
			copy(aliases, ent.Aliases)
			mi.Entities[i].Aliases = aliases
		}

		if ent.Type != "" {
			mi.Entities[i].Type = ent.Type
			mi.Types[ent.Type] = struct{}{}
		}
	}
	return mi
}
