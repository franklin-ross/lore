package lore

import "testing"

func TestVocabResolvesAliasToCanonical(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations())
	canon, known := v.Resolve("father")
	if !known || canon != "parent" {
		t.Fatalf("father -> %q known=%v; want parent true", canon, known)
	}
}

func TestVocabResolvesCanonicalByName(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations())
	canon, known := v.Resolve("Parent")
	if !known || canon != "parent" {
		t.Fatalf("Parent -> %q known=%v; want parent true", canon, known)
	}
}

func TestVocabUnknownLabelIsGeneric(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations())
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
	v := NewRelationVocab(BuiltinRelations())
	if got := v.Reciprocal("parent"); got != "child" {
		t.Fatalf("parent reciprocal = %q; want child", got)
	}
	// child never declares a reciprocal; it should be backfilled to parent.
	if got := v.Reciprocal("child"); got != "parent" {
		t.Fatalf("child reciprocal = %q; want parent (backfilled)", got)
	}
}

func TestVocabSymmetricReciprocalIsSelf(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations())
	if got := v.Reciprocal("spouse"); got != "spouse" {
		t.Fatalf("spouse reciprocal = %q; want spouse", got)
	}
}

func TestVocabPluralDefaultAndConfigured(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations())
	if got := v.Plural("child"); got != "children" {
		t.Fatalf("child plural = %q; want children", got)
	}
	if got := v.Plural("friend"); got != "friends" {
		t.Fatalf("friend plural = %q; want friends (default +s)", got)
	}
}

func TestVocabGenderVariantsAreAliasesOfOneCanonical(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations())
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

func TestRelationAuntUncleConvergeToOneEdge(t *testing.T) {
	// uncle (declared) and its undeclared nibling reverse must be one edge,
	// each side keeping its own surface label.
	world := setupTestWorld(t, "Bob (person): uncle -> Tom\n\nTom (person): A man.\n")
	v := NewRelationVocab(BuiltinRelations())

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
	cfg := Config{Relations: map[string]RelationConfig{
		"spouse": {Reciprocal: "spouse", Plural: "spice"},
	}}
	v := VocabFromConfig(cfg)
	if got := v.Plural("spouse"); got != "spice" {
		t.Fatalf("overridden spouse plural = %q; want spice", got)
	}
}
