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
			// The field name and operator matched, but the value is missing
			// or malformed (e.g. `inventory =` with nothing after, or
			// `inventory = +sword` where `+sword` isn't a valid field value).
			// Report a diagnostic covering the attempted directive and
			// swallow it up to the next terminator so stray sigils inside
			// the broken value don't fire as tag directives.
			s.skipSpacesTabs()
			s.skipToTerminator()
			s.addIssue(SeverityWarning, "expected a value after field operator", s.spanFrom(start))
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

// skipToTerminator advances s.pos past everything up to (but not including)
// the next top-level directive terminator. Used to recover from a broken
// directive so its malformed tail doesn't seed spurious directives further
// along.
func (s *directiveScanner) skipToTerminator() {
	for s.pos < len(s.text) && !s.isTerminator(s.text[s.pos]) {
		s.advanceRune()
	}
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

// readValue reads a value after a field operator. Returns the parsed value
// and advances past its final token. Stops at a terminator (`.`, `!`, `?`,
// `;`, newline, end of text). Emits issues for missing list separators.
//
// For a numeric literal (first non-space char looks like a number), returns
// a numeric FieldValue. Otherwise reads a comma-separated sequence of text
// items; the result is always a text FieldValue with at least one item.
func (s *directiveScanner) readValue() (*FieldValue, bool) {
	s.skipSpacesTabs()
	if s.pos >= len(s.text) || s.isTerminator(s.text[s.pos]) {
		return nil, false
	}

	// Numeric literal path: only if the text immediately looks like a number
	// AND the matched number is a complete token — i.e. followed by a
	// terminator, comma, whitespace, or end-of-text. That lets values like
	// `2026/02/01` or `1.5cm` fall through to the text path instead of being
	// silently truncated to the leading digits.
	if s.looksLikeNumber() {
		saved := s.pos
		if n, ok := s.tryNumber(); ok {
			if s.pos >= len(s.text) || s.isTerminator(s.text[s.pos]) || s.text[s.pos] == ',' || s.text[s.pos] == ' ' || s.text[s.pos] == '\t' {
				return &FieldValue{Kind: FieldNumeric, Number: n}, true
			}
			s.pos = saved
		}
	}

	// Text value: read comma-separated items until a terminator.
	var items []string
	for {
		s.skipSpacesTabs()
		if s.pos >= len(s.text) || s.isTerminator(s.text[s.pos]) {
			break
		}
		item, ok := s.readListItem()
		if !ok {
			break
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
		// Neither terminator nor comma at this position — list is done.
		// Quoted↔bareword adjacency inside an item is reported by
		// readListItem/checkQuotedAdjacency. Bareword↔bareword "missing
		// separators" are indistinguishable from legitimate multi-word items
		// (e.g. "two handed sword") and are accepted as-is per the spec —
		// the current-state display will reveal the unexpected grouping.
		break
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
// whitespace-containing bareword run. Emits a missing-separator diagnostic
// if it detects a quoted-adjacent-bareword (or bareword-adjacent-quoted)
// sequence with no comma between them.
func (s *directiveScanner) readListItem() (string, bool) {
	if s.pos >= len(s.text) {
		return "", false
	}
	if s.text[s.pos] == '"' {
		item, ok := s.readQuotedString()
		if !ok {
			return "", false
		}
		s.checkQuotedAdjacency()
		return item, true
	}
	itemStart := s.pos
	item, ok := s.readBarewordRun()
	if !ok {
		return "", false
	}
	s.checkRunOn(StateSpan{
		File:      s.file,
		Line:      s.line,
		StartByte: itemStart,
		EndByte:   s.pos,
	})
	// After a bareword run, check for a stray quoted string (the reverse
	// adjacency). If found, emit the same diagnostic.
	s.skipSpacesTabs()
	if s.pos < len(s.text) && s.text[s.pos] == '"' {
		s.addIssue(SeverityWarning, "expected ',' or terminator before quoted list item; did you forget a comma?", StateSpan{
			File:      s.file,
			Line:      s.line,
			StartByte: s.pos,
			EndByte:   s.pos,
		})
	}
	return item, true
}

// checkQuotedAdjacency is called after reading a quoted string. If the next
// non-space character is a bareword letter, the author forgot a comma.
// Emit a diagnostic.
func (s *directiveScanner) checkQuotedAdjacency() {
	save := s.pos
	s.skipSpacesTabs()
	if s.pos >= len(s.text) {
		s.pos = save
		return
	}
	c := s.text[s.pos]
	if c == ',' || s.isTerminator(c) {
		s.pos = save
		return
	}
	// Non-separator, non-terminator — probably a bareword.
	r, _ := utf8.DecodeRuneInString(s.text[s.pos:])
	if unicode.IsLetter(r) {
		s.addIssue(SeverityWarning, "expected ',' or terminator after quoted list item; did you forget a comma?", StateSpan{
			File:      s.file,
			Line:      s.line,
			StartByte: s.pos,
			EndByte:   s.pos,
		})
	}
	s.pos = save
}

// readQuotedString reads `"...contents..."`. Assumes the current byte is `"`.
// Returns the inner contents verbatim. Unterminated strings return false.
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
	if raw == "" {
		return "", false
	}
	return raw, true
}

// readBarewordRun reads a run of words separated by single spaces, stopping
// at a comma, terminator, or quoted string. Internal whitespace is
// preserved; edge whitespace is trimmed. Returns the trimmed text.
//
// A "word" here is a maximal run of letters, digits, and the additional
// value-friendly punctuation listed in isBarewordRune — underscore, hyphen,
// slash, colon, apostrophe, and parentheses. Words are joined by runs of
// ASCII space/tab, which are preserved as single characters in the output
// but collapsed at the edges.
func (s *directiveScanner) readBarewordRun() (string, bool) {
	start := s.pos
	lastWordEnd := s.pos
	for s.pos < len(s.text) {
		r, w := utf8.DecodeRuneInString(s.text[s.pos:])
		if isBarewordRune(r) {
			s.pos += w
			lastWordEnd = s.pos
			continue
		}
		if r == ' ' || r == '\t' {
			// Peek ahead: if another word character follows, keep going.
			savedPos := s.pos
			s.skipSpacesTabs()
			if s.pos >= len(s.text) {
				s.pos = savedPos
				break
			}
			next, _ := utf8.DecodeRuneInString(s.text[s.pos:])
			if isBarewordRune(next) {
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
	// Rewind s.pos back to the last word's end so trailing whitespace after
	// the bareword run doesn't get consumed here. Callers do their own
	// whitespace skipping before checking for commas/terminators.
	s.pos = lastWordEnd
	return strings.TrimSpace(s.text[start:lastWordEnd]), true
}

// isBarewordRune reports whether r is allowed inside an unquoted field value.
// Includes letters, digits, and common value-shaped punctuation: underscore,
// hyphen, slash (dates, paths), colon (timestamps, ratios), apostrophe
// (possessives), and parentheses (disambiguators like "Barovia (town)").
// Stays out of the tag/identifier set — tag names and field names still use
// the stricter readIdentifier rules.
func isBarewordRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	switch r {
	case '_', '-', '/', ':', '\'', '(', ')':
		return true
	}
	return false
}

// checkRunOn emits an info-level diagnostic on the bareword item just read
// (covered by span) if the text immediately after it looks like another
// field directive. This catches "run-on" directives where the author
// forgot a ';' separator. Example:
//
//	inventory += helm health -= 3.
//
// After reading the bareword "helm health", s.pos sits before " -= 3.".
// hasFieldOpAhead detects that and we flag the preceding item so the editor
// can squiggle it.
//
// Quoted values are exempt because readListItem handles them separately and
// does not call this function.
func (s *directiveScanner) checkRunOn(span StateSpan) {
	if s.hasFieldOpAhead() {
		s.addIssue(SeverityInfo, "value contains operator; separate directives with ';'", span)
	}
}

// hasFieldOpAhead peeks ahead from s.pos (without moving it) and returns
// true if the remaining text (after optional spaces/tabs) contains a field
// directive operator (=, +=, or -=) before any terminator. This is the
// signal for a run-on directive.
func (s *directiveScanner) hasFieldOpAhead() bool {
	i := s.pos
	// Skip spaces and tabs.
	for i < len(s.text) && (s.text[i] == ' ' || s.text[i] == '\t') {
		i++
	}
	if i >= len(s.text) {
		return false
	}
	// Check for -=
	if s.text[i] == '-' && i+1 < len(s.text) && s.text[i+1] == '=' {
		return true
	}
	// Check for +=
	if s.text[i] == '+' && i+1 < len(s.text) && s.text[i+1] == '=' {
		return true
	}
	// Check for a bare `=`, which is the "`field = value` follows" case
	// without a repeated identifier (e.g. `inventory += helm = 5`).
	if s.text[i] == '=' {
		return true
	}
	// Check for a bare identifier followed by an operator.
	// The identifier must start with a letter.
	r, w := utf8.DecodeRuneInString(s.text[i:])
	if !unicode.IsLetter(r) {
		return false
	}
	i += w
	for i < len(s.text) {
		r, w = utf8.DecodeRuneInString(s.text[i:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			i += w
			continue
		}
		break
	}
	// Skip spaces after the identifier.
	for i < len(s.text) && (s.text[i] == ' ' || s.text[i] == '\t') {
		i++
	}
	if i >= len(s.text) {
		return false
	}
	c := s.text[i]
	if c == '=' {
		return true
	}
	if (c == '+' || c == '-') && i+1 < len(s.text) && s.text[i+1] == '=' {
		return true
	}
	return false
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
