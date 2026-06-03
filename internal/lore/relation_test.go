package lore

import (
	"slices"
	"testing"
)

func TestVocabResolvesAliasToCanonical(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	canon, known := v.Resolve("father")
	if !known || canon != "parent" {
		t.Fatalf("father -> %q known=%v; want parent true", canon, known)
	}
}

func TestVocabResolvesCanonicalByName(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	canon, known := v.Resolve("Parent")
	if !known || canon != "parent" {
		t.Fatalf("Parent -> %q known=%v; want parent true", canon, known)
	}
}

func TestVocabUnknownLabelIsGeneric(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	canon, known := v.Resolve("bestie")
	if known {
		t.Fatalf("bestie should be unknown, got canon %q", canon)
	}
	if canon != "bestie" {
		t.Fatalf("generic canonical = %q; want bestie", canon)
	}
	if got := v.Reciprocal("bestie"); got != "" {
		t.Fatalf("generic reciprocal = %q; want empty", got)
	}
}

func TestVocabReciprocalBidirectional(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	if got := v.Reciprocal("parent"); got != "child" {
		t.Fatalf("parent reciprocal = %q; want child", got)
	}
	// child never declares a reciprocal; it should be backfilled to parent.
	if got := v.Reciprocal("child"); got != "parent" {
		t.Fatalf("child reciprocal = %q; want parent (backfilled)", got)
	}
}

func TestVocabSymmetricReciprocalIsSelf(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	if got := v.Reciprocal("spouse"); got != "spouse" {
		t.Fatalf("spouse reciprocal = %q; want spouse", got)
	}
}

func TestVocabPluralDefaultAndConfigured(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	if got := v.Plural("child"); got != "children" {
		t.Fatalf("child plural = %q; want children", got)
	}
	if got := v.Plural("friend"); got != "friends" {
		t.Fatalf("friend plural = %q; want friends (default +s)", got)
	}
}

func TestVocabLabels(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	labels := v.Labels()
	// Completion offers canonicals and true-synonym aliases.
	for _, w := range []string{"father", "parent", "member", "spouse", "aunt"} {
		if !slices.Contains(labels, w) {
			t.Errorf("Labels() missing %q; got %v", w, labels)
		}
	}
	// Plurals resolve as input but aren't separate suggestions — the singular
	// canonical stands in for them, like case-insensitivity.
	if slices.Contains(labels, "members") {
		t.Errorf("Labels() should not list the plural %q", "members")
	}
	if canon, known := v.Resolve("members"); !known || canon != "member" {
		t.Errorf("Resolve(members) = %q,%v; want member,true (still resolves)", canon, known)
	}
}

func TestVocabGenderVariantsAreAliasesOfOneCanonical(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	for label, want := range map[string]string{
		"aunt": "pibling", "uncle": "pibling",
		"niece": "nibling", "nephew": "nibling",
	} {
		if canon, known := v.Resolve(label); !known || canon != want {
			t.Fatalf("%s -> %q known=%v; want %s", label, canon, known, want)
		}
	}
	// The shared reciprocal round-trips because there is one canonical, not two.
	if v.Reciprocal("pibling") != "nibling" || v.Reciprocal("nibling") != "pibling" {
		t.Fatalf("pibling/nibling reciprocals don't round-trip: %q / %q",
			v.Reciprocal("pibling"), v.Reciprocal("nibling"))
	}
}

func TestVocabGenitiveSynthesisedFromReciprocal(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	// Genitives are no longer listed by hand — they're derived from a noun plus
	// the reciprocal, so the direction can't be inverted.
	for label, want := range map[string]string{
		"daughter-of": "parent", // daughter is a noun of child; reciprocal parent
		"son-of":      "parent",
		"father-of":   "child",   // father is a noun of parent; reciprocal child
		"aunt-of":     "nibling", // aunt is a noun of pibling; reciprocal nibling
		"niece-of":    "pibling",
		"grandson-of": "grandparent",
		"owner-of":    "possession",
		"member-of":   "member-of", // canonical in its own right, not overridden
	} {
		if canon, known := v.Resolve(label); !known || canon != want {
			t.Errorf("%s -> %q known=%v; want %s", label, canon, known, want)
		}
	}
}

func TestVocabRawAliasesResolveButGetNoGenitive(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	// Raw aliases resolve like aliases — they're how an author types the edge.
	for label, want := range map[string]string{
		"owns": "possession", "leads": "follower", "serves": "leader",
		"contains": "contents", "holds": "contents", "forged": "creation",
	} {
		if canon, known := v.Resolve(label); !known || canon != want {
			t.Errorf("verb %s -> %q known=%v; want %s", label, canon, known, want)
		}
	}
	// "of" is a noun genitive — verbs never get one, and a noun already ending
	// in "-of" isn't double-suffixed.
	for _, junk := range []string{"owns-of", "leads-of", "contains-of", "member-of-of"} {
		if canon, known := v.Resolve(junk); known {
			t.Errorf("%s should not resolve, got %q", junk, canon)
		}
	}
}

// within/inside are raw aliases (locatives): "A is within B" already names the
// subject, so no genitive is synthesised — crucially not the inverted
// `within-of -> contents`. `inside-of` is real usage and listed as a raw
// synonym, so it resolves; `within-of` isn't listed and stays a generic edge.
func TestVocabLocativesAreRawAliasesNotInverted(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	for _, label := range []string{"within", "inside", "inside-of"} {
		if canon, known := v.Resolve(label); !known || canon != "container" {
			t.Errorf("%s -> %q known=%v; want container (not the inverse contents)", label, canon, known)
		}
	}
	// No synthesised genitive: `within-of` would have inverted to contents.
	if canon, known := v.Resolve("within-of"); known {
		t.Errorf("within-of should be a generic edge, got %q", canon)
	}
}

func TestVocabLabelsIncludeRawAliasesAndGenitives(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	labels := v.Labels()
	for _, w := range []string{"owns", "daughter-of", "owner-of"} {
		if !slices.Contains(labels, w) {
			t.Errorf("Labels() missing %q", w)
		}
	}
	// No verb genitive should ever be offered as a completion.
	if slices.Contains(labels, "owns-of") {
		t.Errorf("Labels() should not list verb genitive %q", "owns-of")
	}
}

func TestRelationAuntUncleConvergeToOneEdge(t *testing.T) {
	// uncle (declared) and its undeclared nibling reverse must be one edge,
	// each side keeping its own surface label.
	world := setupTestWorld(t, "Bob (person): uncle -> Tom\n\nTom (person): A man.\n")
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())

	bob, _ := world.FindEntity("Bob")
	bobRel := FormatRelationsBlock(world.ResolveRelations(v, bob), v)
	if bobRel != "uncle → Tom" {
		t.Fatalf("Bob relations = %q; want \"uncle → Tom\"", bobRel)
	}

	tom, _ := world.FindEntity("Tom")
	tomRel := FormatRelationsBlock(world.ResolveRelations(v, tom), v)
	if tomRel != "nibling → Bob" {
		t.Fatalf("Tom relations = %q; want neutral \"nibling → Bob\"", tomRel)
	}
}

// pluralise() delegates to go-pluralize; this guards the integration on the
// words our domain actually cares about — regulars, the fantasy `-f`→`-ves`
// set, and the classic irregulars the old naive rules got wrong.
func TestPluraliseHandlesDomainWords(t *testing.T) {
	cases := map[string]string{
		"parent": "parents", "ally": "allies", "boss": "bosses", // regulars
		"child": "children", "wife": "wives", "person": "people", // irregulars
		"dwarf": "dwarves", "elf": "elves", "wolf": "wolves", "thief": "thieves", // fantasy -f→-ves
		"contents": "contents", "belongings": "belongings", // already plural, untouched
	}
	for in, want := range cases {
		if got := pluralise(in); got != want {
			t.Errorf("pluralise(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestVocabPluralUsesRulesAndConfigWins(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	// Rule-derived plurals (no explicit config any more).
	if got := v.Plural("ally"); got != "allies" {
		t.Fatalf("Plural(ally) = %q; want allies", got)
	}
	if got := v.Plural("enemy"); got != "enemies" {
		t.Fatalf("Plural(enemy) = %q; want enemies", got)
	}
	// The library handles irregulars without a lexicon entry.
	if got := v.Plural("child"); got != "children" {
		t.Fatalf("Plural(child) = %q; want children", got)
	}
	// The lexicon override wins where the library can't infer it: a `-of`
	// compound pluralises the head noun, not the trailing preposition.
	if got := v.Plural("member-of"); got != "members-of" {
		t.Fatalf("Plural(member-of) = %q; want members-of", got)
	}
}

func TestVocabConfigOverlaysBuiltins(t *testing.T) {
	cfg := Config{Relations: map[string]RelationConfig{
		"mentor": {Reciprocal: "student", Aliases: []string{"teacher"}},
	}}
	v := VocabFromConfig(cfg)

	// Built-ins still present.
	if canon, known := v.Resolve("father"); !known || canon != "parent" {
		t.Fatalf("builtin father -> %q known=%v", canon, known)
	}
	// Config relation resolves, alias included.
	if canon, known := v.Resolve("teacher"); !known || canon != "mentor" {
		t.Fatalf("teacher -> %q known=%v; want mentor true", canon, known)
	}
	// Reciprocal backfilled for the config relation.
	if got := v.Reciprocal("student"); got != "mentor" {
		t.Fatalf("student reciprocal = %q; want mentor", got)
	}
}

func TestVocabConfigOverridesBuiltin(t *testing.T) {
	cfg := Config{Plurals: map[string]string{"spouse": "spice"}}
	v := VocabFromConfig(cfg)
	if got := v.Plural("spouse"); got != "spice" {
		t.Fatalf("overridden spouse plural = %q; want spice", got)
	}
}
