package lore

import (
	"strconv"
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
			issues = append(issues, s.takeIssues()...)
			events = append(events, ev)
			continue
		}
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
		if !s.atWordBoundaryLeft() {
			return StateEvent{}, false
		}
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
	// Field directive: `name`, optional WS, operator `=` / `+=` / `-=`, optional WS, value.
	if target, op, ok := s.tryFieldOp(); ok {
		value, vok := s.readValue()
		if !vok {
			// Failed to read a value — rewind to start.
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
	tag := strings.ReplaceAll(strings.TrimSpace(raw), " ", "-")
	if tag == "" {
		return "", false
	}
	return tag, true
}

// atWordBoundaryLeft reports whether the character immediately before s.pos
// is a word boundary — either start-of-input or a non-word rune (non-letter,
// non-digit, and not '_' or '-'). This prevents directive sigils from
// firing mid-word, so `foo+injured` is prose, not a +injured directive.
func (s *directiveScanner) atWordBoundaryLeft() bool {
	if s.pos == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(s.text[:s.pos])
	return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-')
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

// tryFieldOp looks for `identifier <ws>? <op>` at the current position. On
// success it returns the target name and operator and advances past the
// operator. On failure it rewinds to the original position.
func (s *directiveScanner) tryFieldOp() (string, StateOp, bool) {
	saved := s.pos
	if !s.atWordBoundaryLeft() {
		return "", StateOpUnknown, false
	}
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

// skipSpacesTabs advances past ASCII spaces and tabs.
func (s *directiveScanner) skipSpacesTabs() {
	for s.pos < len(s.text) && (s.text[s.pos] == ' ' || s.text[s.pos] == '\t') {
		s.pos++
	}
}

// readValue reads a value after a field operator. For this task, only
// numeric literals are handled; text values are added in the next task.
func (s *directiveScanner) readValue() (*FieldValue, bool) {
	s.skipSpacesTabs()
	if s.pos >= len(s.text) {
		return nil, false
	}
	if n, ok := s.tryNumber(); ok {
		return &FieldValue{Kind: FieldNumeric, Number: n}, true
	}
	return nil, false
}

// tryNumber matches an optional leading `-`, then one or more digits, then
// an optional `.` followed by one or more digits. Max-munch, but a trailing
// `.` without digits is NOT consumed (so `100.` leaves the `.` as a
// terminator and parses `100`).
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
	n, err := strconv.ParseFloat(s.text[start:i], 64)
	if err != nil {
		return 0, false
	}
	s.pos = i
	return n, true
}
