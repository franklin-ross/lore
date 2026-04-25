package lore

import "strings"

// blankInlineAsides returns a copy of text where every inline-aside range
// (the entire `(Subject: body)` parenthetical) is replaced with spaces of
// the same byte length. Used by merge to strip aside contents out of a
// definition body before scanning for directives, so directives inside the
// aside aren't double-counted as the owner's events.
func blankInlineAsides(text string) string {
	spans := findInlineAsideRanges(text)
	if len(spans) == 0 {
		return text
	}
	buf := []byte(text)
	for _, sp := range spans {
		for i := sp.StartByte; i < sp.EndByte && i < len(buf); i++ {
			if buf[i] != '\n' && buf[i] != '\r' {
				buf[i] = ' '
			}
		}
	}
	return string(buf)
}

// findInlineAsideRanges scans text for top-level paren-balanced groups whose
// contents look like a header definition (`Name: rest` or `Name (type): rest`).
// Returned spans cover the outer `(` through the matching `)` inclusive,
// in byte coordinates relative to text. Newlines disqualify a group — a
// stray `(` doesn't swallow the rest of the prose looking for its match.
//
// Used by stripDirectivesFromText so the entire aside disappears from a
// description's CleanText, not just the inner directives.
func findInlineAsideRanges(text string) []StateSpan {
	var spans []StateSpan
	for _, g := range scanParenGroups(text) {
		contents := text[g.start+1 : g.end-1]
		if !looksLikeHeader(contents) {
			continue
		}
		spans = append(spans, StateSpan{StartByte: g.start, EndByte: g.end})
	}
	return spans
}

// parenGroup is a half-open byte range covering `(` through `)` inclusive
// (i.e. end is one past the closing paren) in some source text.
type parenGroup struct {
	start int
	end   int
}

// scanParenGroups returns the outer paren-balanced groups in text, ignoring
// any `(` that doesn't match a `)` on the same line. Nested groups are
// consumed as part of their outer.
func scanParenGroups(text string) []parenGroup {
	var groups []parenGroup
	i := 0
	for i < len(text) {
		if text[i] != '(' {
			i++
			continue
		}
		end := matchCloseParen(text, i)
		if end < 0 {
			i++
			continue
		}
		groups = append(groups, parenGroup{start: i, end: end + 1})
		i = end + 1
	}
	return groups
}

// matchCloseParen returns the byte offset of the `)` that balances the `(`
// at openPos. It tracks depth and bails out on any newline or end-of-text
// before depth reaches zero.
func matchCloseParen(text string, openPos int) int {
	depth := 1
	for i := openPos + 1; i < len(text); i++ {
		c := text[i]
		if c == '\n' || c == '\r' {
			return -1
		}
		if c == '(' {
			depth++
			continue
		}
		if c == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// looksLikeHeader reports whether contents could plausibly be the inside
// of an inline aside — a header line of the form `Name: rest` or
// `Name (type): rest`. ParseHeader is the source of truth.
func looksLikeHeader(contents string) bool {
	_, ok := ParseHeader(strings.TrimSpace(contents))
	return ok
}

// inlineAsideHit is a concrete inline aside found in raw file content,
// already resolved through ParseHeader. ParseFile uses these to synthesise
// Definitions for asides written inline in prose, so they participate in
// merge/resolve like any other definition.
type inlineAsideHit struct {
	Line       int    // 1-based file line of the opening '('
	Header     Header // parsed from the aside's contents
	Body       string // header.DescStart, kept on the struct for clarity
	BodyLine   int    // 1-based file line where the body starts
	BodyColumn int    // 0-based byte column on BodyLine
}

// extractInlineAsides walks file content and returns every aside whose
// contents pass ParseHeader. The body's line/column are computed so a
// synthesised Definition can map directive spans back to real file
// locations.
func extractInlineAsides(content string) []inlineAsideHit {
	if !strings.ContainsRune(content, '(') {
		return nil
	}
	lineStarts := buildLineStarts(content)

	var hits []inlineAsideHit
	for _, g := range scanParenGroups(content) {
		raw := content[g.start+1 : g.end-1]
		header, ok := ParseHeader(strings.TrimSpace(raw))
		if !ok {
			continue
		}
		colon := indexHeaderColon(raw)
		if colon < 0 {
			continue
		}
		// Body starts at the colon inside the aside, plus one for the colon
		// itself, plus the leading whitespace ParseHeader stripped from
		// DescStart.
		bodyStart := g.start + 1 + colon + 1
		for bodyStart < g.end-1 && (content[bodyStart] == ' ' || content[bodyStart] == '\t') {
			bodyStart++
		}
		openLine, _ := lineColAt(lineStarts, g.start)
		bodyLine, bodyCol := lineColAt(lineStarts, bodyStart)
		hits = append(hits, inlineAsideHit{
			Line:       openLine,
			Header:     header,
			Body:       header.DescStart,
			BodyLine:   bodyLine,
			BodyColumn: bodyCol,
		})
	}
	return hits
}

// buildLineStarts returns the byte offset of the start of each line in s.
// Index 0 is always 0; subsequent entries are positions immediately after
// each '\n'.
func buildLineStarts(s string) []int {
	starts := []int{0}
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// lineColAt returns the 1-based line and 0-based byte column for the byte
// offset pos using the line starts table.
func lineColAt(starts []int, pos int) (int, int) {
	lo, hi := 0, len(starts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if starts[mid] <= pos {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo + 1, pos - starts[lo]
}
