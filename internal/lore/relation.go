package lore

import (
	"sort"
	"strings"

	"github.com/gertd/go-pluralize"
)

// pluralizer holds the English inflection rules. Built once — the client is
// read-only after construction, so it's safe for concurrent Plural calls.
var pluralizer = pluralize.NewClient()

// RelationDef defines a relation type: its canonical name, the canonical name
// of its reciprocal (the label shown on the far endpoint), the noun aliases
// that normalise to it, and raw aliases taken as-is (verbs, locatives).
//
// Reciprocal is the canonical name of the reverse relation. A relation whose
// reciprocal is itself (Canonical == Reciprocal) is symmetric — `spouse`,
// `sibling`, `friend`. An empty Reciprocal means the relation has no defined
// reverse, so the far side renders as a named-incoming edge.
//
// Canonical, Reciprocal, and Aliases are all nouns: they name *what the target
// is to the subject* (`A: <noun> -> B` = "B is A's <noun>"). From a noun the
// vocab synthesises the genitive `<noun>-of`, which names *what the subject is
// to the target* and resolves to the reciprocal — `daughter-of` resolves to
// `parent`. Deriving the genitive from the reciprocal makes its direction
// impossible to invert by hand.
//
// RawAliases holds surface labels taken as-is: they resolve to the canonical
// but aren't processed into derived forms the way Aliases are — no synthesised
// genitive, no plural transform. The name states the contract (raw, not further
// interpreted) rather than a part of speech, so it fits both kinds that belong
// here, each already carrying its own direction such that a genitive would be
// wrong: verbs (`A: owns -> B` = "A owns B") and locatives (`A: within -> B` =
// "A is within B"). A verb lives on the def whose subject is the actor — `owns`
// on `possession`, `leads` on `follower`.
type RelationDef struct {
	Canonical  string
	Reciprocal string
	Aliases    []string
	RawAliases []string
}

// nouns returns a def's noun surface forms — its canonical followed by its
// aliases, in display casing. RawAliases are excluded: the genitive and plural
// transforms apply only to nouns.
func (d RelationDef) nouns() []string {
	out := make([]string, 0, 1+len(d.Aliases))
	if d.Canonical != "" {
		out = append(out, d.Canonical)
	}
	return append(out, d.Aliases...)
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
	pluralIndex map[string]string      // lowercased singular surface -> display plural
	rawAliases  map[string]bool        // lowercased raw-alias surfaces (never pluralised)
}

// BuiltinRelations is the default vocabulary shipped for common familial,
// social, and membership relations. Users extend or override these by defining
// a relation of the same canonical name in lore.toml.
func BuiltinRelations() []RelationDef {
	return []RelationDef{
		// Nouns only in Aliases. The genitive forms (`daughter-of`, `son-of`,
		// `parent-of`, …) are synthesised in NewRelationVocab from each noun plus
		// the reciprocal, so they never need listing — and can't be inverted by
		// hand the way a typed `daughter-of` on the wrong def could.
		{Canonical: "parent", Reciprocal: "child", Aliases: []string{"father", "mother"}},
		{Canonical: "child", Aliases: []string{"son", "daughter"}},
		{Canonical: "step-parent", Reciprocal: "step-child", Aliases: []string{"step-father", "step-mother"}},
		{Canonical: "step-child", Aliases: []string{"step-son", "step-daughter"}},
		// Step- vs half- split on blood, not the prefix. Step-relations are
		// non-blood and get their own canonicals (step-parent↔step-child) — a
		// step-parent is not your parent, and the status drives plot. Half- is a
		// degree on an existing blood tie: a half-sibling *is* your sibling (one
		// shared parent), so it folds into `sibling`. Half- has no parent/child
		// form to cluster anyway — you can't be half someone's parent.
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
		{Canonical: "grandparent", Reciprocal: "grandchild", Aliases: []string{"grandmother", "grandfather"}},
		{Canonical: "grandchild", Aliases: []string{"grandson", "granddaughter"}},
		{Canonical: "spouse", Reciprocal: "spouse", Aliases: []string{"husband", "wife", "partner", "betrothed", "lover", "concubine", "mistress"}},
		{Canonical: "member", Reciprocal: "member-of"},
		{Canonical: "member-of"},
		// Social allegiance — bread and butter of campaign notes. All symmetric.
		// `friend` and `ally` overlap deliberately (warmth vs alignment); pick
		// whichever fits the prose. Plurals (members/allies/friends/enemies)
		// resolve automatically, so they aren't listed as aliases.
		{Canonical: "ally", Reciprocal: "ally"},
		{Canonical: "friend", Reciprocal: "friend"},
		{Canonical: "enemy", Reciprocal: "enemy", Aliases: []string{"rival", "nemesis"}},
		// Containment is generic — boxes, chests, ships, regions all "contain".
		// Canonicals are nouns (they key the edge, label the undeclared reverse
		// side, and feed the pluraliser, which assumes a proper singular): the
		// holder's side is `contents`, the held side `container`. `contains`/
		// `holds` are raw aliases (verbs) on `contents` — were they canonical,
		// `pluralise` would mangle them ("containses"). `contents` already reads
		// plural, so its plural is pinned to itself. `contents`/`container` sit on
		// opposite endpoints — a reciprocal pair, not aliases of one canonical.
		//
		// `within`/`inside` are locatives: `A: within -> B` already reads "A is
		// within B", so they're raw aliases of `container`, not nouns — no
		// genitive is synthesised for them (which would wrongly point at
		// `contents`). `inside-of` is real usage, so it's listed too; raw, it's
		// taken as the synonym it is rather than processed to invert subject/target.
		{Canonical: "contents", Reciprocal: "container", RawAliases: []string{"contains", "holds"}},
		{Canonical: "container", RawAliases: []string{"within", "inside", "inside-of"}},
		{Canonical: "residence", Reciprocal: "resident", Aliases: []string{"home", "abode", "dwelling"}},
		{Canonical: "resident", Aliases: []string{"tenant", "inhabitant", "occupant"}},
		// `owns` (subject is the owner) is a raw alias on `possession`, since
		// `A: possession -> B` already reads "A owns B".
		{Canonical: "possession", Reciprocal: "owner", Aliases: []string{"belongings", "property"}, RawAliases: []string{"owns"}},
		{Canonical: "owner", Aliases: []string{"proprietor", "holder"}},
		// `leader` is a noun: `A: leader -> B` = "B is A's leader", so A is the
		// follower. The verbs `serves`/`follows` (subject = follower) therefore
		// sit on `leader`; `leads` (subject = leader) on `follower`.
		{Canonical: "leader", Reciprocal: "follower", Aliases: []string{"chief", "boss"}, RawAliases: []string{"serves", "follows"}},
		{Canonical: "follower", Aliases: []string{"servant", "subordinate", "minion"}, RawAliases: []string{"leads"}},
		{Canonical: "mentor", Reciprocal: "pupil", Aliases: []string{"teacher"}},
		{Canonical: "pupil", Aliases: []string{"apprentice", "student", "disciple"}},
		// `made`/`forged` (subject = maker) are raw aliases on `creation`, since
		// `A: creation -> B` reads "B is A's creation" (A made B).
		{Canonical: "creator", Reciprocal: "creation", Aliases: []string{"maker", "author"}},
		{Canonical: "creation", RawAliases: []string{"made", "crafted", "forged", "built"}},
	}
}

// BuiltinPlurals is the irregular-plural lexicon for the default vocabulary,
// keyed by the singular surface. Pluralisation is a property of the word, not
// the relation, so these live in one flat map rather than on each RelationDef.
// pluralise() (go-pluralize) already handles every English irregular our
// vocabulary meets — `child`/`wife`/`nemesis`, the `-f`→`-ves` set, and the
// already-plural `contents`/`belongings` — so only `member-of` needs an entry:
// no English pluraliser inflects the head noun of a `-of` compound, giving
// "member-ofs" instead of "members-of". Users extend this with a top-level
// [plurals] table in lore.toml (e.g. to pin a contested word like `dwarf`).
func BuiltinPlurals() map[string]string {
	return map[string]string{
		"member-of": "members-of",
	}
}

// NewRelationVocab builds a vocabulary from the given definitions and an
// irregular-plural lexicon (surface -> display plural). Later definitions
// override earlier ones with the same canonical name, so callers pass built-ins
// first and config entries second; pass BuiltinRelations()/BuiltinPlurals() for
// the defaults, or EffectiveRelationDefs(cfg)/EffectivePlurals(cfg) for a
// project's combined set.
//
// Reciprocity is made bidirectional: defining parent.reciprocal = child also
// gives child.reciprocal = parent, unless child already declares its own
// reciprocal. The canonical name itself always resolves as a label, in
// addition to its aliases.
func NewRelationVocab(defs []RelationDef, plurals map[string]string) *RelationVocab {
	v := &RelationVocab{
		byCanonical: make(map[string]RelationDef),
		aliasIndex:  make(map[string]string),
		pluralIndex: make(map[string]string),
		rawAliases:  make(map[string]bool),
	}
	for surface, plural := range plurals {
		if key := canonKey(surface); key != "" {
			v.pluralIndex[key] = strings.TrimSpace(plural)
		}
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

	// Index canonical names, aliases, and raw aliases. Canonical names win over
	// the rest on collision so a relation is always reachable by its own name.
	// Raw aliases resolve like aliases — they're how authors type an edge — but
	// are recorded in rawAliases so the genitive/plural passes skip them.
	index := func(label, canon string) {
		key := canonKey(label)
		if key == "" {
			return
		}
		if _, isCanon := v.byCanonical[key]; isCanon {
			return
		}
		v.aliasIndex[key] = canon
	}
	for canon, d := range v.byCanonical {
		for _, a := range d.Aliases {
			index(a, canon)
		}
		for _, ra := range d.RawAliases {
			index(ra, canon)
			if key := canonKey(ra); key != "" {
				v.rawAliases[key] = true
			}
		}
	}
	for canon := range v.byCanonical {
		v.aliasIndex[canon] = canon
	}

	// Synthesise genitive "-of" labels from nouns. For a noun N on a relation
	// whose reciprocal is R, `N-of` names what the subject is to the target and
	// resolves to R: `daughter-of` comes from `daughter` (a noun of `child`,
	// reciprocal `parent`) and resolves to `parent` — Doug is Sarah's parent.
	// Deriving from the reciprocal means the direction can't be inverted by
	// hand. Raw aliases are excluded ("of" is a noun genitive); a noun already ending
	// in "-of" (e.g. `member-of`) is skipped to avoid `member-of-of`. Explicit
	// labels already indexed win, so this only fills gaps.
	for _, d := range v.byCanonical {
		if d.Reciprocal == "" {
			continue
		}
		for _, n := range d.nouns() {
			key := canonKey(n)
			if key == "" || strings.HasSuffix(key, "-of") {
				continue
			}
			ofKey := key + "-of"
			if _, taken := v.aliasIndex[ofKey]; taken {
				continue
			}
			if _, isCanon := v.byCanonical[ofKey]; isCanon {
				continue
			}
			v.aliasIndex[ofKey] = d.Reciprocal
		}
	}

	// Plurals resolve as input labels too, so you can write `Cuthbert: children
	// -> Milly, Bobby` as naturally as `child -> Milly`, and `wives`/`daughters`
	// as well as `wife`/`daughter`. Index the plural of every noun surface
	// (canonical + aliases; raw aliases are excluded by nouns()). Already-indexed
	// labels win, so this only fills gaps.
	addPluralInput := func(surface string) {
		canon, ok := v.aliasIndex[canonKey(surface)]
		if !ok {
			return
		}
		plural := pluralise(surface)
		if p, ok := v.pluralOf(surface); ok {
			plural = p
		}
		key := canonKey(plural)
		if key == "" || key == canonKey(surface) {
			return
		}
		if _, taken := v.aliasIndex[key]; taken {
			return
		}
		v.aliasIndex[key] = canon
	}
	for _, d := range v.byCanonical {
		for _, n := range d.nouns() {
			addPluralInput(n)
		}
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

// Plural returns the display plural for a canonical relation: the lexicon entry
// when one exists, otherwise the display name run through the regular rules.
func (v *RelationVocab) Plural(canonical string) string {
	key := canonKey(canonical)
	if p, ok := v.pluralIndex[key]; ok && p != "" {
		return p
	}
	if d, ok := v.byCanonical[key]; ok && d.Canonical != "" {
		return pluralise(d.Canonical)
	}
	return pluralise(key)
}

// pluralOf returns the lexicon plural for any surface (canonical or alias) and
// whether one was set. Used by the header pluraliser to honour irregulars
// before falling back to the regular rules.
func (v *RelationVocab) pluralOf(surface string) (string, bool) {
	p, ok := v.pluralIndex[canonKey(surface)]
	return p, ok && p != ""
}

// isRawAlias reports whether surface was declared as a raw alias — a verb or
// locative taken as-is. The header pluraliser leaves these untouched: they name
// the subject, not a count of things, so pluralising them is meaningless.
func (v *RelationVocab) isRawAlias(surface string) bool {
	return v.rawAliases[canonKey(surface)]
}

// Display returns the canonical relation's name in its original casing, for
// rendering. Falls back to the lowercased key for unknown relations.
func (v *RelationVocab) Display(canonical string) string {
	if d, ok := v.byCanonical[canonKey(canonical)]; ok && d.Canonical != "" {
		return d.Canonical
	}
	return canonKey(canonical)
}

// SurfaceAliases returns a canonical relation's declared surface synonyms in
// display casing — its noun aliases followed by its raw aliases — excluding the
// canonical name itself and the synthesised genitive/plural forms. Nil for an
// unknown relation.
func (v *RelationVocab) SurfaceAliases(canonical string) []string {
	d, ok := v.byCanonical[canonKey(canonical)]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(d.Aliases)+len(d.RawAliases))
	out = append(out, d.Aliases...)
	return append(out, d.RawAliases...)
}

// Labels returns every relation label in the vocabulary — canonical names,
// their aliases and raw aliases, and the synthesised genitives — in display casing,
// sorted and de-duplicated. Used to offer relation labels as completions in
// the directive label slot.
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
		for _, ra := range d.RawAliases {
			add(ra)
		}
		// Mirror the genitive synthesis in NewRelationVocab so derived `-of`
		// labels are completable too.
		if d.Reciprocal != "" {
			for _, n := range d.nouns() {
				if k := canonKey(n); k != "" && !strings.HasSuffix(k, "-of") {
					add(n + "-of")
				}
			}
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

// pluralise returns the English plural of a noun via the go-pluralize inflection
// rules, which cover the irregulars our vocabulary actually meets — `child` →
// `children`, `wife` → `wives`, the fantasy `-f` → `-ves` set (`elf`/`dwarf`/
// `wolf`/`thief`), `person` → `people` — and leave already-plural or uncountable
// forms (`contents`, `belongings`) untouched. The two things it can't infer are
// handled before it: raw aliases (verbs/locatives) never reach here, and the
// lexicon (BuiltinPlurals + [plurals]) overrides compounds like `member-of`.
func pluralise(w string) string {
	return pluralizer.Plural(w)
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
			Aliases:    rc.Aliases,
			RawAliases: rc.RawAliases,
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

// EffectivePlurals returns the project's full irregular-plural lexicon:
// built-ins overlaid with the [plurals] table from lore.toml. Mirrors
// EffectiveRelationDefs — config entries win on a shared surface.
func EffectivePlurals(cfg Config) map[string]string {
	out := BuiltinPlurals()
	for surface, plural := range cfg.Plurals {
		out[canonKey(surface)] = plural
	}
	return out
}

// VocabFromConfig builds the effective relation vocabulary for a project:
// built-ins first, then the project's [relations.*] and [plurals] entries
// overlaid on top.
func VocabFromConfig(cfg Config) *RelationVocab {
	return NewRelationVocab(EffectiveRelationDefs(cfg), EffectivePlurals(cfg))
}
