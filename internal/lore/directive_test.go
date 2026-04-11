package lore

import (
	"reflect"
	"testing"
)

func TestParseDirectivesPlainTag(t *testing.T) {
	events, issues := ParseDirectives("Took an arrow. +injured He's in bad shape.", "test.md", 3)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Op != StateOpAdd || events[0].Target != "injured" {
		t.Fatalf("event = %+v", events[0])
	}
}

func TestParseDirectivesRemoveTag(t *testing.T) {
	events, issues := ParseDirectives("Patched up. -injured", "test.md", 1)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	if len(events) != 1 || events[0].Op != StateOpRemove || events[0].Target != "injured" {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseDirectivesHyphenatedTag(t *testing.T) {
	events, _ := ParseDirectives("+critically-injured", "test.md", 1)
	if len(events) != 1 || events[0].Target != "critically-injured" {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseDirectivesQuotedTagEscape(t *testing.T) {
	events, _ := ParseDirectives(`+"critically injured"`, "test.md", 1)
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	// Quoted tag is normalised by replacing spaces with hyphens.
	if events[0].Target != "critically-injured" {
		t.Fatalf("target = %q", events[0].Target)
	}
}

func TestParseDirectivesUnicodeTag(t *testing.T) {
	events, _ := ParseDirectives("+呪われた", "test.md", 1)
	if len(events) != 1 || events[0].Target != "呪われた" {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseDirectivesMultipleTags(t *testing.T) {
	events, _ := ParseDirectives("+injured +bleeding He stumbled", "test.md", 1)
	if len(events) != 2 {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Target != "injured" || events[1].Target != "bleeding" {
		t.Fatalf("targets = %+v", events)
	}
}

func TestParseDirectivesSpanTracking(t *testing.T) {
	events, _ := ParseDirectives("He took one. +injured indeed.", "test.md", 7)
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	ev := events[0]
	if ev.Span.File != "test.md" || ev.Span.Line != 7 {
		t.Fatalf("span = %+v", ev.Span)
	}
	if ev.Span.StartByte != 13 {
		// "He took one. " is 13 bytes, directive starts at 13
		t.Fatalf("start = %d", ev.Span.StartByte)
	}
	if ev.Span.EndByte != 21 {
		// "+injured" is 8 bytes, ends at 21
		t.Fatalf("end = %d", ev.Span.EndByte)
	}
}

func TestParseDirectivesNoDirectivesInPlainProse(t *testing.T) {
	events, issues := ParseDirectives("Just plain prose with nothing special.", "test.md", 1)
	if len(events) != 0 {
		t.Fatalf("events = %+v", events)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestParseDirectivesIgnoresStrayPlus(t *testing.T) {
	// A '+' not followed by an identifier is prose.
	events, _ := ParseDirectives("He had a + sign on the door.", "test.md", 1)
	if len(events) != 0 {
		t.Fatalf("events = %+v", reflect.ValueOf(events))
	}
}
