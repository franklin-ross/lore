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
