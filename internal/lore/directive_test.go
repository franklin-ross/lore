package lore

import (
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
		t.Fatalf("events = %+v", events)
	}
}

func TestParseDirectivesEmptyQuotedTagNotDirective(t *testing.T) {
	events, issues := ParseDirectives(`+"" He was whole.`, "test.md", 1)
	if len(events) != 0 {
		t.Fatalf("expected empty quoted tag to be ignored, got %+v", events)
	}
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
}

func TestParseDirectivesMidWordSigilIsProse(t *testing.T) {
	// 'foo+injured' must not parse as a +injured directive — the '+' is
	// not at a word boundary.
	events, _ := ParseDirectives("foo+injured bar", "test.md", 1)
	if len(events) != 0 {
		t.Fatalf("expected no directives, got %+v", events)
	}
}

func TestParseDirectivesSigilAtStartOfInputIsDirective(t *testing.T) {
	// Start of input is a word boundary, so a leading '+tag' parses fine.
	events, _ := ParseDirectives("+injured", "test.md", 1)
	if len(events) != 1 || events[0].Target != "injured" {
		t.Fatalf("events: %+v", events)
	}
}

func TestParseDirectivesSigilAfterPunctuationIsDirective(t *testing.T) {
	// Punctuation is a word boundary too.
	events, _ := ParseDirectives("(+injured)", "test.md", 1)
	if len(events) != 1 || events[0].Target != "injured" {
		t.Fatalf("events: %+v", events)
	}
}

func TestParseDirectivesNumericSet(t *testing.T) {
	events, issues := ParseDirectives("population = 100", "test.md", 1)
	if len(issues) != 0 {
		t.Fatalf("issues: %+v", issues)
	}
	if len(events) != 1 {
		t.Fatalf("events: %+v", events)
	}
	ev := events[0]
	if ev.Op != StateOpSet || ev.Target != "population" {
		t.Fatalf("event: %+v", ev)
	}
	if ev.Value == nil || ev.Value.Kind != FieldNumeric || ev.Value.Number != 100 {
		t.Fatalf("value: %+v", ev.Value)
	}
}

func TestParseDirectivesNumericIncrement(t *testing.T) {
	events, _ := ParseDirectives("population += 50", "test.md", 1)
	if len(events) != 1 || events[0].Op != StateOpIncrement {
		t.Fatalf("events: %+v", events)
	}
	if events[0].Value.Number != 50 {
		t.Fatalf("value: %+v", events[0].Value)
	}
}

func TestParseDirectivesNumericDecrement(t *testing.T) {
	events, _ := ParseDirectives("population -= 25", "test.md", 1)
	if len(events) != 1 || events[0].Op != StateOpRemove {
		t.Fatalf("events: %+v", events)
	}
	if events[0].Value.Number != 25 {
		t.Fatalf("value: %+v", events[0].Value)
	}
}

func TestParseDirectivesNumericDecimal(t *testing.T) {
	events, _ := ParseDirectives("weight = 3.14", "test.md", 1)
	if len(events) != 1 || events[0].Value.Number != 3.14 {
		t.Fatalf("events: %+v", events)
	}
}

func TestParseDirectivesNumericNegative(t *testing.T) {
	events, _ := ParseDirectives("temperature = -5", "test.md", 1)
	if len(events) != 1 {
		t.Fatalf("events: %+v", events)
	}
	if events[0].Value.Number != -5 {
		t.Fatalf("value: %+v", events[0].Value)
	}
}

func TestParseDirectivesNumericTrailingDotIsTerminator(t *testing.T) {
	// `population = 100.` should parse as "100" followed by a terminating ".".
	// The period is NOT part of the number (no max-munch across trailing dot).
	events, _ := ParseDirectives("population = 100. Plus more.", "test.md", 1)
	if len(events) != 1 {
		t.Fatalf("events: %+v", events)
	}
	if events[0].Value.Number != 100 {
		t.Fatalf("value: %+v", events[0].Value)
	}
}

func TestParseDirectivesFieldNameWithHyphen(t *testing.T) {
	events, _ := ParseDirectives("max-hp = 30", "test.md", 1)
	if len(events) != 1 || events[0].Target != "max-hp" {
		t.Fatalf("events: %+v", events)
	}
}
