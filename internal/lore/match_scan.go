package lore

import (
	"sort"
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
//     name and no `(type)` suffix to disambiguate).
//   - Equal spans from the same entity (alias literally equal to its
//     own name) collapse to one.
//
// `(type)` suffixes following a name match are always honoured: only
// the entity whose type matches the suffix is emitted; other same-name
// entities are skipped at that position. When `includeDisambig` is
// true, the matching entity's span is extended through the closing
// paren so it suppresses overlapping plain matches in the reference
// index. When false, the span covers just the name — appropriate for
// visual highlighting where the user wouldn't expect "(country)"
// itself to be coloured.
//
// Returns nil when there's no match index, no candidate matches, or the
// input is empty — callers should treat nil as "leave the text alone".
func ScanEntities(world *World, text string, includeDisambig bool) []EntitySpan {
	if world == nil || world.Match == nil || len(world.Match.Entities) == 0 || text == "" {
		return nil
	}
	mi := world.Match

	type cand struct {
		start, end, entityIdx int
	}
	var all []cand

	for i := range mi.Entities {
		em := &mi.Entities[i]

		for _, pos := range FindWordMatches(text, em.Name) {
			end := pos + len(em.Name)
			suffixPresent := MatchesAnyTypeSuffix(text, end, mi.Types) >= 0
			ownSuffixEnd := -1
			if em.Type != "" {
				ownSuffixEnd = MatchesTypeSuffix(text, end, em.Type)
			}
			if suffixPresent && ownSuffixEnd < 0 {
				// `(type)` follows but it's a different entity's type.
				// The user has disambiguated away from this entity.
				continue
			}
			if includeDisambig && ownSuffixEnd >= 0 {
				all = append(all, cand{pos, ownSuffixEnd, i})
				continue
			}
			all = append(all, cand{pos, end, i})
		}

		for _, alias := range em.Aliases {
			for _, pos := range FindWordMatches(text, alias) {
				all = append(all, cand{pos, pos + len(alias), i})
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
