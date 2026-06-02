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
	v := NewRelationVocab(defs)
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
