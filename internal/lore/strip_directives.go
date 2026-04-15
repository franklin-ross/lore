package lore

import (
	"regexp"
	"sort"
	"strings"
)

// stripDirectivesFromText returns text with every event span removed and
// adjacent separators cleaned up. Events must carry spans in joined-description
// byte coordinates (i.e. before translateSpans has remapped them to file
// lines/columns).
//
// Cleanup rules:
//   - orphan ';' (directive separators left stranded between strips) are
//     dropped; semicolons inside prose may also be lost, which is an
//     acceptable tradeoff given how rarely prose uses them.
//   - runs of whitespace collapse to a single space.
//   - consecutive periods separated by whitespace collapse to one period.
//   - leading/trailing whitespace is trimmed.
func stripDirectivesFromText(text string, events []StateEvent) string {
	if len(events) == 0 {
		return text
	}
	sorted := make([]StateEvent, len(events))
	copy(sorted, events)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Span.StartByte < sorted[j].Span.StartByte
	})

	var b strings.Builder
	b.Grow(len(text))
	pos := 0
	for _, ev := range sorted {
		if ev.Span.StartByte < pos {
			continue // overlap; already covered
		}
		b.WriteString(text[pos:ev.Span.StartByte])
		pos = ev.Span.EndByte
	}
	if pos < len(text) {
		b.WriteString(text[pos:])
	}

	return cleanupStrippedText(b.String())
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
