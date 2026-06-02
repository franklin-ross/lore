package lore

import (
	"sort"
	"strings"
)

// RelationDef defines a relation type: its canonical name, the canonical name
// of its reciprocal (the label shown on the far endpoint), an optional display
// plural, and the surface labels that normalise to it.
//
// Reciprocal is the canonical name of the reverse relation. A relation whose
// reciprocal is itself (Canonical == Reciprocal) is symmetric — `spouse`,
// `sibling`, `friend`. An empty Reciprocal means the relation has no defined
// reverse, so the far side renders as a named-incoming edge.
type RelationDef struct {
	Canonical  string
	Reciprocal string
	Plural     string
	Aliases    []string
}

// RelationVocab resolves relation labels to canonical relations and answers
// reciprocal and plural lookups. Build it from BuiltinRelations overlaid with
// any lore.toml [relations.*] entries via NewRelationVocab.
//
// Lookup is case-insensitive (labels are vocabulary words, not entity names),
// but resolution is display-preserving: callers keep the surface label the
// author typed and consult the vocab only for canonicalisation, reciprocity,
// and pluralisation — mirroring how entity aliases behave.
type RelationVocab struct {
	byCanonical map[string]RelationDef // canonical (lowercased) -> def
	aliasIndex  map[string]string      // lowercased label or alias -> canonical
}

// BuiltinRelations is the default vocabulary shipped for common familial,
// social, and membership relations. Users extend or override these by defining
// a relation of the same canonical name in lore.toml.
func BuiltinRelations() []RelationDef {
	return []RelationDef{
		{Canonical: "parent", Reciprocal: "child", Aliases: []string{"father", "mother", "dad", "mum", "mom"}},
		{Canonical: "child", Plural: "children", Aliases: []string{"son", "daughter", "kid"}},
		{Canonical: "step-parent", Reciprocal: "step-child", Aliases: []string{"step-father", "step-mother"}},
		{Canonical: "step-child", Plural: "step-children", Aliases: []string{"step-son", "step-daughters"}},
		{Canonical: "sibling", Reciprocal: "sibling", Aliases: []string{"brother", "sister", "half-brother", "half-sister"}},
		{Canonical: "step-sibling", Reciprocal: "step-sibling", Aliases: []string{"step-brother", "step-sister"}},
		// Gender variants are aliases of one canonical, never separate
		// canonicals — two canonicals sharing a reciprocal break edge identity
		// (a reciprocal is a one-to-one back-pointer). English lacks a common
		// neutral word for aunt/uncle and niece/nephew, so the linguistic
		// neutrals pibling/nibling are the canonicals. They only ever surface
		// on an undeclared reverse side, where gender is unknown anyway.
		{Canonical: "pibling", Reciprocal: "nibling", Aliases: []string{"aunt", "uncle"}},
		{Canonical: "nibling", Aliases: []string{"niece", "nephew"}},
		{Canonical: "cousin", Reciprocal: "cousin"},
		{Canonical: "grandparent", Reciprocal: "grandchild"},
		{Canonical: "grandchild", Plural: "grandchildren"},
		{Canonical: "spouse", Reciprocal: "spouse", Aliases: []string{"husband", "wife", "partner", "married"}},
		{Canonical: "member", Reciprocal: "member-of", Aliases: []string{"members"}},
		{Canonical: "member-of"},
		{Canonical: "contains", Reciprocal: "within"},
		{Canonical: "within", Aliases: []string{"inside"}},
	}
}

// NewRelationVocab builds a vocabulary from the given definitions. Later
// definitions override earlier ones with the same canonical name, so callers
// pass built-ins first and config entries second.
//
// Reciprocity is made bidirectional: defining parent.reciprocal = child also
// gives child.reciprocal = parent, unless child already declares its own
// reciprocal. The canonical name itself always resolves as a label, in
// addition to its aliases.
func NewRelationVocab(defs []RelationDef) *RelationVocab {
	v := &RelationVocab{
		byCanonical: make(map[string]RelationDef),
		aliasIndex:  make(map[string]string),
	}
	for _, d := range defs {
		canon := canonKey(d.Canonical)
		if canon == "" {
			continue
		}
		// Keep the original casing as the display name; the lowercased canon
		// is the lookup/identity key. So `memberOf` resolves case-insensitively
		// but still renders as `memberOf`.
		d.Canonical = strings.TrimSpace(d.Canonical)
		d.Reciprocal = canonKey(d.Reciprocal)
		v.byCanonical[canon] = d
	}

	// Backfill reciprocals so the relationship is symmetric in the table:
	// if A.reciprocal = B and B doesn't name its own reciprocal, set
	// B.reciprocal = A. Create a bare B entry if config never defined one.
	for _, d := range defs {
		canon := canonKey(d.Canonical)
		recip := canonKey(d.Reciprocal)
		if canon == "" || recip == "" || recip == canon {
			continue
		}
		other, ok := v.byCanonical[recip]
		if !ok {
			other = RelationDef{Canonical: recip}
		}
		if other.Reciprocal == "" {
			other.Reciprocal = canon
			v.byCanonical[recip] = other
		}
	}

	// Index canonical names and aliases. Canonical names win over aliases on
	// collision so a relation is always reachable by its own name.
	for canon, d := range v.byCanonical {
		for _, a := range d.Aliases {
			key := canonKey(a)
			if key == "" {
				continue
			}
			if _, isCanon := v.byCanonical[key]; isCanon {
				continue
			}
			v.aliasIndex[key] = canon
		}
	}
	for canon := range v.byCanonical {
		v.aliasIndex[canon] = canon
	}
	return v
}

// Resolve maps a surface label to its canonical relation. known is false for a
// label absent from the vocabulary — a generic relation — in which case
// canonical is the lowercased label itself and the relation has no reciprocal.
func (v *RelationVocab) Resolve(label string) (canonical string, known bool) {
	key := canonKey(label)
	if key == "" {
		return "", false
	}
	if canon, ok := v.aliasIndex[key]; ok {
		return canon, true
	}
	return key, false
}

// Reciprocal returns the canonical reciprocal of a canonical relation, or ""
// if the relation is unknown or has no defined reverse. A symmetric relation
// returns itself.
func (v *RelationVocab) Reciprocal(canonical string) string {
	if d, ok := v.byCanonical[canonKey(canonical)]; ok {
		return d.Reciprocal
	}
	return ""
}

// Plural returns the display plural for a canonical relation: the configured
// plural when set, otherwise the display name with a trailing "s".
func (v *RelationVocab) Plural(canonical string) string {
	key := canonKey(canonical)
	if d, ok := v.byCanonical[key]; ok {
		if d.Plural != "" {
			return d.Plural
		}
		if d.Canonical != "" {
			return pluralise(d.Canonical)
		}
	}
	return pluralise(key)
}

// Display returns the canonical relation's name in its original casing, for
// rendering. Falls back to the lowercased key for unknown relations.
func (v *RelationVocab) Display(canonical string) string {
	if d, ok := v.byCanonical[canonKey(canonical)]; ok && d.Canonical != "" {
		return d.Canonical
	}
	return canonKey(canonical)
}

// Labels returns every relation label in the vocabulary — canonical names and
// their aliases, in display casing — sorted and de-duplicated. Used to offer
// relation labels as completions in the directive label slot.
func (v *RelationVocab) Labels() []string {
	seen := make(map[string]bool)
	var out []string
	add := func(label string) {
		key := canonKey(label)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, label)
	}
	for _, d := range v.byCanonical {
		add(d.Canonical)
		for _, a := range d.Aliases {
			add(a)
		}
	}
	sort.Strings(out)
	return out
}

// Known reports whether canonical names a defined relation.
func (v *RelationVocab) Known(canonical string) bool {
	_, ok := v.byCanonical[canonKey(canonical)]
	return ok
}

// canonKey normalises a relation name or alias to its lookup key: trimmed and
// lowercased. Relation labels are vocabulary, matched case-insensitively.
func canonKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// pluralise applies regular English pluralisation: a sibilant ending (s, x, z,
// ch, sh) takes "es"; a consonant followed by "y" becomes "ies"; everything
// else takes "s". Irregulars (child → children) aren't covered — those use a
// relation's configured Plural. Safe for canonical relation names, which are
// always proper singulars.
func pluralise(w string) string {
	lw := strings.ToLower(w)
	switch {
	case strings.HasSuffix(lw, "s"), strings.HasSuffix(lw, "x"), strings.HasSuffix(lw, "z"),
		strings.HasSuffix(lw, "ch"), strings.HasSuffix(lw, "sh"):
		return w + "es"
	case len(lw) >= 2 && strings.HasSuffix(lw, "y") && isConsonant(lw[len(lw)-2]):
		return w[:len(w)-1] + "ies"
	default:
		return w + "s"
	}
}

func isConsonant(b byte) bool {
	if b < 'a' || b > 'z' {
		return false
	}
	switch b {
	case 'a', 'e', 'i', 'o', 'u':
		return false
	}
	return true
}

// relationDefsFromConfig converts the lore.toml [relations.*] table into
// RelationDefs, suitable for overlaying onto BuiltinRelations.
func relationDefsFromConfig(cfg map[string]RelationConfig) []RelationDef {
	if len(cfg) == 0 {
		return nil
	}
	defs := make([]RelationDef, 0, len(cfg))
	for name, rc := range cfg {
		defs = append(defs, RelationDef{
			Canonical:  name,
			Reciprocal: rc.Reciprocal,
			Plural:     rc.Plural,
			Aliases:    rc.Aliases,
		})
	}
	return defs
}

// EffectiveRelationDefs returns the project's full relation definition set:
// built-ins first, then the [relations.*] entries overlaid on top. This is the
// set both the vocabulary and the validator operate on.
func EffectiveRelationDefs(cfg Config) []RelationDef {
	return append(BuiltinRelations(), relationDefsFromConfig(cfg.Relations)...)
}

// VocabFromConfig builds the effective relation vocabulary for a project:
// built-ins first, then the project's [relations.*] entries overlaid on top.
func VocabFromConfig(cfg Config) *RelationVocab {
	return NewRelationVocab(EffectiveRelationDefs(cfg))
}
