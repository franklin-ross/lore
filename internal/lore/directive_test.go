package lore

import (
	"strings"
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

func TestParseDirectivesTextScalarBareword(t *testing.T) {
	events, _ := ParseDirectives("status = alive", "test.md", 1)
	if len(events) != 1 {
		t.Fatalf("events: %+v", events)
	}
	v := events[0].Value
	if v == nil || v.Kind != FieldText || len(v.Text) != 1 || v.Text[0] != "alive" {
		t.Fatalf("value: %+v", v)
	}
}

func TestParseDirectivesTextScalarQuoted(t *testing.T) {
	events, _ := ParseDirectives(`weapon = "two handed sword"`, "test.md", 1)
	if len(events) != 1 {
		t.Fatalf("events: %+v", events)
	}
	v := events[0].Value
	if v == nil || v.Kind != FieldText || len(v.Text) != 1 || v.Text[0] != "two handed sword" {
		t.Fatalf("value: %+v", v)
	}
}

func TestParseDirectivesTextMultiWordBareword(t *testing.T) {
	events, _ := ParseDirectives("status = wounded and dying. Blah.", "test.md", 1)
	if len(events) != 1 {
		t.Fatalf("events: %+v", events)
	}
	if events[0].Value.Text[0] != "wounded and dying" {
		t.Fatalf("text: %+v", events[0].Value.Text)
	}
}

func TestParseDirectivesTextList(t *testing.T) {
	events, _ := ParseDirectives("inventory = helm, boots, two-handed sword. We left.", "test.md", 1)
	if len(events) != 1 {
		t.Fatalf("events: %+v", events)
	}
	v := events[0].Value
	if len(v.Text) != 3 {
		t.Fatalf("items: %+v", v.Text)
	}
	if v.Text[0] != "helm" || v.Text[1] != "boots" || v.Text[2] != "two-handed sword" {
		t.Fatalf("items: %+v", v.Text)
	}
}

func TestParseDirectivesTextListQuotedWithComma(t *testing.T) {
	events, _ := ParseDirectives(`inventory += "potion, red", torch`, "test.md", 1)
	if len(events) != 1 {
		t.Fatalf("events: %+v", events)
	}
	v := events[0].Value
	if len(v.Text) != 2 {
		t.Fatalf("items: %+v", v.Text)
	}
	if v.Text[0] != "potion, red" || v.Text[1] != "torch" {
		t.Fatalf("items: %+v", v.Text)
	}
}

func TestParseDirectivesTextAppend(t *testing.T) {
	events, _ := ParseDirectives(`inventory += "longsword"`, "test.md", 1)
	if len(events) != 1 || events[0].Op != StateOpIncrement {
		t.Fatalf("events: %+v", events)
	}
}

func TestParseDirectivesTerminatorSemicolon(t *testing.T) {
	events, _ := ParseDirectives("inventory += helm; health -= 3. Done.", "test.md", 1)
	if len(events) != 2 {
		t.Fatalf("events: %+v", events)
	}
	if events[0].Target != "inventory" || events[0].Op != StateOpIncrement {
		t.Fatalf("first: %+v", events[0])
	}
	if events[1].Target != "health" || events[1].Op != StateOpRemove {
		t.Fatalf("second: %+v", events[1])
	}
}

func TestParseDirectivesTerminatorSentencePunctuation(t *testing.T) {
	for _, term := range []string{".", "!", "?"} {
		text := "status = alive" + term + " And something else."
		events, _ := ParseDirectives(text, "test.md", 1)
		if len(events) != 1 {
			t.Fatalf("term %q events: %+v", term, events)
		}
		if events[0].Value.Text[0] != "alive" {
			t.Fatalf("term %q value: %+v", term, events[0].Value.Text)
		}
	}
}

func TestParseDirectivesMissingListSeparatorDiagnostic(t *testing.T) {
	events, issues := ParseDirectives(`inventory += "two handed sword" helm.`, "test.md", 1)
	if len(events) != 1 {
		t.Fatalf("events: %+v", events)
	}
	if len(issues) == 0 {
		t.Fatalf("expected a missing-separator diagnostic")
	}
	found := false
	for _, iss := range issues {
		if strings.Contains(iss.Message, "separator") || strings.Contains(iss.Message, "comma") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issue messages: %+v", issues)
	}
}

func TestParseDirectivesQuotedListItemAfterBareword(t *testing.T) {
	_, issues := ParseDirectives(`inventory += helm "longsword".`, "test.md", 1)
	if len(issues) == 0 {
		t.Fatalf("expected a missing-separator diagnostic, got nothing")
	}
}

func TestParseDirectivesEmptyQuotedValueNotAccepted(t *testing.T) {
	// An empty quoted string as a value must not produce a text value with
	// an empty item — that would violate FieldValue's "len >= 1" invariant.
	events, _ := ParseDirectives(`weapon = ""`, "test.md", 1)
	if len(events) != 0 {
		t.Fatalf("expected no event, got %+v", events)
	}
}

func TestParseDirectivesBarewordBarewordIsAcceptedAsMultiWord(t *testing.T) {
	// Per the spec, bareword↔bareword without a comma is not a diagnostic.
	// `inventory += sword, shield, and we kept walking.` parses as three
	// items, with "and we kept walking" as the third — surprising but
	// intended (the user can see the mistake in the state display).
	events, issues := ParseDirectives("inventory += sword, shield, and we kept walking.", "test.md", 1)
	if len(events) != 1 {
		t.Fatalf("events: %+v", events)
	}
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	items := events[0].Value.Text
	if len(items) != 3 || items[0] != "sword" || items[1] != "shield" || items[2] != "and we kept walking" {
		t.Fatalf("items: %+v", items)
	}
}

func TestParseDirectivesTrailingCommaAccepted(t *testing.T) {
	// A trailing comma after the last item is silently accepted.
	events, _ := ParseDirectives("inventory += helm,", "test.md", 1)
	if len(events) != 1 || len(events[0].Value.Text) != 1 || events[0].Value.Text[0] != "helm" {
		t.Fatalf("events: %+v", events)
	}
}

func TestParseDirectivesRunOnDiagnostic(t *testing.T) {
	// `inventory += helm health -= 3.` parses the whole thing as one text value.
	_, issues := ParseDirectives("inventory += helm health -= 3.", "test.md", 1)
	if len(issues) == 0 {
		t.Fatal("expected a run-on diagnostic")
	}
	found := false
	for _, iss := range issues {
		if strings.Contains(iss.Message, ";") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no `;` suggestion in issues: %+v", issues)
	}
}

func TestParseDirectivesRunOnDiagnosticInListItem(t *testing.T) {
	// `inventory += helm, health -= 3.` — second list item is "health -= 3".
	_, issues := ParseDirectives("inventory += helm, health -= 3.", "test.md", 1)
	if len(issues) == 0 {
		t.Fatal("expected a run-on diagnostic")
	}
}

func TestParseDirectivesNoRunOnWhenQuoted(t *testing.T) {
	// A value that LOOKS like a directive but is quoted should not fire.
	_, issues := ParseDirectives(`note = "health -= 3 is how we track it"`, "test.md", 1)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
}

func TestParseDirectivesNoRunOnForEqualsInSingleWord(t *testing.T) {
	// A single-word bareword value without any operator character should not fire.
	_, issues := ParseDirectives("status = alive", "test.md", 1)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
}
