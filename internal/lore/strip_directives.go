package lore

import (
	"regexp"
	"sort"
	"strings"
)

// stripSpansFromText returns text with every span removed and adjacent
// separators cleaned up. Spans must be in joined-description byte coordinates
// (i.e. before translateSpans has remapped them to file lines/columns).
// Spans may overlap; overlapping ranges are deduplicated.
//
// Cleanup rules:
//   - orphan ';' (directive separators left stranded between strips) are
//     dropped; semicolons inside prose may also be lost, which is an
//     acceptable tradeoff given how rarely prose uses them.
//   - runs of whitespace collapse to a single space.
//   - consecutive periods separated by whitespace collapse to one period.
//   - leading/trailing whitespace is trimmed.
func stripSpansFromText(text string, spans []StateSpan) string {
	if len(spans) == 0 {
		return text
	}
	sorted := make([]StateSpan, len(spans))
	copy(sorted, spans)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartByte < sorted[j].StartByte
	})

	var b strings.Builder
	b.Grow(len(text))
	pos := 0
	for _, sp := range sorted {
		if sp.StartByte < pos {
			continue // overlap; already covered
		}
		b.WriteString(text[pos:sp.StartByte])
		pos = sp.EndByte
	}
	if pos < len(text) {
		b.WriteString(text[pos:])
	}

	return cleanupStrippedText(b.String())
}

// stripDirectivesFromText strips directive event spans and inline-aside
// `(...)` ranges from the description text. Used to render the prose
// without state syntax in hover and similar views. Aside ranges are
// computed by paren-balanced scanning, so the whole `(Subject: body)`
// disappears from the surrounding prose even though only its inner
// directives are tracked as events.
func stripDirectivesFromText(text string, events []StateEvent) string {
	asides := findInlineAsideRanges(text)
	if len(events) == 0 && len(asides) == 0 {
		return text
	}
	spans := make([]StateSpan, 0, len(events)+len(asides))
	for _, ev := range events {
		spans = append(spans, ev.Span)
	}
	spans = append(spans, asides...)
	return stripSpansFromText(text, spans)
}

var (
	strippedSemicolonRe   = regexp.MustCompile(`\s*;\s*`)
	strippedDoubleDotRe   = regexp.MustCompile(`\.(\s*\.)+`)
	strippedSpacesRe      = regexp.MustCompile(` {2,}`)
	strippedLeadingPunct  = regexp.MustCompile(`^[\s.;,]+`)
	strippedTrailingPunct = regexp.MustCompile(`[\s;,]+$`)
)

func cleanupStrippedText(s string) string {
	s = strippedSemicolonRe.ReplaceAllString(s, " ")
	s = strippedDoubleDotRe.ReplaceAllString(s, ".")
	s = strippedSpacesRe.ReplaceAllString(s, " ")
	s = strippedLeadingPunct.ReplaceAllString(s, "")
	s = strippedTrailingPunct.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}
