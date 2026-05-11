package lore

import (
	"fmt"
	"strings"
)

// replaceInlineAsidesWithName returns text with each inline aside
// `(Name (type): body)` replaced by the entity's display name, so prose that
// originally read "told us of a (Destroyed Town (landmark): …) worth a look"
// reduces to "told us of a Destroyed Town worth a look" rather than losing
// the noun entirely. When the aside's name resolves ambiguously in `world`,
// the replacement carries a `(type)` suffix to match how the user would
// otherwise disambiguate references in regular prose.
func replaceInlineAsidesWithName(text string, world *World) string {
	if !strings.ContainsRune(text, '(') {
		return text
	}
	var b strings.Builder
	b.Grow(len(text))
	pos := 0
	for _, g := range scanParenGroups(text) {
		contents := text[g.start+1 : g.end-1]
		header, ok := ParseHeader(strings.TrimSpace(contents))
		if !ok {
			continue
		}
		b.WriteString(text[pos:g.start])
		b.WriteString(displayNameForAside(header, world))
		pos = g.end
	}
	b.WriteString(text[pos:])
	return b.String()
}

// displayNameForAside returns the rendered name for an inline-aside header.
// It uses world ambiguity to decide whether a `(type)` suffix is needed:
// unambiguous lookups render as the bare name (which reads naturally inside
// prose), while ambiguous ones get the disambiguator the user would have
// written by hand.
func displayNameForAside(h Header, world *World) string {
	if world == nil {
		return h.Name
	}
	_, err := world.FindEntity(h.Name)
	if _, ambiguous := err.(*AmbiguousError); ambiguous && h.Type != "" {
		return fmt.Sprintf("%s (%s)", h.Name, h.Type)
	}
	return h.Name
}

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

// scanAllParenGroups walks text and yields every paren-balanced group,
// including ones nested inside an outer group. Result is in source order
// (outer before inner). Used by aside extraction so a `(Andúril ...)`
// written inside a block `(Aragorn: ...)` body can still surface as its
// own entity definition.
func scanAllParenGroups(text string) []parenGroup {
	var groups []parenGroup
	var walk func(s, e int)
	walk = func(s, e int) {
		i := s
		for i < e {
			if text[i] != '(' {
				i++
				continue
			}
			end := matchCloseParen(text, i)
			if end < 0 || end >= e {
				i++
				continue
			}
			groups = append(groups, parenGroup{start: i, end: end + 1})
			walk(i+1, end)
			i = end + 1
		}
	}
	walk(0, len(text))
	return groups
}

// scanParenGroups returns the outer paren-balanced groups in text. Nested
// groups are consumed as part of their outer. Groups may span multiple
// lines — a `(Name:` opened on one line and closed by `)` on a later line
// is a single group, which is what enables block-form asides.
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
// at openPos. Tracks depth across newlines so block-form asides like
// `(Aragorn:\n\nbody\n)` resolve as a single group. Fenced code blocks are
// skipped so a stray `)` inside ```...``` doesn't close prematurely.
// Returns -1 if EOF is reached before depth hits zero.
func matchCloseParen(text string, openPos int) int {
	depth := 1
	i := openPos + 1
	for i < len(text) {
		c := text[i]
		if c == '`' && i+2 < len(text) && text[i+1] == '`' && text[i+2] == '`' {
			// Treat ``` as a code fence only at line start. Inline
			// triple-backticks mid-line don't open a multi-line fence.
			atLineStart := i == 0 || text[i-1] == '\n'
			if atLineStart {
				end := skipFencedCodeBlock(text, i+3)
				if end < 0 {
					return -1
				}
				i = end
				continue
			}
		}
		if c == '(' {
			depth++
			i++
			continue
		}
		if c == ')' {
			depth--
			if depth == 0 {
				return i
			}
			i++
			continue
		}
		i++
	}
	return -1
}

// skipFencedCodeBlock advances past a fenced code block whose opening ```
// just ended at startAfterOpen. Returns the byte offset immediately after
// the closing fence, or -1 if no closing fence is found.
func skipFencedCodeBlock(text string, startAfterOpen int) int {
	nl := strings.IndexByte(text[startAfterOpen:], '\n')
	if nl < 0 {
		return -1
	}
	pos := startAfterOpen + nl + 1
	for pos < len(text) {
		if pos+3 <= len(text) && text[pos] == '`' && text[pos+1] == '`' && text[pos+2] == '`' {
			return pos + 3
		}
		nl := strings.IndexByte(text[pos:], '\n')
		if nl < 0 {
			return -1
		}
		pos = pos + nl + 1
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

// inlineAsideHit is a concrete aside found in raw file content, already
// resolved through ParseHeader. ParseFile uses these to synthesise
// Definitions for asides written in prose (inline `(Name: body)`) or
// across paragraphs (block `(Name:\n\nbody\n)`), so they participate in
// merge/resolve like any other definition.
type inlineAsideHit struct {
	Line        int           // 1-based file line of the opening '('
	Header      Header        // parsed from the aside's contents
	Body        string        // header.DescStart, kept on the struct for clarity
	BodyLine    int           // 1-based file line where the body starts
	BodyColumn  int           // 0-based byte column on BodyLine
	OpenColumn  int           // 0-based byte column of '(' on Line
	CloseLine   int           // 1-based file line of the matching ')'
	CloseColumn int           // 0-based byte column one past the matching ')' on CloseLine
	Segments    []descSegment // body byte-range → file line/column, one entry per body line
}

// extractInlineAsides walks file content and returns every aside whose
// contents pass ParseHeader. Both single-line `(Name: body)` and
// multi-line `(Name:\n\nbody\n)` forms are accepted — the only difference
// is whether the body fits on one line. Segment mappings let the merge
// phase translate directive spans back to file coordinates regardless of
// how many lines the body covers.
func extractInlineAsides(content string) []inlineAsideHit {
	if !strings.ContainsRune(content, '(') {
		return nil
	}
	lineStarts := buildLineStarts(content)

	var hits []inlineAsideHit
	for _, g := range scanAllParenGroups(content) {
		raw := content[g.start+1 : g.end-1]
		header, ok := ParseHeader(strings.TrimSpace(raw))
		if !ok {
			continue
		}
		colon := IndexHeaderColon(raw)
		if colon < 0 {
			continue
		}
		// Body starts after the colon and any leading whitespace —
		// including newlines, so a block aside whose body starts on the
		// next paragraph maps to the first body byte rather than the
		// blank line that separates header from body.
		bodyStart := g.start + 1 + colon + 1
		bodyEnd := g.end - 1
		for bodyStart < bodyEnd && isSpaceOrNewline(content[bodyStart]) {
			bodyStart++
		}
		openLine, openCol := lineColAt(lineStarts, g.start)
		closeLine, closeCol := lineColAt(lineStarts, g.end)
		bodyLine, bodyCol := lineColAt(lineStarts, bodyStart)
		segments := buildBodySegments(header.DescStart, bodyStart, bodyLine, bodyCol, lineStarts)
		hits = append(hits, inlineAsideHit{
			Line:        openLine,
			Header:      header,
			Body:        header.DescStart,
			BodyLine:    bodyLine,
			BodyColumn:  bodyCol,
			OpenColumn:  openCol,
			CloseLine:   closeLine,
			CloseColumn: closeCol,
			Segments:    segments,
		})
	}
	return hits
}

// isSpaceOrNewline matches the bytes that ParseHeader's strings.TrimSpace
// strips from the leading edge of DescStart. Keeping the predicate in sync
// with TrimSpace is what lets bodyStart line up with DescStart's first byte.
func isSpaceOrNewline(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

// buildBodySegments produces a descSegment for each line of body so the
// directive scanner's joined-coordinate spans can be translated back to
// real file lines and columns. For single-line bodies it returns a single
// segment, matching the previous behaviour.
func buildBodySegments(body string, bodyStartByte, bodyLine, bodyColumn int, lineStarts []int) []descSegment {
	segments := []descSegment{{joinedStart: 0, line: bodyLine, column: bodyColumn}}
	for off := 0; off < len(body); off++ {
		if body[off] != '\n' {
			continue
		}
		nextJoined := off + 1
		if nextJoined > len(body) {
			break
		}
		line, col := lineColAt(lineStarts, bodyStartByte+nextJoined)
		segments = append(segments, descSegment{
			joinedStart: nextJoined,
			line:        line,
			column:      col,
		})
	}
	return segments
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
