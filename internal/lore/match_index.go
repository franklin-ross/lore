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

	// needles buckets every name + alias by its first byte. The hot scan
	// walks text byte-by-byte and only probes the bucket for the current
	// byte, which drops the inner loop from O(entities) to O(needles
	// sharing this first byte) per text position. With proper-noun-heavy
	// content (mostly distinct first letters), most positions hit empty
	// buckets and skip outright.
	needles [256][]matchNeedle
}

// matchNeedle is one searchable string (a primary name or an alias) plus a
// pointer back to its owning entity. isName lets the scanner apply `(type)`
// disambiguator logic to name hits only.
type matchNeedle struct {
	text      string
	entityIdx int
	isName    bool
}

// EntityMatch is the per-entity lookup row.
type EntityMatch struct {
	Name    string
	Aliases []string
	Type    string // empty when the entity has no type
}

// BuildMatchIndex produces a fresh MatchIndex for the given world. Merge
// calls this on every rebuild; tests and other callers that synthesise a
// World by hand should use it too so the internal needle buckets get
// populated — constructing a MatchIndex via a struct literal yields one
// that finds nothing.
func BuildMatchIndex(world *World) *MatchIndex {
	mi := &MatchIndex{
		Entities: make([]EntityMatch, len(world.Entities)),
		Types:    make(map[string]struct{}),
	}
	for i := range world.Entities {
		ent := &world.Entities[i]
		mi.Entities[i].Name = ent.Name

		if ent.Name != "" {
			b := ent.Name[0]
			mi.needles[b] = append(mi.needles[b], matchNeedle{
				text: ent.Name, entityIdx: i, isName: true,
			})
		}

		if len(ent.Aliases) > 0 {
			aliases := make([]string, len(ent.Aliases))
			copy(aliases, ent.Aliases)
			mi.Entities[i].Aliases = aliases
			for _, a := range aliases {
				if a == "" {
					continue
				}
				b := a[0]
				mi.needles[b] = append(mi.needles[b], matchNeedle{
					text: a, entityIdx: i, isName: false,
				})
			}
		}

		if ent.Type != "" {
			mi.Entities[i].Type = ent.Type
			mi.Types[ent.Type] = struct{}{}
		}
	}
	return mi
}

// candidateHit is one raw needle hit produced by scanCandidates: a word-
// bounded byte-exact match of a name or alias, with no `(type)` disambiguator
// processing applied yet.
type candidateHit struct {
	start, end, entityIdx int
	isName                bool
}

// scanCandidates walks `text` once and emits every word-bounded byte-exact
// hit of any name or alias. The walk visits each byte; positions whose
// first-byte bucket is empty cost a single load and branch. Word-boundary
// checks decode at most two runes (one before the candidate start, one
// after the candidate end) and only run when a bucket has entries — so
// they're not a per-byte cost.
//
// Emits in left-to-right order; multiple hits at the same start byte
// (e.g. several entities sharing a name, or a name plus an alias of the
// same length) are emitted back-to-back. Callers handle disambiguator
// resolution and span dedup.
func (mi *MatchIndex) scanCandidates(text string) []candidateHit {
	if mi == nil || text == "" {
		return nil
	}
	var out []candidateHit
	for i := 0; i < len(text); i++ {
		bucket := mi.needles[text[i]]
		if len(bucket) == 0 {
			continue
		}
		if !isWordBoundaryBefore(text, i) {
			continue
		}
		for _, n := range bucket {
			end := i + len(n.text)
			if end > len(text) {
				continue
			}
			// First byte already matched by bucket index; compare the rest.
			if text[i+1:end] != n.text[1:] {
				continue
			}
			if !isWordBoundaryAfter(text, end) {
				continue
			}
			out = append(out, candidateHit{i, end, n.entityIdx, n.isName})
		}
	}
	return out
}
