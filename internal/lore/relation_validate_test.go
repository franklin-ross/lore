package lore

import (
	"strings"
	"testing"
)

func TestValidateBuiltinsAreClean(t *testing.T) {
	if issues := ValidateRelations(BuiltinRelations()); len(issues) != 0 {
		t.Fatalf("built-in relations should validate cleanly, got: %+v", issues)
	}
}

// Every built-in canonical must pluralise to a sane header — no doubling
// ("contentses") and no compound mangling ("member-ofs"). The library handles
// already-plural forms; the lexicon covers what it can't.
func TestBuiltinCanonicalsPluraliseSanely(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	for _, d := range BuiltinRelations() {
		c := d.Canonical
		got := v.Plural(c)
		// Doubling an already-s-ending canonical ("contents" -> "contentses").
		if strings.HasSuffix(strings.ToLower(c), "s") && strings.EqualFold(got, c+"es") {
			t.Errorf("Plural(%q) = %q doubles the s-ending; pin it in BuiltinPlurals", c, got)
		}
		// Appending to the trailing preposition of a compound ("member-ofs").
		if strings.Contains(c, "-of") && strings.EqualFold(got, c+"s") {
			t.Errorf("Plural(%q) = %q inflects the preposition; pin it in BuiltinPlurals", c, got)
		}
	}
}

// No label (canonical or alias) may appear on two canonicals. The vocab's
// alias index is last-write-wins and validation doesn't catch it, so a
// duplicate would silently bind a word to the wrong relation. Guard the
// built-ins against that.
func TestBuiltinsHaveNoDuplicateLabels(t *testing.T) {
	owner := map[string]string{}
	for _, d := range BuiltinRelations() {
		for _, label := range append([]string{d.Canonical}, d.Aliases...) {
			key := strings.ToLower(label)
			if prev, ok := owner[key]; ok {
				t.Errorf("label %q claimed by both %q and %q", label, prev, d.Canonical)
			}
			owner[key] = d.Canonical
		}
	}
}

func TestValidateManyToOneReciprocal(t *testing.T) {
	defs := []RelationDef{
		{Canonical: "aunt", Reciprocal: "nibling"},
		{Canonical: "uncle", Reciprocal: "nibling"},
	}
	issues := ValidateRelations(defs)
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d: %+v", len(issues), issues)
	}
	msg := issues[0].Message
	if !strings.Contains(msg, `"aunt"`) || !strings.Contains(msg, `"uncle"`) || !strings.Contains(msg, `"nibling"`) {
		t.Fatalf("message missing names: %q", msg)
	}
}

func TestValidateManyToOneAgainstBuiltin(t *testing.T) {
	// A config relation that reciprocates a built-in's reciprocal collides.
	cfg := Config{Relations: map[string]RelationConfig{
		"auntie": {Reciprocal: "nibling"}, // built-in pibling already reciprocates nibling
	}}
	issues := ValidateRelations(EffectiveRelationDefs(cfg))
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d: %+v", len(issues), issues)
	}
}

func TestValidateNonMutualReciprocal(t *testing.T) {
	defs := []RelationDef{
		{Canonical: "parent", Reciprocal: "child"},
		{Canonical: "child", Reciprocal: "sibling"},
		{Canonical: "sibling", Reciprocal: "sibling"},
	}
	issues := ValidateRelations(defs)
	if len(issues) == 0 {
		t.Fatal("expected a non-mutual reciprocal issue")
	}
}

func TestValidateMutualBothSidesIsClean(t *testing.T) {
	// Declaring the reciprocal on both sides is redundant but must not trip
	// the non-mutual check when the two agree.
	defs := []RelationDef{
		{Canonical: "parent", Reciprocal: "child"},
		{Canonical: "child", Reciprocal: "parent"},
	}
	if issues := ValidateRelations(defs); len(issues) != 0 {
		t.Fatalf("matching both-sides reciprocal should be clean, got: %+v", issues)
	}
	v := NewRelationVocab(defs, nil)
	if v.Reciprocal("parent") != "child" || v.Reciprocal("child") != "parent" {
		t.Fatalf("reciprocals should round-trip: %q / %q", v.Reciprocal("parent"), v.Reciprocal("child"))
	}
}

func TestValidateSymmetricIsFine(t *testing.T) {
	defs := []RelationDef{
		{Canonical: "spouse", Reciprocal: "spouse"},
		{Canonical: "friend", Reciprocal: "friend"},
	}
	if issues := ValidateRelations(defs); len(issues) != 0 {
		t.Fatalf("symmetric relations should be clean, got: %+v", issues)
	}
}
