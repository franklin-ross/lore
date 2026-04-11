# Entity State Tracking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add first-class state tracking (tags and fields) to Lore entities, with display in hover/CLI and LSP completions + diagnostics.

**Architecture:** Directives are parsed inside description bodies by a new directive lexer during `ParseFile`. Parsed directives are stored on each `Description` as a list of `StateEvent`s with source spans. A new resolution phase runs after descriptions are attached in `Merge`, folding each entity's events (in file order) into the current `Tags` and `Fields` while emitting state issues. Display helpers render the resolved state block for hover, CLI, and future LSP formatters. LSP grows a new `PublishDiagnostics` path and context-sensitive completions for directives.

**Tech Stack:** Go 1.24, `io/fs` for virtual filesystems in tests, `github.com/tliron/glsp` for LSP protocol types, `golang.org/x/text/unicode` not needed — we use `unicode.IsLetter` / `unicode.IsDigit` from stdlib.

**Spec:** [`docs/specs/2026-04-11-entity-state-tracking.md`](../../specs/2026-04-11-entity-state-tracking.md)

---

## File Map

**New files:**

- `internal/lore/state.go` — types: `FieldKind`, `FieldValue`, `StateOp`, `StateEvent`, `StateIssue`, `StateSpan`, and helpers.
- `internal/lore/state_test.go` — tests for the types.
- `internal/lore/directive.go` — directive lexer+parser: scans a description body string and returns `[]StateEvent` plus lexer-time `StateIssue`s.
- `internal/lore/directive_test.go` — tests for directive parsing.
- `internal/lore/state_resolve.go` — resolution phase: folds events into current state, emits resolver-time issues.
- `internal/lore/state_resolve_test.go` — tests for resolution.
- `internal/lore/state_display.go` — renders a state block as plain text or markdown.
- `internal/lore/state_display_test.go` — tests for display.

**Modified files:**

- `internal/lore/entity.go` — add `Tags`, `Fields`, `StateHistory`, `StateIssues` to `Entity`; add `Events` to `Description`.
- `internal/lore/merge.go` — invoke directive parser per description; add state resolution phase.
- `internal/lore/world.go` — add `StateIssues` aggregator; extend `Check` to include state issues.
- `internal/lore/parser_test.go` — no changes expected, but new fields should not break existing tests.
- `internal/lsp/server.go` — extend `formatEntityHover` to include state block; add `publishDiagnostics` helper.
- `internal/lsp/document.go` (or `server.go`) — call `publishDiagnostics` on didOpen/didChange/didSave.
- `internal/lsp/completion.go` — extend with context-sensitive directive completions.
- `cmd/main.go` — `cmdQuery` includes state block; `cmdCheck` surfaces state issues.

---

## Task 1: State Types and Entity Extension

**Files:**
- Create: `internal/lore/state.go`
- Create: `internal/lore/state_test.go`
- Modify: `internal/lore/entity.go`

- [ ] **Step 1: Write the failing test**

Create `internal/lore/state_test.go`:

```go
package lore

import "testing"

func TestFieldValueNumeric(t *testing.T) {
	v := FieldValue{Kind: FieldNumeric, Number: 42}
	if v.Kind != FieldNumeric {
		t.Fatalf("kind = %v", v.Kind)
	}
	if v.Number != 42 {
		t.Fatalf("number = %v", v.Number)
	}
}

func TestFieldValueText(t *testing.T) {
	v := FieldValue{Kind: FieldText, Text: []string{"sword", "shield"}}
	if len(v.Text) != 2 {
		t.Fatalf("text len = %d", len(v.Text))
	}
	if v.Text[0] != "sword" || v.Text[1] != "shield" {
		t.Fatalf("text = %v", v.Text)
	}
}

func TestStateEventConstruction(t *testing.T) {
	ev := StateEvent{
		Op:     StateOpSet,
		Target: "status",
		Value:  &FieldValue{Kind: FieldText, Text: []string{"alive"}},
		Span:   StateSpan{File: "test.md", Line: 5, StartByte: 10, EndByte: 22},
	}
	if ev.Op != StateOpSet {
		t.Fatalf("op = %v", ev.Op)
	}
	if ev.Target != "status" {
		t.Fatalf("target = %q", ev.Target)
	}
}

func TestEntityHasStateFields(t *testing.T) {
	var e Entity
	e.Tags = map[string]bool{"injured": true}
	e.Fields = map[string]FieldValue{
		"population": {Kind: FieldNumeric, Number: 100},
	}
	e.StateHistory = []StateEvent{{Op: StateOpAdd, Target: "injured"}}
	if !e.Tags["injured"] {
		t.Fatal("tag not set")
	}
	if e.Fields["population"].Number != 100 {
		t.Fatal("field not set")
	}
	if len(e.StateHistory) != 1 {
		t.Fatal("history not set")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `task test-unit -- ./internal/lore/... -run 'TestFieldValue|TestStateEvent|TestEntityHasStateFields'`
Expected: FAIL with "undefined: FieldValue", "undefined: StateEvent", etc.

- [ ] **Step 3: Create `internal/lore/state.go`**

```go
package lore

// FieldKind is the kind of a field value. A field's kind is fixed at its
// first assignment and cannot change.
type FieldKind int

const (
	FieldUnknown FieldKind = iota
	FieldNumeric
	FieldText
)

// FieldValue holds the current value of a field. Only the member matching
// Kind is meaningful.
type FieldValue struct {
	Kind   FieldKind
	Number float64  // valid when Kind == FieldNumeric
	Text   []string // valid when Kind == FieldText; len >= 1
}

// StateOp is the kind of state-changing operation a StateEvent represents.
type StateOp int

const (
	StateOpUnknown StateOp = iota
	StateOpAdd            // +tag
	StateOpRemove         // -tag, field -= value
	StateOpSet            // field = value
	StateOpIncrement      // field += value
)

// StateSpan is the byte span of a directive within its source file.
type StateSpan struct {
	File      string
	Line      int // 1-based line of the description's header
	StartByte int // byte offset within the description text
	EndByte   int // exclusive
}

// StateEvent is a single parsed state directive. For tag operations
// (StateOpAdd/StateOpRemove with no Value) Target holds the tag name and
// Value is nil. For field operations Target holds the field name and Value
// is non-nil.
type StateEvent struct {
	Op     StateOp
	Target string
	Value  *FieldValue
	Span   StateSpan
}

// StateIssue is a diagnostic produced by directive parsing or state
// resolution. Severity controls how it is displayed.
type StateIssue struct {
	Severity StateIssueSeverity
	Message  string
	Span     StateSpan
}

// StateIssueSeverity classifies a state issue for display purposes.
type StateIssueSeverity int

const (
	SeverityInfo StateIssueSeverity = iota
	SeverityWarning
	SeverityError
)
```

- [ ] **Step 4: Modify `internal/lore/entity.go`**

Update the `Entity` struct and `Description` struct:

```go
// Entity is a named thing in the world: a character, location, quest, etc.
type Entity struct {
	Name         string
	Type         string
	Aliases      []string
	Descriptions []Description

	// Resolved state, populated by Merge after all descriptions are attached.
	Tags         map[string]bool
	Fields       map[string]FieldValue
	StateHistory []StateEvent
	StateIssues  []StateIssue
}

// Description is a block of prose attached to an entity, with its source
// location. Events holds any state directives parsed out of the description
// text, in source order.
type Description struct {
	Text   string
	File   string
	Line   int
	Events []StateEvent
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `task test-unit -- ./internal/lore/... -run 'TestFieldValue|TestStateEvent|TestEntityHasStateFields'`
Expected: PASS.

- [ ] **Step 6: Run the full existing test suite to check for regressions**

Run: `task test-unit`
Expected: PASS (the new fields on Entity and Description are zero-initialised; no existing code touches them yet).

- [ ] **Step 7: Commit**

```bash
git add internal/lore/state.go internal/lore/state_test.go internal/lore/entity.go
git commit -m "feat(state): add state type definitions and entity fields"
```

---

## Task 2: Directive Lexer — Tags

**Files:**
- Create: `internal/lore/directive.go`
- Create: `internal/lore/directive_test.go`

Build the core directive scanner for tag directives (`+tag` and `-tag`), including Unicode identifier rules and the quoted escape hatch.

- [ ] **Step 1: Write the failing test**

Create `internal/lore/directive_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `task test-unit -- ./internal/lore/... -run 'TestParseDirectives'`
Expected: FAIL with "undefined: ParseDirectives".

- [ ] **Step 3: Create `internal/lore/directive.go` with tag support**

```go
package lore

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ParseDirectives scans a description body for state directives and returns
// the parsed events plus any lexer-time issues. File and line identify the
// source location of the description for span tracking.
//
// The scanner walks the text rune by rune, trying to match a directive at
// each position and otherwise advancing. Directives are recognised by shape;
// text that does not match a directive pattern is silently skipped as prose.
func ParseDirectives(text, file string, line int) (events []StateEvent, issues []StateIssue) {
	s := &directiveScanner{
		text: text,
		file: file,
		line: line,
	}
	for s.pos < len(s.text) {
		if ev, ok := s.tryDirective(); ok {
			events = append(events, ev)
			continue
		}
		// Consume issues accumulated while trying to match.
		issues = append(issues, s.takeIssues()...)
		s.advanceRune()
	}
	issues = append(issues, s.takeIssues()...)
	return events, issues
}

type directiveScanner struct {
	text    string
	file    string
	line    int
	pos     int
	pending []StateIssue
}

// tryDirective attempts to parse a directive starting at the current
// position. On success it advances past the directive and returns the event.
// On failure it leaves pos where it was.
func (s *directiveScanner) tryDirective() (StateEvent, bool) {
	start := s.pos
	// Tag directive: `+tag` or `-tag`, possibly `+"quoted"` or `-"quoted"`.
	if s.pos < len(s.text) && (s.text[s.pos] == '+' || s.text[s.pos] == '-') {
		op := StateOpAdd
		if s.text[s.pos] == '-' {
			op = StateOpRemove
		}
		savedPos := s.pos
		s.pos++
		if target, ok := s.readTagName(); ok {
			return StateEvent{
				Op:     op,
				Target: target,
				Span:   s.spanFrom(start),
			}, true
		}
		s.pos = savedPos
	}
	return StateEvent{}, false
}

// readTagName reads either an identifier (bareword) or a quoted-tag escape
// sequence at the current position.
func (s *directiveScanner) readTagName() (string, bool) {
	if s.pos < len(s.text) && s.text[s.pos] == '"' {
		return s.readQuotedTag()
	}
	return s.readIdentifier()
}

// readIdentifier reads a run of letter/digit/underscore/hyphen characters
// starting with a letter. Returns false if the current position doesn't
// begin an identifier.
func (s *directiveScanner) readIdentifier() (string, bool) {
	if s.pos >= len(s.text) {
		return "", false
	}
	r, width := utf8.DecodeRuneInString(s.text[s.pos:])
	if !unicode.IsLetter(r) {
		return "", false
	}
	start := s.pos
	s.pos += width
	for s.pos < len(s.text) {
		r, w := utf8.DecodeRuneInString(s.text[s.pos:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			s.pos += w
			continue
		}
		break
	}
	return s.text[start:s.pos], true
}

// readQuotedTag reads `"multi word tag"` and normalises internal whitespace
// to hyphens. Assumes the current rune is an opening `"`.
func (s *directiveScanner) readQuotedTag() (string, bool) {
	if s.pos >= len(s.text) || s.text[s.pos] != '"' {
		return "", false
	}
	s.pos++
	start := s.pos
	for s.pos < len(s.text) && s.text[s.pos] != '"' {
		s.pos++
	}
	if s.pos >= len(s.text) {
		// Unterminated quoted tag — treat as not a directive.
		return "", false
	}
	raw := s.text[start:s.pos]
	s.pos++ // consume closing quote
	return strings.ReplaceAll(strings.TrimSpace(raw), " ", "-"), true
}

// advanceRune moves past a single rune.
func (s *directiveScanner) advanceRune() {
	_, w := utf8.DecodeRuneInString(s.text[s.pos:])
	if w == 0 {
		s.pos++
		return
	}
	s.pos += w
}

// spanFrom returns a StateSpan covering start..s.pos.
func (s *directiveScanner) spanFrom(start int) StateSpan {
	return StateSpan{
		File:      s.file,
		Line:      s.line,
		StartByte: start,
		EndByte:   s.pos,
	}
}

// takeIssues returns and clears any pending issues.
func (s *directiveScanner) takeIssues() []StateIssue {
	out := s.pending
	s.pending = nil
	return out
}

func (s *directiveScanner) addIssue(severity StateIssueSeverity, message string, span StateSpan) {
	s.pending = append(s.pending, StateIssue{
		Severity: severity,
		Message:  message,
		Span:     span,
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `task test-unit -- ./internal/lore/... -run 'TestParseDirectives'`
Expected: PASS (all 9 tag tests).

- [ ] **Step 5: Commit**

```bash
git add internal/lore/directive.go internal/lore/directive_test.go
git commit -m "feat(state): lex +tag and -tag directives"
```

---

## Task 3: Directive Lexer — Numeric Field Ops

**Files:**
- Modify: `internal/lore/directive.go`
- Modify: `internal/lore/directive_test.go`

Add field-assignment directive recognition for numeric values: `field = 100`, `field += 50`, `field -= 25`. Numeric literals use max-munch, so `100.` parses as `100` followed by a period.

- [ ] **Step 1: Write the failing tests**

Add to `internal/lore/directive_test.go`:

```go
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
	// The period is NOT part of the number (not max-munch with trailing dot).
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `task test-unit -- ./internal/lore/... -run 'TestParseDirectivesNumeric|TestParseDirectivesFieldName'`
Expected: FAIL.

- [ ] **Step 3: Extend `tryDirective` and add field-directive scanning**

Modify `internal/lore/directive.go`. Inside `tryDirective`, after the tag branch but before returning false, add the field-directive branch:

```go
// Field directive: `name`, optional WS, operator `=` / `+=` / `-=`, optional WS, value.
if target, op, ok := s.tryFieldOp(); ok {
	value, vok := s.readValue(op)
	if !vok {
		// Failed to read a value — rewind.
		s.pos = start
		return StateEvent{}, false
	}
	return StateEvent{
		Op:     op,
		Target: target,
		Value:  value,
		Span:   s.spanFrom(start),
	}, true
}
```

Add helper methods:

```go
// tryFieldOp looks for `identifier <ws>? <op>` at the current position. On
// success it returns the target name and operator and advances past the
// operator. On failure it rewinds.
func (s *directiveScanner) tryFieldOp() (string, StateOp, bool) {
	saved := s.pos
	name, ok := s.readIdentifier()
	if !ok {
		return "", StateOpUnknown, false
	}
	s.skipSpacesTabs()
	if s.pos >= len(s.text) {
		s.pos = saved
		return "", StateOpUnknown, false
	}
	c := s.text[s.pos]
	switch c {
	case '=':
		s.pos++
		return name, StateOpSet, true
	case '+':
		if s.pos+1 < len(s.text) && s.text[s.pos+1] == '=' {
			s.pos += 2
			return name, StateOpIncrement, true
		}
	case '-':
		if s.pos+1 < len(s.text) && s.text[s.pos+1] == '=' {
			s.pos += 2
			return name, StateOpRemove, true
		}
	}
	s.pos = saved
	return "", StateOpUnknown, false
}

func (s *directiveScanner) skipSpacesTabs() {
	for s.pos < len(s.text) && (s.text[s.pos] == ' ' || s.text[s.pos] == '\t') {
		s.pos++
	}
}

// readValue reads a value after a field operator. For this task, only
// numeric literals are handled; text values are added in the next task.
func (s *directiveScanner) readValue(op StateOp) (*FieldValue, bool) {
	s.skipSpacesTabs()
	if s.pos >= len(s.text) {
		return nil, false
	}
	// Try number.
	if n, ok := s.tryNumber(); ok {
		return &FieldValue{Kind: FieldNumeric, Number: n}, true
	}
	return nil, false
}

// tryNumber matches an optional leading `-`, then one or more digits, then
// an optional `.` followed by one or more digits. Max-munch, but a trailing
// `.` without digits is NOT consumed.
func (s *directiveScanner) tryNumber() (float64, bool) {
	start := s.pos
	i := start
	if i < len(s.text) && s.text[i] == '-' {
		i++
	}
	digitsStart := i
	for i < len(s.text) && s.text[i] >= '0' && s.text[i] <= '9' {
		i++
	}
	if i == digitsStart {
		return 0, false
	}
	if i+1 < len(s.text) && s.text[i] == '.' && s.text[i+1] >= '0' && s.text[i+1] <= '9' {
		i++
		for i < len(s.text) && s.text[i] >= '0' && s.text[i] <= '9' {
			i++
		}
	}
	n, err := parseFloat(s.text[start:i])
	if err != nil {
		return 0, false
	}
	s.pos = i
	return n, true
}

func parseFloat(s string) (float64, error) {
	return strconvParseFloat(s, 64)
}
```

Add `strconvParseFloat` import alias at the top of the file:

```go
import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)
```

And replace the `parseFloat` helper with a direct call: delete the helper and call `strconv.ParseFloat(...)` directly in `tryNumber`:

```go
n, err := strconv.ParseFloat(s.text[start:i], 64)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `task test-unit -- ./internal/lore/... -run 'TestParseDirectives'`
Expected: PASS (all tag tests still pass, numeric tests now pass).

- [ ] **Step 5: Commit**

```bash
git add internal/lore/directive.go internal/lore/directive_test.go
git commit -m "feat(state): lex numeric field directives"
```

---

## Task 4: Directive Lexer — Text Field Values and Lists

**Files:**
- Modify: `internal/lore/directive.go`
- Modify: `internal/lore/directive_test.go`

Add text-value support: bareword runs, quoted strings, comma-separated lists, and the missing-list-separator diagnostic for quoted-adjacent-bareword. Also add directive termination rules (`.`, `!`, `?`, `;`, newline).

- [ ] **Step 1: Write the failing tests**

Add to `internal/lore/directive_test.go`:

```go
func TestParseDirectivesTextScalarBareword(t *testing.T) {
	events, _ := ParseDirectives("status = alive", "test.md", 1)
	if len(events) != 1 {
		t.Fatalf("events: %+v", events)
	}
	v := events[0].Value
	if v.Kind != FieldText || len(v.Text) != 1 || v.Text[0] != "alive" {
		t.Fatalf("value: %+v", v)
	}
}

func TestParseDirectivesTextScalarQuoted(t *testing.T) {
	events, _ := ParseDirectives(`weapon = "two handed sword"`, "test.md", 1)
	if len(events) != 1 {
		t.Fatalf("events: %+v", events)
	}
	v := events[0].Value
	if v.Kind != FieldText || len(v.Text) != 1 || v.Text[0] != "two handed sword" {
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
	if len(issues) != 1 {
		t.Fatalf("issues: %+v", issues)
	}
	if !strings.Contains(issues[0].Message, "separator") && !strings.Contains(issues[0].Message, "comma") {
		t.Fatalf("issue message: %q", issues[0].Message)
	}
}

func TestParseDirectivesQuotedListItemAfterBareword(t *testing.T) {
	// Reverse of the previous: bareword followed by quoted string with no comma.
	_, issues := ParseDirectives(`inventory += helm "longsword".`, "test.md", 1)
	if len(issues) != 1 {
		t.Fatalf("issues: %+v", issues)
	}
}
```

Add import if missing: `"strings"`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `task test-unit -- ./internal/lore/... -run 'TestParseDirectivesText|TestParseDirectivesTerminator|TestParseDirectivesMissing|TestParseDirectivesQuoted'`
Expected: FAIL.

- [ ] **Step 3: Rewrite `readValue` to handle text values, lists, and termination**

Replace `readValue` in `internal/lore/directive.go`:

```go
// readValue reads a value after a field operator. Returns the parsed value
// and advances past its final token. Stops at a terminator (`.`, `!`, `?`,
// `;`, end of text). Emits issues for missing list separators.
func (s *directiveScanner) readValue(op StateOp) (*FieldValue, bool) {
	s.skipSpacesTabs()
	if s.pos >= len(s.text) {
		return nil, false
	}

	// Numeric literal: only if the text immediately looks like a number.
	// We probe without committing so that a leading '-' followed by text
	// falls through to bareword reading.
	if s.looksLikeNumber() {
		if n, ok := s.tryNumber(); ok {
			return &FieldValue{Kind: FieldNumeric, Number: n}, true
		}
	}

	// Text value: read comma-separated items until a terminator.
	items := []string{}
	for {
		s.skipSpacesTabs()
		if s.pos >= len(s.text) || s.isTerminator(s.text[s.pos]) {
			break
		}
		item, ok := s.readListItem()
		if !ok {
			return nil, false
		}
		items = append(items, item)
		s.skipSpacesTabs()
		if s.pos >= len(s.text) || s.isTerminator(s.text[s.pos]) {
			break
		}
		if s.text[s.pos] == ',' {
			s.pos++
			continue
		}
		// Neither terminator nor comma — missing separator between items.
		issueStart := s.pos
		s.addIssue(SeverityWarning, "expected ',' or terminator between list items; did you forget a comma?", StateSpan{
			File:      s.file,
			Line:      s.line,
			StartByte: issueStart,
			EndByte:   issueStart,
		})
		// Consume until the next comma or terminator to avoid cascading errors.
		for s.pos < len(s.text) && s.text[s.pos] != ',' && !s.isTerminator(s.text[s.pos]) {
			s.advanceRune()
		}
		if s.pos < len(s.text) && s.text[s.pos] == ',' {
			s.pos++
		}
	}

	if len(items) == 0 {
		return nil, false
	}
	return &FieldValue{Kind: FieldText, Text: items}, true
}

// looksLikeNumber reports whether the current position begins a number
// literal (optional '-' then at least one digit).
func (s *directiveScanner) looksLikeNumber() bool {
	i := s.pos
	if i < len(s.text) && s.text[i] == '-' {
		i++
	}
	return i < len(s.text) && s.text[i] >= '0' && s.text[i] <= '9'
}

// isTerminator reports whether b ends a directive at the top level.
func (s *directiveScanner) isTerminator(b byte) bool {
	return b == '.' || b == '!' || b == '?' || b == ';' || b == '\n' || b == '\r'
}

// readListItem reads a single list item: either a quoted string or a
// whitespace-containing bareword run. Returns the item text and advances.
func (s *directiveScanner) readListItem() (string, bool) {
	if s.pos >= len(s.text) {
		return "", false
	}
	if s.text[s.pos] == '"' {
		item, ok := s.readQuotedString()
		if !ok {
			return "", false
		}
		// After a quoted string in a list, the next non-whitespace must be
		// a comma or terminator. If it's a bareword/quote, that's a missing
		// separator.
		s.skipSpacesTabs()
		if s.pos < len(s.text) && !s.isTerminator(s.text[s.pos]) && s.text[s.pos] != ',' {
			issueStart := s.pos
			s.addIssue(SeverityWarning, "expected ',' or terminator after list item; did you forget a comma?", StateSpan{
				File:      s.file,
				Line:      s.line,
				StartByte: issueStart,
				EndByte:   issueStart,
			})
			// Skip the stray content up to the next comma or terminator.
			for s.pos < len(s.text) && s.text[s.pos] != ',' && !s.isTerminator(s.text[s.pos]) {
				s.advanceRune()
			}
		}
		return item, true
	}
	return s.readBarewordRun()
}

// readQuotedString reads `"...contents..."`. Assumes the current byte is `"`.
func (s *directiveScanner) readQuotedString() (string, bool) {
	if s.pos >= len(s.text) || s.text[s.pos] != '"' {
		return "", false
	}
	s.pos++
	start := s.pos
	for s.pos < len(s.text) && s.text[s.pos] != '"' {
		s.pos++
	}
	if s.pos >= len(s.text) {
		return "", false
	}
	raw := s.text[start:s.pos]
	s.pos++ // closing quote
	return raw, true
}

// readBarewordRun reads a run of words separated by spaces, stopping at
// a comma, terminator, or quoted string.
func (s *directiveScanner) readBarewordRun() (string, bool) {
	start := s.pos
	lastWordEnd := s.pos
	for s.pos < len(s.text) {
		r, w := utf8.DecodeRuneInString(s.text[s.pos:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			s.pos += w
			lastWordEnd = s.pos
			continue
		}
		// Allow internal spaces: peek ahead to see if another bareword follows.
		if r == ' ' || r == '\t' {
			savedPos := s.pos
			s.skipSpacesTabs()
			if s.pos >= len(s.text) {
				s.pos = savedPos
				break
			}
			next, _ := utf8.DecodeRuneInString(s.text[s.pos:])
			if unicode.IsLetter(next) || unicode.IsDigit(next) {
				continue
			}
			s.pos = savedPos
			break
		}
		break
	}
	if lastWordEnd == start {
		return "", false
	}
	return strings.TrimSpace(s.text[start:lastWordEnd]), true
}
```

Also update `tryDirective` to terminate correctly — the existing tag path already terminates at the end of the tag name, which is fine. The field path now consumes the value through `readValue`.

Add a final adjustment: in `tryDirective`, after a successful field directive, advance past the terminator if one is present, so the outer scan doesn't re-evaluate it:

Do NOT advance past the terminator — leave it in place. The outer scanner will skip it via `advanceRune`, and it doesn't start another directive because tag scans require `+`/`-` and field scans require an identifier followed by an operator.

- [ ] **Step 4: Run tests to verify they pass**

Run: `task test-unit -- ./internal/lore/...`
Expected: PASS (all directive tests + regressions).

- [ ] **Step 5: Commit**

```bash
git add internal/lore/directive.go internal/lore/directive_test.go
git commit -m "feat(state): lex text field values, lists, and termination"
```

---

## Task 5: Directive-Run-On Diagnostic

**Files:**
- Modify: `internal/lore/directive.go`
- Modify: `internal/lore/directive_test.go`

Detect text values that contain directive operators (`=`, `+=`, `-=`) outside quoted regions, and emit an info-level hint suggesting `;` as a separator.

- [ ] **Step 1: Write the failing tests**

Add to `internal/lore/directive_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `task test-unit -- ./internal/lore/... -run 'TestParseDirectivesRunOn|TestParseDirectivesNoRunOn'`
Expected: FAIL (tests run but the diagnostic isn't emitted).

- [ ] **Step 3: Emit the run-on diagnostic**

In `internal/lore/directive.go`, extend `readValue` so that after reading each list item from a bareword run, it scans the item text for operator substrings and emits an issue if found. Add a helper:

```go
// checkRunOn emits a diagnostic if the given text value item contains a
// state operator substring, which usually means the author forgot a `;`.
func (s *directiveScanner) checkRunOn(item string, span StateSpan) {
	// Only applies to bareword items. Quoted items are exempt (they're
	// deliberately literal).
	if containsOperator(item) {
		s.addIssue(SeverityInfo, "value contains operator; separate directives with ';'", span)
	}
}

func containsOperator(s string) bool {
	return strings.Contains(s, "+=") || strings.Contains(s, "-=") || strings.Contains(s, "=")
}
```

In `readListItem`, when returning from `readBarewordRun`, also record the span covering that item and call `checkRunOn`:

```go
if s.text[s.pos] == '"' {
	// ... existing quoted handling, no run-on check ...
}
itemStart := s.pos
item, ok := s.readBarewordRun()
if !ok {
	return "", false
}
s.checkRunOn(item, StateSpan{
	File:      s.file,
	Line:      s.line,
	StartByte: itemStart,
	EndByte:   s.pos,
})
return item, true
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `task test-unit -- ./internal/lore/... -run 'TestParseDirectives'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/lore/directive.go internal/lore/directive_test.go
git commit -m "feat(state): add directive run-on diagnostic"
```

---

## Task 6: State Resolution — Tags

**Files:**
- Create: `internal/lore/state_resolve.go`
- Create: `internal/lore/state_resolve_test.go`

Build the resolution phase that folds `StateEvent`s into an entity's current state. This task covers tags only; fields come in the next tasks.

- [ ] **Step 1: Write the failing tests**

Create `internal/lore/state_resolve_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `task test-unit -- ./internal/lore/... -run 'TestResolveStateTags'`
Expected: FAIL with "undefined: ResolveState".

- [ ] **Step 3: Create `internal/lore/state_resolve.go`**

```go
package lore

import "fmt"

// ResolveState folds a sequence of state events (in file order) into a
// resolved tag set, field map, and any issues produced by the resolution
// phase. Lexer-time issues are not returned here — callers combine them
// with the returned resolution issues.
func ResolveState(events []StateEvent) (map[string]bool, map[string]FieldValue, []StateIssue) {
	tags := make(map[string]bool)
	fields := make(map[string]FieldValue)
	var issues []StateIssue

	for _, ev := range events {
		switch ev.Op {
		case StateOpAdd:
			if ev.Value == nil {
				// Tag add.
				tags[ev.Target] = true
			}
			// Field add is an increment; handled in later tasks.
		case StateOpRemove:
			if ev.Value == nil {
				// Tag remove.
				if !tags[ev.Target] {
					issues = append(issues, StateIssue{
						Severity: SeverityWarning,
						Message:  fmt.Sprintf("tag %q is not currently active", ev.Target),
						Span:     ev.Span,
					})
				}
				delete(tags, ev.Target)
			}
			// Field remove is handled in later tasks.
		}
	}

	return tags, fields, issues
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `task test-unit -- ./internal/lore/... -run 'TestResolveStateTags'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/lore/state_resolve.go internal/lore/state_resolve_test.go
git commit -m "feat(state): resolve tag add/remove events"
```

---

## Task 7: State Resolution — Numeric Fields

**Files:**
- Modify: `internal/lore/state_resolve.go`
- Modify: `internal/lore/state_resolve_test.go`

Extend the resolver to handle numeric field init, increment, decrement, and kind-conflict diagnostics.

- [ ] **Step 1: Write the failing tests**

Add to `internal/lore/state_resolve_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `task test-unit -- ./internal/lore/... -run 'TestResolveStateNumeric|TestResolveStateKindConflict'`
Expected: FAIL.

- [ ] **Step 3: Extend the resolver**

Modify the `ResolveState` loop in `internal/lore/state_resolve.go`. Add a field-handling branch before the tag branches:

```go
for _, ev := range events {
	if ev.Value != nil {
		issues = append(issues, applyFieldEvent(fields, ev)...)
		continue
	}
	// Tag handling (existing code).
	switch ev.Op {
	case StateOpAdd:
		tags[ev.Target] = true
	case StateOpRemove:
		if !tags[ev.Target] {
			issues = append(issues, StateIssue{
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("tag %q is not currently active", ev.Target),
				Span:     ev.Span,
			})
		}
		delete(tags, ev.Target)
	}
}
```

Add the field helper:

```go
// applyFieldEvent applies a single field-valued event to the fields map,
// returning any issues produced. Numeric-only for this task.
func applyFieldEvent(fields map[string]FieldValue, ev StateEvent) []StateIssue {
	var issues []StateIssue
	existing, hasExisting := fields[ev.Target]

	switch ev.Op {
	case StateOpSet:
		if hasExisting && existing.Kind != ev.Value.Kind {
			issues = append(issues, StateIssue{
				Severity: SeverityError,
				Message:  fmt.Sprintf("cannot change kind of field %q", ev.Target),
				Span:     ev.Span,
			})
			return issues
		}
		fields[ev.Target] = *ev.Value
	case StateOpIncrement:
		if !hasExisting {
			if ev.Value.Kind == FieldNumeric {
				fields[ev.Target] = *ev.Value
			} else {
				// Text append on uninitialised field — initialise with the item.
				fields[ev.Target] = FieldValue{Kind: FieldText, Text: append([]string(nil), ev.Value.Text...)}
			}
			return issues
		}
		if existing.Kind != ev.Value.Kind {
			issues = append(issues, StateIssue{
				Severity: SeverityError,
				Message:  fmt.Sprintf("cannot %s %s to %s field %q", opName(ev.Op), kindName(ev.Value.Kind), kindName(existing.Kind), ev.Target),
				Span:     ev.Span,
			})
			return issues
		}
		if ev.Value.Kind == FieldNumeric {
			existing.Number += ev.Value.Number
			fields[ev.Target] = existing
		}
		// Text append handled in next task.
	case StateOpRemove:
		if !hasExisting {
			issues = append(issues, StateIssue{
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("field %q is not initialised", ev.Target),
				Span:     ev.Span,
			})
			return issues
		}
		if existing.Kind != ev.Value.Kind {
			issues = append(issues, StateIssue{
				Severity: SeverityError,
				Message:  fmt.Sprintf("cannot %s %s from %s field %q", opName(ev.Op), kindName(ev.Value.Kind), kindName(existing.Kind), ev.Target),
				Span:     ev.Span,
			})
			return issues
		}
		if ev.Value.Kind == FieldNumeric {
			existing.Number -= ev.Value.Number
			fields[ev.Target] = existing
		}
		// Text remove handled in next task.
	}
	return issues
}

func opName(op StateOp) string {
	switch op {
	case StateOpIncrement:
		return "append"
	case StateOpRemove:
		return "remove"
	case StateOpSet:
		return "assign"
	}
	return "apply"
}

func kindName(k FieldKind) string {
	switch k {
	case FieldNumeric:
		return "numeric"
	case FieldText:
		return "text"
	}
	return "unknown"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `task test-unit -- ./internal/lore/... -run 'TestResolveState'`
Expected: PASS (tag tests still pass, numeric tests pass).

- [ ] **Step 5: Commit**

```bash
git add internal/lore/state_resolve.go internal/lore/state_resolve_test.go
git commit -m "feat(state): resolve numeric field ops with kind conflict checks"
```

---

## Task 8: State Resolution — Text Fields

**Files:**
- Modify: `internal/lore/state_resolve.go`
- Modify: `internal/lore/state_resolve_test.go`

Implement text field resolution: initialise with items, append with `+=`, remove with `-=`, and emit diagnostics for remove-missing and kind conflicts.

- [ ] **Step 1: Write the failing tests**

Add to `internal/lore/state_resolve_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `task test-unit -- ./internal/lore/... -run 'TestResolveStateText'`
Expected: FAIL on append/remove/missing (numeric paths pass from previous task).

- [ ] **Step 3: Complete text handling in `applyFieldEvent`**

In `internal/lore/state_resolve.go`, modify the `StateOpIncrement` text branch and the `StateOpRemove` text branch:

```go
case StateOpIncrement:
	// ... existing numeric branch ...
	if ev.Value.Kind == FieldText {
		combined := append([]string(nil), existing.Text...)
		combined = append(combined, ev.Value.Text...)
		fields[ev.Target] = FieldValue{Kind: FieldText, Text: combined}
	}
```

```go
case StateOpRemove:
	// ... existing numeric branch ...
	if ev.Value.Kind == FieldText {
		remaining := existing.Text
		for _, itemToRemove := range ev.Value.Text {
			idx := -1
			for i, it := range remaining {
				if it == itemToRemove {
					idx = i
					break
				}
			}
			if idx < 0 {
				issues = append(issues, StateIssue{
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("list %q has no item %q to remove", ev.Target, itemToRemove),
					Span:     ev.Span,
				})
				continue
			}
			remaining = append(remaining[:idx], remaining[idx+1:]...)
		}
		if len(remaining) == 0 {
			delete(fields, ev.Target)
		} else {
			fields[ev.Target] = FieldValue{Kind: FieldText, Text: remaining}
		}
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `task test-unit -- ./internal/lore/... -run 'TestResolveState'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/lore/state_resolve.go internal/lore/state_resolve_test.go
git commit -m "feat(state): resolve text field ops with kind and remove checks"
```

---

## Task 9: Wire Directive Parsing into Description Parsing

**Files:**
- Modify: `internal/lore/merge.go`
- Modify: `internal/lore/parser_test.go` (add coverage)

Invoke `ParseDirectives` for every description produced during `ParseFile`/`Merge`. Store the events on the `Description` struct.

- [ ] **Step 1: Write the failing test**

Add to `internal/lore/parser_test.go`:

```go
func TestParseDescriptionCapturesDirectives(t *testing.T) {
	world := setupTestWorld(t, "Sildar (character): Fighter. +injured\n")
	sildar, err := world.FindEntity("Sildar")
	if err != nil {
		t.Fatal(err)
	}
	if len(sildar.Descriptions) != 1 {
		t.Fatalf("descriptions: %+v", sildar.Descriptions)
	}
	if len(sildar.Descriptions[0].Events) != 1 {
		t.Fatalf("events: %+v", sildar.Descriptions[0].Events)
	}
	ev := sildar.Descriptions[0].Events[0]
	if ev.Op != StateOpAdd || ev.Target != "injured" {
		t.Fatalf("event: %+v", ev)
	}
	if ev.Span.File != "test.md" {
		t.Fatalf("span file: %+v", ev.Span)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `task test-unit -- ./internal/lore/... -run TestParseDescriptionCapturesDirectives`
Expected: FAIL (Description.Events is empty).

- [ ] **Step 3: Invoke the directive parser during Merge**

In `internal/lore/merge.go`, in Phase 2 where descriptions are attached, parse directives before appending:

```go
if def.Description == "" {
	continue
}
events, _ := ParseDirectives(def.Description, fp.Path, def.Line)
ent.Descriptions = append(ent.Descriptions, Description{
	Text:   def.Description,
	File:   fp.Path,
	Line:   def.Line,
	Events: events,
})
```

Lexer-time issues will be used in the next task, so discard them here for now (we'll capture them then).

- [ ] **Step 4: Run tests to verify they pass**

Run: `task test-unit -- ./internal/lore/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/lore/merge.go internal/lore/parser_test.go
git commit -m "feat(state): parse directives from each description during merge"
```

---

## Task 10: State Resolution Phase in Merge

**Files:**
- Modify: `internal/lore/merge.go`
- Modify: `internal/lore/world.go`
- Modify: `internal/lore/parser_test.go`

Add a resolution phase to `Merge` that walks each entity's descriptions in file order, collects events and lexer issues, resolves state, and stores the resolved tags/fields and combined issues on the entity.

- [ ] **Step 1: Write the failing test**

Add to `internal/lore/parser_test.go`:

```go
func TestMergeResolvesEntityState(t *testing.T) {
	project := setupTestProject(t, map[string]string{
		"a-first.md":  "Phandalin (location): Sleepy. population = 100\n",
		"b-second.md": "Phandalin: Raided. population -= 50\n",
	})

	world, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}
	p, err := world.FindEntity("Phandalin")
	if err != nil {
		t.Fatal(err)
	}
	pop, ok := p.Fields["population"]
	if !ok || pop.Number != 50 {
		t.Fatalf("population: %+v", p.Fields)
	}
	if len(p.StateIssues) != 0 {
		t.Fatalf("issues: %+v", p.StateIssues)
	}
}

func TestMergeSurfacesStateIssues(t *testing.T) {
	world := setupTestWorld(t, "Sildar (character): Fighter. -injured\n")
	sildar, _ := world.FindEntity("Sildar")
	if len(sildar.StateIssues) != 1 {
		t.Fatalf("issues: %+v", sildar.StateIssues)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `task test-unit -- ./internal/lore/... -run 'TestMergeResolvesEntityState|TestMergeSurfacesStateIssues'`
Expected: FAIL.

- [ ] **Step 3: Modify `Merge` to invoke `ResolveState` per entity**

Add a new phase in `internal/lore/merge.go` after Phase 2 (description attachment) and before Phase 3 (cross-references):

```go
// Phase 2.5: resolve state for each entity from the accumulated descriptions.
// Events are in file-order because Phase 2 walks sorted files in scan order.
for i := range world.Entities {
	ent := &world.Entities[i]
	var events []StateEvent
	var lexIssues []StateIssue
	for _, desc := range ent.Descriptions {
		events = append(events, desc.Events...)
	}
	// Re-run the lexer on each description text to collect issues. The
	// Description.Events already contains the parsed events; here we
	// recover lexer-time issues the same way.
	for _, desc := range ent.Descriptions {
		_, iss := ParseDirectives(desc.Text, desc.File, desc.Line)
		lexIssues = append(lexIssues, iss...)
	}
	tags, fields, resolveIssues := ResolveState(events)
	ent.Tags = tags
	ent.Fields = fields
	ent.StateHistory = events
	ent.StateIssues = append(lexIssues, resolveIssues...)
}
```

This re-parses each description to capture lexer issues (events were already captured in Task 9). A cleaner refactor (return events+issues from a single call and cache both on the Description) can come later if desired — for now, correctness first.

Actually, refactor now: capture issues on the Description too. Add `LexIssues []StateIssue` to `Description` in `internal/lore/entity.go`:

```go
type Description struct {
	Text      string
	File      string
	Line      int
	Events    []StateEvent
	LexIssues []StateIssue
}
```

Then in Phase 2 (Task 9's change), capture issues:

```go
events, lexIssues := ParseDirectives(def.Description, fp.Path, def.Line)
ent.Descriptions = append(ent.Descriptions, Description{
	Text:      def.Description,
	File:      fp.Path,
	Line:      def.Line,
	Events:    events,
	LexIssues: lexIssues,
})
```

And Phase 2.5 becomes:

```go
for i := range world.Entities {
	ent := &world.Entities[i]
	var events []StateEvent
	var lexIssues []StateIssue
	for _, desc := range ent.Descriptions {
		events = append(events, desc.Events...)
		lexIssues = append(lexIssues, desc.LexIssues...)
	}
	tags, fields, resolveIssues := ResolveState(events)
	ent.Tags = tags
	ent.Fields = fields
	ent.StateHistory = events
	ent.StateIssues = append(lexIssues, resolveIssues...)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `task test-unit -- ./internal/lore/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/lore/merge.go internal/lore/entity.go internal/lore/parser_test.go
git commit -m "feat(state): resolve state for every entity during merge"
```

---

## Task 11: State Display Helper

**Files:**
- Create: `internal/lore/state_display.go`
- Create: `internal/lore/state_display_test.go`

Render a state block from an entity's resolved tags and fields. Sorted alphabetically. Items containing separators or `=` are quoted in the display. Returns the empty string for empty state.

- [ ] **Step 1: Write the failing test**

Create `internal/lore/state_display_test.go`:

```go
package lore

import "testing"

func TestFormatStateEmpty(t *testing.T) {
	out := FormatStateBlock(nil, nil)
	if out != "" {
		t.Fatalf("expected empty, got %q", out)
	}
}

func TestFormatStateTagsOnly(t *testing.T) {
	tags := map[string]bool{"injured": true, "bleeding": true}
	out := FormatStateBlock(tags, nil)
	// Tags sorted alphabetically, each prefixed with +.
	want := "+bleeding +injured"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestFormatStateNumericField(t *testing.T) {
	fields := map[string]FieldValue{
		"population": {Kind: FieldNumeric, Number: 100},
	}
	out := FormatStateBlock(nil, fields)
	want := "population: 100"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestFormatStateTextListField(t *testing.T) {
	fields := map[string]FieldValue{
		"inventory": {Kind: FieldText, Text: []string{"longsword", "chainmail"}},
	}
	out := FormatStateBlock(nil, fields)
	want := "inventory: chainmail, longsword"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestFormatStateTextSingleItemRendersAsScalar(t *testing.T) {
	fields := map[string]FieldValue{
		"status": {Kind: FieldText, Text: []string{"alive"}},
	}
	out := FormatStateBlock(nil, fields)
	if out != "status: alive" {
		t.Fatalf("got %q", out)
	}
}

func TestFormatStateQuotedItemsContainingSeparators(t *testing.T) {
	fields := map[string]FieldValue{
		"inventory": {Kind: FieldText, Text: []string{"potion, red", "sword"}},
	}
	out := FormatStateBlock(nil, fields)
	// Sorted alphabetically: "potion, red" < "sword"
	want := `inventory: "potion, red", sword`
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestFormatStateQuotesEqualsInItem(t *testing.T) {
	fields := map[string]FieldValue{
		"notes": {Kind: FieldText, Text: []string{"key = value"}},
	}
	out := FormatStateBlock(nil, fields)
	if out != `notes: "key = value"` {
		t.Fatalf("got %q", out)
	}
}

func TestFormatStateCombined(t *testing.T) {
	tags := map[string]bool{"injured": true}
	fields := map[string]FieldValue{
		"population": {Kind: FieldNumeric, Number: 42},
	}
	out := FormatStateBlock(tags, fields)
	want := "+injured\npopulation: 42"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `task test-unit -- ./internal/lore/... -run TestFormatState`
Expected: FAIL with "undefined: FormatStateBlock".

- [ ] **Step 3: Create `internal/lore/state_display.go`**

```go
package lore

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// FormatStateBlock renders a state summary (tags + fields) as plain text.
// Returns an empty string if there is no state to show.
//
// Output format:
//
//	+tag1 +tag2
//	field1: value
//	field2: a, b, c
//
// Tags line first (if any), then fields alphabetically. Items containing
// separators or '=' are wrapped in quotes.
func FormatStateBlock(tags map[string]bool, fields map[string]FieldValue) string {
	var lines []string

	if len(tags) > 0 {
		names := make([]string, 0, len(tags))
		for t := range tags {
			names = append(names, t)
		}
		sort.Strings(names)
		parts := make([]string, len(names))
		for i, n := range names {
			parts[i] = "+" + n
		}
		lines = append(lines, strings.Join(parts, " "))
	}

	if len(fields) > 0 {
		names := make([]string, 0, len(fields))
		for n := range fields {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			lines = append(lines, formatField(n, fields[n]))
		}
	}

	return strings.Join(lines, "\n")
}

func formatField(name string, v FieldValue) string {
	switch v.Kind {
	case FieldNumeric:
		return fmt.Sprintf("%s: %s", name, formatNumber(v.Number))
	case FieldText:
		items := append([]string(nil), v.Text...)
		sort.Strings(items)
		parts := make([]string, len(items))
		for i, it := range items {
			parts[i] = quoteIfNeeded(it)
		}
		return fmt.Sprintf("%s: %s", name, strings.Join(parts, ", "))
	}
	return fmt.Sprintf("%s: ?", name)
}

func formatNumber(n float64) string {
	if n == float64(int64(n)) {
		return strconv.FormatInt(int64(n), 10)
	}
	return strconv.FormatFloat(n, 'g', -1, 64)
}

// quoteIfNeeded wraps item in double quotes if it contains a character that
// would otherwise confuse a reader scanning the rendered list: separators
// (`,`), sentence punctuation, or `=`. `+` and `-` are left alone since
// they appear naturally in compound words.
func quoteIfNeeded(item string) string {
	if strings.ContainsAny(item, ",.!?;=") {
		return `"` + item + `"`
	}
	return item
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `task test-unit -- ./internal/lore/... -run TestFormatState`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/lore/state_display.go internal/lore/state_display_test.go
git commit -m "feat(state): render state block with sorted, quoted items"
```

---

## Task 12: Hover Integration

**Files:**
- Modify: `internal/lsp/server.go`
- Modify: `internal/lsp/server_test.go` (or `lifecycle_test.go` if that's where hover is tested)

Include the state block at the top of the hover markdown for each entity.

- [ ] **Step 1: Inspect existing hover tests**

Run: `task test-unit -- ./internal/lsp/... -run Hover -v`

Look at `internal/lsp/server_test.go` to find an existing hover test and its format.

- [ ] **Step 2: Write the failing test**

Add to `internal/lsp/server_test.go` (or the file where hover is tested):

```go
func TestHoverIncludesStateBlock(t *testing.T) {
	srv := setupLSPServer(t, map[string]string{
		"sildar.md": "Sildar (character): Fighter. +injured population = 0\n",
	})
	// Position the cursor on "Sildar" in a line that references it.
	// Use the same helper other hover tests use.
	// ...

	// Request hover, assert that the returned markdown contains "+injured".
	// The exact helper invocation depends on the existing test style.
}
```

Adapt this skeleton to the existing hover test helpers in this package — the key assertion is `strings.Contains(hoverContent, "+injured")` and similar for any field. If no hover helper exists, use `formatEntityHover` directly:

```go
func TestFormatEntityHoverIncludesState(t *testing.T) {
	ent := &lore.Entity{
		Name: "Sildar",
		Type: "character",
		Descriptions: []lore.Description{
			{Text: "Fighter.", File: "t.md", Line: 1},
		},
		Tags: map[string]bool{"injured": true},
	}
	out := formatEntityHover(ent)
	if !strings.Contains(out, "+injured") {
		t.Fatalf("hover missing state block: %q", out)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `task test-unit -- ./internal/lsp/... -run TestFormatEntityHoverIncludesState`
Expected: FAIL.

- [ ] **Step 4: Extend `formatEntityHover`**

Modify `internal/lsp/server.go`, replacing the `formatEntityHover` function:

```go
func formatEntityHover(ent *lore.Entity) string {
	var b strings.Builder
	if ent.Type != "" {
		fmt.Fprintf(&b, "**%s** (%s)", ent.Name, ent.Type)
	} else {
		fmt.Fprintf(&b, "**%s**", ent.Name)
	}
	if len(ent.Aliases) > 0 {
		fmt.Fprintf(&b, "\n\nAlso known as: %s", strings.Join(ent.Aliases, ", "))
	}
	if state := lore.FormatStateBlock(ent.Tags, ent.Fields); state != "" {
		b.WriteString("\n\n```\n")
		b.WriteString(state)
		b.WriteString("\n```")
	}
	if len(ent.Descriptions) > 0 {
		b.WriteString("\n\n---\n\n")
		texts := make([]string, len(ent.Descriptions))
		for i, d := range ent.Descriptions {
			texts[i] = d.Text
		}
		b.WriteString(truncate(strings.Join(texts, "\n\n"), 2000))
	}
	return b.String()
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `task test-unit -- ./internal/lsp/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/lsp/server.go internal/lsp/server_test.go
git commit -m "feat(lsp): include state block in entity hover"
```

---

## Task 13: CLI Query State Block

**Files:**
- Modify: `cmd/main.go`
- Modify: `e2e_test.go`

Show the state block in `lore query <name>` output, above the description.

- [ ] **Step 1: Write the failing e2e test**

Open `e2e_test.go` and find an existing `lore query` test. Add:

```go
func TestQueryIncludesStateBlock(t *testing.T) {
	dir := setupProject(t, map[string]string{
		"sildar.md": "Sildar (character): Fighter. +injured population = 3\n",
	})
	out := runLore(t, dir, "query", "Sildar")
	if !strings.Contains(out, "+injured") {
		t.Fatalf("query output missing tag: %q", out)
	}
	if !strings.Contains(out, "population: 3") {
		t.Fatalf("query output missing field: %q", out)
	}
}
```

Use whatever helper names `e2e_test.go` already uses for `setupProject` and `runLore` — check an existing test first to match the style.

- [ ] **Step 2: Run the test to verify it fails**

Run: `task test-e2e -- -run TestQueryIncludesStateBlock`
Expected: FAIL.

- [ ] **Step 3: Modify `cmdQuery` in `cmd/main.go`**

In the function body, after printing the aliases line and before iterating descriptions:

```go
if state := lore.FormatStateBlock(ent.Tags, ent.Fields); state != "" {
	fmt.Println(state)
	fmt.Println()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `task test-e2e`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/main.go e2e_test.go
git commit -m "feat(cli): show state block in lore query output"
```

---

## Task 14: CLI Check Surfaces State Issues

**Files:**
- Modify: `internal/lore/world.go`
- Modify: `cmd/main.go`
- Modify: `e2e_test.go`

Extend `World.Check()` to include state issues from every entity, converting them into the existing `Issue` type.

- [ ] **Step 1: Write the failing test**

Add to `e2e_test.go`:

```go
func TestCheckReportsStateIssues(t *testing.T) {
	dir := setupProject(t, map[string]string{
		"sildar.md": "Sildar (character): Fighter. -injured\n",
	})
	out := runLore(t, dir, "check")
	if !strings.Contains(out, "injured") {
		t.Fatalf("check output missing state issue: %q", out)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `task test-e2e -- -run TestCheckReportsStateIssues`
Expected: FAIL.

- [ ] **Step 3: Extend `World.Check`**

In `internal/lore/world.go`, replace the placeholder implementation:

```go
func (w *World) Check() []Issue {
	var issues []Issue
	for _, ent := range w.Entities {
		for _, si := range ent.StateIssues {
			issues = append(issues, Issue{
				File:    si.Span.File,
				Line:    si.Span.Line,
				Message: si.Message,
			})
		}
	}
	return issues
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `task test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/lore/world.go e2e_test.go
git commit -m "feat(cli): surface state issues in lore check"
```

---

## Task 15: LSP PublishDiagnostics Plumbing

**Files:**
- Modify: `internal/lsp/server.go`
- Modify: `internal/lsp/document.go` (or wherever didOpen/didChange/didSave live)
- Modify: `internal/lsp/server_test.go`

Add a helper that publishes all state issues for a given file as LSP diagnostics. Call it on didOpen, didChange, and didSave.

- [ ] **Step 1: Locate the didOpen/didChange/didSave handlers**

Run: `grep -n "didOpen\|didChange\|didSave\|DidOpen\|DidChange\|DidSave" internal/lsp/*.go`

Identify which file owns each handler and note them.

- [ ] **Step 2: Write the failing test**

Add to `internal/lsp/server_test.go` (follow the existing style for creating a server and feeding a document):

```go
func TestPublishDiagnosticsForStateIssues(t *testing.T) {
	srv := newTestServer(t) // use whatever helper exists
	openDocument(t, srv, "file:///test.md", "Sildar (character): Fighter. -injured\n")
	diags := collectPublishedDiagnostics(t, srv, "file:///test.md")
	if len(diags) != 1 {
		t.Fatalf("diags: %+v", diags)
	}
	if !strings.Contains(diags[0].Message, "injured") {
		t.Fatalf("diag message: %q", diags[0].Message)
	}
}
```

If `collectPublishedDiagnostics` / `openDocument` helpers don't exist, stub them out at the bottom of the test file. They should drive the server the same way other tests do.

- [ ] **Step 3: Run the test to verify it fails**

Run: `task test-unit -- ./internal/lsp/... -run TestPublishDiagnostics`
Expected: FAIL.

- [ ] **Step 4: Implement `publishDiagnostics`**

In `internal/lsp/server.go`, add:

```go
import (
	"strings"

	"lore/internal/lore"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"github.com/tliron/glsp"
)

// publishDiagnostics pushes state issues for the given file path to the
// editor. Call after any index mutation that affects that file's issues.
func (s *Server) publishDiagnostics(ctx *glsp.Context, uri, path string) {
	world := s.world()
	var items []protocol.Diagnostic
	for _, ent := range world.Entities {
		for _, si := range ent.StateIssues {
			if si.Span.File != path {
				continue
			}
			items = append(items, toProtocolDiagnostic(si))
		}
	}
	ctx.Notify("textDocument/publishDiagnostics", &protocol.PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: items,
	})
}

func toProtocolDiagnostic(si lore.StateIssue) protocol.Diagnostic {
	sev := protocol.DiagnosticSeverityInformation
	switch si.Severity {
	case lore.SeverityWarning:
		sev = protocol.DiagnosticSeverityWarning
	case lore.SeverityError:
		sev = protocol.DiagnosticSeverityError
	}
	line := uint32(si.Span.Line - 1)
	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{Line: line, Character: uint32(si.Span.StartByte)},
			End:   protocol.Position{Line: line, Character: uint32(si.Span.EndByte)},
		},
		Severity: &sev,
		Source:   ptrStr("lore"),
		Message:  si.Message,
	}
}
```

Note: the span's byte offsets are measured relative to the description text, not the line. For v1 we accept that the diagnostic range may be approximate (the editor will still highlight the right line). Improving this to absolute file coordinates is a later refinement.

- [ ] **Step 5: Call `publishDiagnostics` from the lifecycle handlers**

In the didOpen/didChange/didSave handlers, after updating the index, call:

```go
s.publishDiagnostics(ctx, params.TextDocument.URI, projectRelativePath)
```

where `projectRelativePath` is computed the same way existing handlers do it (look for an existing helper that maps URI → project path).

- [ ] **Step 6: Run tests to verify they pass**

Run: `task test-unit -- ./internal/lsp/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/lsp/server.go internal/lsp/document.go internal/lsp/server_test.go
git commit -m "feat(lsp): publish state issues as diagnostics on document changes"
```

---

## Task 16: LSP Completions for State Directives

**Files:**
- Modify: `internal/lsp/completion.go`
- Modify: `internal/lsp/server_test.go`

Extend the completion handler with context-sensitive directive completions: tag names after `+`, currently-active tags after `-`, and list item completions for text field `+=` / `-=`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/lsp/server_test.go`:

```go
func TestCompletionSuggestsTagsAfterPlus(t *testing.T) {
	srv := setupLSPServer(t, map[string]string{
		"a.md": "Sildar (character): Fighter. +injured +bleeding\n",
		"b.md": "Gundren (character): Dwarf. +\n", // cursor at end of line after '+'
	})
	items := requestCompletion(t, srv, "file:///b.md", /*line*/ 0, /*character*/ 30)
	if !containsLabel(items, "injured") || !containsLabel(items, "bleeding") {
		t.Fatalf("labels: %+v", items)
	}
}

func TestCompletionSuggestsActiveTagsAfterMinus(t *testing.T) {
	srv := setupLSPServer(t, map[string]string{
		"a.md": "Sildar (character): Fighter. +injured +bleeding\n-\n",
	})
	items := requestCompletion(t, srv, "file:///a.md", /*line*/ 1, /*character*/ 1)
	if !containsLabel(items, "injured") {
		t.Fatalf("labels: %+v", items)
	}
}
```

Adjust the character positions to match the actual text layout. Add helper stubs if they don't exist. This task is the most LSP-specific — follow whatever style the existing `TestCompletion*` tests use.

- [ ] **Step 2: Run tests to verify they fail**

Run: `task test-unit -- ./internal/lsp/... -run TestCompletion`
Expected: FAIL (existing tests pass, new ones fail).

- [ ] **Step 3: Extend `completion.go`**

Modify `internal/lsp/completion.go`. At the top of the handler, inspect the character immediately before the cursor:

```go
func (s *Server) completion(ctx *glsp.Context, params *protocol.CompletionParams) (any, error) {
	uri := params.TextDocument.URI
	line := params.Position.Line
	char := params.Position.Character

	content, _ := s.indexContent(uri)
	lineText := extractLine(content, int(line))
	if int(char) > 0 && int(char) <= len(lineText) {
		prev := lineText[char-1]
		if prev == '+' {
			return s.tagCompletions(true), nil
		}
		if prev == '-' {
			return s.tagCompletions(false), nil
		}
	}

	return s.entityCompletions(), nil // existing behaviour
}
```

Move the current body of `completion` into a new `entityCompletions` helper that returns `*protocol.CompletionList`. Add:

```go
// tagCompletions returns all known tags across the project (allKnown=true)
// or tags currently active somewhere in the project (allKnown=false).
// Per-entity "currently active" context is deferred; v1 suggests all tags
// ever seen or all tags currently active on any entity.
func (s *Server) tagCompletions(allKnown bool) *protocol.CompletionList {
	world := s.world()
	seen := make(map[string]bool)
	for _, ent := range world.Entities {
		if allKnown {
			for _, ev := range ent.StateHistory {
				if ev.Value == nil && (ev.Op == StateOpAdd || ev.Op == StateOpRemove) {
					seen[ev.Target] = true
				}
			}
		} else {
			for tag := range ent.Tags {
				seen[tag] = true
			}
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)

	kind := protocol.CompletionItemKindKeyword
	items := make([]protocol.CompletionItem, 0, len(names))
	for _, n := range names {
		items = append(items, protocol.CompletionItem{
			Label: n,
			Kind:  &kind,
		})
	}
	return &protocol.CompletionList{Items: items}
}
```

Add imports for `sort` and `lore` (reference `lore.StateOpAdd` etc. instead of bare names — the snippet above uses short form; qualify them properly).

Also add `indexContent` and `extractLine` helpers — or reuse existing ones if the package already has them. Look at `internal/lsp/position.go` for existing line-extraction logic.

- [ ] **Step 4: Run tests to verify they pass**

Run: `task test-unit -- ./internal/lsp/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/lsp/completion.go internal/lsp/server_test.go
git commit -m "feat(lsp): complete tag names after + and active tags after -"
```

---

## Task 17: Documentation — Update format.md and todo.md

**Files:**
- Modify: `docs/format.md`
- Modify: `docs/todo.md`

Update user-facing docs to mention state tracking.

- [ ] **Step 1: Update `docs/format.md`**

Add a new section after "Description" and before "Multiple Definitions":

```markdown
## State Directives

Entity descriptions can include **state directives** — compact tags and
fields that track the current state of an entity across sessions.

### Tags

Bare labels you add or remove:

    Sildar: Took an arrow to the knee. +injured
    Sildar: Patched up by the cleric. -injured

Multi-word tags use hyphens: `+critically-injured`.

### Fields

Named values. Numeric fields support arithmetic; text fields hold ordered
lists (a single-value list renders as a scalar):

    Phandalin (location): Sleepy frontier town. population = 100
    Phandalin: Redbrands raid the square. population -= 50

    Sildar: Hands us his longsword. inventory += "longsword"
    Sildar: Gives the sword to Gundren. inventory -= "longsword"

Multi-word bareword values are fine (`inventory += two-handed sword`).
Quotes are only needed when the value contains punctuation.

### Display

The current state of an entity is shown above its description in
`lore query` output and in VSCode hover tooltips. The full specification
lives in [`docs/specs/2026-04-11-entity-state-tracking.md`](specs/2026-04-11-entity-state-tracking.md).
```

- [ ] **Step 2: Update `docs/todo.md`**

Remove (or mark done) any state-tracking items that this plan fulfils, and leave a note pointing at the spec. Strike through the task list items that are now complete.

- [ ] **Step 3: Commit**

```bash
git add docs/format.md docs/todo.md
git commit -m "docs: document entity state directives in format.md"
```

---

## Self-Review Checklist

After completing all tasks, verify against the spec:

- [ ] Tags: `+tag`, `-tag`, hyphenated, quoted escape, Unicode — Task 2 + Task 6.
- [ ] Numeric fields: `=`, `+=`, `-=`, init/increment/decrement — Task 3 + Task 7.
- [ ] Text fields: bareword, multi-word bareword, quoted, list, init/append/remove — Tasks 4, 8.
- [ ] Termination: `.`, `!`, `?`, `;`, newline, inside quoted/number — Task 4.
- [ ] Multiple directives via `;` — Task 4.
- [ ] Directive run-on diagnostic — Task 5.
- [ ] Missing list separator diagnostic — Task 4.
- [ ] Kind conflict diagnostics — Tasks 7, 8.
- [ ] Remove-missing diagnostics (tags + fields) — Tasks 6, 7, 8.
- [ ] Display block (sorted, quoted items) — Task 11.
- [ ] Hover shows state block — Task 12.
- [ ] CLI query shows state block — Task 13.
- [ ] CLI check surfaces state issues — Task 14.
- [ ] LSP publishes state diagnostics — Task 15.
- [ ] LSP completes tags after `+` and `-` — Task 16.
- [ ] Format.md documents the feature — Task 17.

**Spec requirements explicitly out of scope:**
- Casing drift diagnostic — deferred; add in a follow-up if it proves valuable.
- LSP completions for field names / list items in `+=` / `-=` beyond tags — deferred to a follow-up. The v1 suggestion set is tag-focused.
- Faded/italic styling of directives in hover rendering — deferred; the display currently shows the raw description, which includes the directive tokens, but without special styling.
- Dim ANSI for directives in CLI — deferred.
- Point-in-time queries — spec non-goal.

These deferrals should be tracked in `docs/todo.md` under "Outstanding Work" as follow-ups.
