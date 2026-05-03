package lore

import (
	"sort"
	"strings"
)

// EntitySpan is one matched mention of an entity within a single text
// fragment, in byte offsets relative to the start of that text.
type EntitySpan struct {
	Start     int
	End       int
	EntityIdx int // index into World.Entities
}

// ScanEntities finds every entity mention in `text` and resolves
// overlapping spans using a strict-containment rule:
//
//   - A span strictly contained in a longer kept span is dropped (so
//     "Vistani Camp near Vallaki" suppresses inner "Vistani" / "Vallaki"
//     matches, and a name span suppresses an alias inside it).
//   - Equal spans from different entities are both kept — they're a
//     genuine ambiguity (e.g. two entities sharing the same canonical
//     name but different types, mentioned without a `(type)` suffix).
//   - Equal spans from the same entity (alias literally equal to its
//     own name) collapse to one.
//
// When `includeDisambig` is true, "Name (type)" forms produce extended
// spans through the closing paren so they suppress overlapping plain
// matches; that's the right behaviour for the reference index. When
// false, only plain name and alias matches are produced — appropriate
// for visual highlighting where the user wouldn't expect "(npc)" itself
// to be coloured.
//
// Returns nil when there's no match index, no candidate matches, or the
// input is empty — callers should treat nil as "leave the text alone".
func ScanEntities(world *World, text string, includeDisambig bool) []EntitySpan {
	if world == nil || world.Match == nil || len(world.Match.Entities) == 0 || text == "" {
		return nil
	}
	mi := world.Match
	lower := strings.ToLower(text)

	type cand struct {
		start, end, entityIdx int
	}
	var all []cand

	for i := range mi.Entities {
		em := &mi.Entities[i]

		if includeDisambig && em.LowerType != "" {
			for _, pos := range FindWordMatches(lower, em.LowerName) {
				end := pos + len(em.LowerName)
				after := MatchesTypeSuffix(lower, end, em.LowerType)
				if after < 0 {
					continue
				}
				all = append(all, cand{pos, after, i})
			}
		}

		for _, pos := range FindWordMatches(lower, em.LowerName) {
			end := pos + len(em.LowerName)
			if includeDisambig && MatchesAnyTypeSuffix(lower, end, mi.Types) >= 0 {
				continue
			}
			all = append(all, cand{pos, end, i})
		}

		for _, la := range em.LowerAliases {
			for _, pos := range FindWordMatches(lower, la) {
				all = append(all, cand{pos, pos + len(la), i})
			}
		}
	}
	if len(all) == 0 {
		return nil
	}

	sort.Slice(all, func(a, b int) bool {
		if all[a].start != all[b].start {
			return all[a].start < all[b].start
		}
		if (all[a].end - all[a].start) != (all[b].end - all[b].start) {
			return (all[a].end - all[a].start) > (all[b].end - all[b].start)
		}
		return all[a].entityIdx < all[b].entityIdx
	})

	kept := all[:0]
	for _, m := range all {
		skip := false
		for _, k := range kept {
			if k.start > m.start || m.end > k.end {
				continue
			}
			equal := k.start == m.start && k.end == m.end
			if equal {
				if k.entityIdx == m.entityIdx {
					skip = true
					break
				}
				continue
			}
			skip = true
			break
		}
		if !skip {
			kept = append(kept, m)
		}
	}

	out := make([]EntitySpan, len(kept))
	for i, k := range kept {
		out[i] = EntitySpan{Start: k.start, End: k.end, EntityIdx: k.entityIdx}
	}
	return out
}
