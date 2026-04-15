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

func TestResolveStateTextSet(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpSet, Target: "inventory", Value: &FieldValue{Kind: FieldText, Text: []string{"sword", "shield"}}},
	}
	_, fields, issues := ResolveState(events)
	if len(issues) != 0 {
		t.Fatalf("issues: %+v", issues)
	}
	if len(fields["inventory"].Text) != 2 {
		t.Fatalf("items: %+v", fields["inventory"])
	}
}

func TestResolveStateTextAppend(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpSet, Target: "inventory", Value: &FieldValue{Kind: FieldText, Text: []string{"sword"}}},
		{Op: StateOpIncrement, Target: "inventory", Value: &FieldValue{Kind: FieldText, Text: []string{"shield"}}},
	}
	_, fields, _ := ResolveState(events)
	got := fields["inventory"].Text
	if len(got) != 2 || got[0] != "sword" || got[1] != "shield" {
		t.Fatalf("items: %+v", got)
	}
}

func TestResolveStateTextRemove(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpSet, Target: "inventory", Value: &FieldValue{Kind: FieldText, Text: []string{"sword", "shield"}}},
		{Op: StateOpRemove, Target: "inventory", Value: &FieldValue{Kind: FieldText, Text: []string{"sword"}}},
	}
	_, fields, issues := ResolveState(events)
	if len(issues) != 0 {
		t.Fatalf("issues: %+v", issues)
	}
	got := fields["inventory"].Text
	if len(got) != 1 || got[0] != "shield" {
		t.Fatalf("items: %+v", got)
	}
}

func TestResolveStateTextRemoveMissingItemDiagnostic(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpSet, Target: "inventory", Value: &FieldValue{Kind: FieldText, Text: []string{"sword"}}},
		{Op: StateOpRemove, Target: "inventory", Value: &FieldValue{Kind: FieldText, Text: []string{"shield"}}},
	}
	_, _, issues := ResolveState(events)
	if len(issues) != 1 || issues[0].Severity != SeverityWarning {
		t.Fatalf("issues: %+v", issues)
	}
}

func TestResolveStateTextRemoveEmptiesField(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpSet, Target: "inventory", Value: &FieldValue{Kind: FieldText, Text: []string{"sword"}}},
		{Op: StateOpRemove, Target: "inventory", Value: &FieldValue{Kind: FieldText, Text: []string{"sword"}}},
	}
	_, fields, _ := ResolveState(events)
	if _, ok := fields["inventory"]; ok {
		t.Fatalf("expected field to be deleted, got %+v", fields)
	}
}

func TestResolveStateTextKindConflict(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpSet, Target: "x", Value: &FieldValue{Kind: FieldText, Text: []string{"alive"}}},
		{Op: StateOpSet, Target: "x", Value: &FieldValue{Kind: FieldNumeric, Number: 42}},
	}
	_, fields, issues := ResolveState(events)
	if len(issues) != 1 {
		t.Fatalf("issues: %+v", issues)
	}
	if fields["x"].Kind != FieldText {
		t.Fatalf("field: %+v", fields["x"])
	}
}

func TestResolveStateAtSameFileCutoff(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpAdd, Target: "injured", Span: StateSpan{File: "a.md", Line: 5}},
		{Op: StateOpAdd, Target: "cursed", Span: StateSpan{File: "a.md", Line: 10}},
	}
	tags, _, _ := ResolveStateAt(events, "a.md", 7)
	if !tags["injured"] || tags["cursed"] {
		t.Fatalf("tags: %+v", tags)
	}
}

func TestResolveStateAtInclusive(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpAdd, Target: "x", Span: StateSpan{File: "a.md", Line: 5}},
	}
	tags, _, _ := ResolveStateAt(events, "a.md", 5)
	if !tags["x"] {
		t.Fatalf("cursor on directive line should include event; tags: %+v", tags)
	}
}

func TestResolveStateAtEarlierFileIncluded(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpAdd, Target: "early", Span: StateSpan{File: "a.md", Line: 100}},
		{Op: StateOpAdd, Target: "later", Span: StateSpan{File: "b.md", Line: 10}},
	}
	tags, _, _ := ResolveStateAt(events, "b.md", 5)
	if !tags["early"] || tags["later"] {
		t.Fatalf("tags: %+v", tags)
	}
}

func TestResolveStateAtEmptyCursorFileFoldsAll(t *testing.T) {
	events := []StateEvent{
		{Op: StateOpAdd, Target: "a", Span: StateSpan{File: "a.md", Line: 1}},
		{Op: StateOpAdd, Target: "b", Span: StateSpan{File: "b.md", Line: 1}},
	}
	tags, _, _ := ResolveStateAt(events, "", 0)
	if !tags["a"] || !tags["b"] {
		t.Fatalf("tags: %+v", tags)
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
