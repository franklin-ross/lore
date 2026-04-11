package lore

import "testing"

func TestResolveStateTagsAdd(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpAdd, Target: "injured"},
	}
	tags, _, issues := ResolveState(events)
	if len(issues) != 0 {
		t.Fatalf("issues: %+v", issues)
	}
	if !tags["injured"] {
		t.Fatalf("tag not set: %+v", tags)
	}
}

func TestResolveStateTagsRemove(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpAdd, Target: "injured"},
		{Op: StateOpRemove, Target: "injured"},
	}
	tags, _, issues := ResolveState(events)
	if len(issues) != 0 {
		t.Fatalf("issues: %+v", issues)
	}
	if tags["injured"] {
		t.Fatal("tag still set after remove")
	}
}

func TestResolveStateTagsRemoveMissingDiagnostic(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpRemove, Target: "injured", Span: StateSpan{File: "t.md", Line: 1}},
	}
	_, _, issues := ResolveState(events)
	if len(issues) != 1 {
		t.Fatalf("issues: %+v", issues)
	}
	if issues[0].Severity != SeverityWarning {
		t.Fatalf("severity: %v", issues[0].Severity)
	}
}

func TestResolveStateTagsIdempotentAdd(t *testing.T) {
	// Adding a tag twice is fine, no diagnostic.
	events := []StateEvent{
		{Op: StateOpAdd, Target: "injured"},
		{Op: StateOpAdd, Target: "injured"},
	}
	tags, _, issues := ResolveState(events)
	if len(issues) != 0 {
		t.Fatalf("issues: %+v", issues)
	}
	if !tags["injured"] {
		t.Fatal("tag not set")
	}
}

func TestResolveStateNumericSet(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpSet, Target: "population", Value: &FieldValue{Kind: FieldNumeric, Number: 100}},
	}
	_, fields, issues := ResolveState(events)
	if len(issues) != 0 {
		t.Fatalf("issues: %+v", issues)
	}
	f := fields["population"]
	if f.Kind != FieldNumeric || f.Number != 100 {
		t.Fatalf("field: %+v", f)
	}
}

func TestResolveStateNumericIncrement(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpSet, Target: "population", Value: &FieldValue{Kind: FieldNumeric, Number: 100}},
		{Op: StateOpIncrement, Target: "population", Value: &FieldValue{Kind: FieldNumeric, Number: 50}},
	}
	_, fields, issues := ResolveState(events)
	if len(issues) != 0 {
		t.Fatalf("issues: %+v", issues)
	}
	if fields["population"].Number != 150 {
		t.Fatalf("got %v", fields["population"].Number)
	}
}

func TestResolveStateNumericDecrement(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpSet, Target: "population", Value: &FieldValue{Kind: FieldNumeric, Number: 100}},
		{Op: StateOpRemove, Target: "population", Value: &FieldValue{Kind: FieldNumeric, Number: 25}},
	}
	_, fields, _ := ResolveState(events)
	if fields["population"].Number != 75 {
		t.Fatalf("got %v", fields["population"].Number)
	}
}

func TestResolveStateNumericUninitialisedDecrement(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpRemove, Target: "population", Value: &FieldValue{Kind: FieldNumeric, Number: 10}},
	}
	_, _, issues := ResolveState(events)
	if len(issues) != 1 || issues[0].Severity != SeverityWarning {
		t.Fatalf("issues: %+v", issues)
	}
}

func TestResolveStateKindConflictNumericThenText(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpSet, Target: "x", Value: &FieldValue{Kind: FieldNumeric, Number: 100}},
		{Op: StateOpIncrement, Target: "x", Value: &FieldValue{Kind: FieldText, Text: []string{"sword"}}},
	}
	_, fields, issues := ResolveState(events)
	if len(issues) != 1 {
		t.Fatalf("issues: %+v", issues)
	}
	// Field stays numeric (the conflicting op is rejected).
	if fields["x"].Kind != FieldNumeric {
		t.Fatalf("field: %+v", fields["x"])
	}
}
