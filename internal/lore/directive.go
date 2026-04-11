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
