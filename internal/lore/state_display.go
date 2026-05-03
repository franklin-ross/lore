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
// Tags line first (if any), then fields alphabetically. Items that contain
// separators or `=` are wrapped in double quotes.
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
	return fmt.Sprintf("%s: %s", name, formatFieldValue(v))
}

// FormatFieldValue renders just the value portion of a field (no name).
// Numbers print as integers when whole, otherwise as decimals; text values
// sort alphabetically and quote items containing punctuation.
func FormatFieldValue(v FieldValue) string {
	return formatFieldValue(v)
}

// formatFieldValue renders just the value portion of a field (no name).
func formatFieldValue(v FieldValue) string {
	switch v.Kind {
	case FieldNumeric:
		return formatNumber(v.Number)
	case FieldText:
		items := append([]string(nil), v.Text...)
		sort.Strings(items)
		parts := make([]string, len(items))
		for i, it := range items {
			parts[i] = quoteIfNeeded(it)
		}
		return strings.Join(parts, ", ")
	}
	return "?"
}

// formatTagsLine renders a sorted "+a +b" line for a tag set, or "" if empty.
func formatTagsLine(tags map[string]bool) string {
	if len(tags) == 0 {
		return ""
	}
	names := make([]string, 0, len(tags))
	for t := range tags {
		names = append(names, t)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = "+" + n
	}
	return strings.Join(parts, " ")
}

// FormatStateBlockMerged renders a single state block showing the cursor state
// with "(latest: X)" annotations for values that have since changed. Fields
// that differ between cursor and latest get a `(latest: …)` suffix on their
// line; fields that match are shown without annotation. Tags follow the same
// rule: no annotation when cursor and latest agree.
func FormatStateBlockMerged(curTags, latestTags map[string]bool, curFields, latestFields map[string]FieldValue) string {
	var lines []string

	curTagLine := formatTagsLine(curTags)
	latestTagLine := formatTagsLine(latestTags)
	if curTagLine != "" || latestTagLine != "" {
		if curTagLine == latestTagLine {
			lines = append(lines, curTagLine)
		} else {
			cur := curTagLine
			if cur == "" {
				cur = "(none)"
			}
			lat := latestTagLine
			if lat == "" {
				lat = "(none)"
			}
			lines = append(lines, fmt.Sprintf("%s  (latest: %s)", cur, lat))
		}
	}

	seen := make(map[string]bool)
	var names []string
	for n := range curFields {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	for n := range latestFields {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	sort.Strings(names)

	for _, n := range names {
		curV, hasCur := curFields[n]
		latV, hasLat := latestFields[n]
		same := hasCur && hasLat && fieldValuesEqual(curV, latV)

		var curStr string
		if hasCur {
			curStr = formatField(n, curV)
		} else {
			curStr = fmt.Sprintf("%s: (none)", n)
		}

		if same {
			lines = append(lines, curStr)
			continue
		}

		latStr := "(none)"
		if hasLat {
			latStr = formatFieldValue(latV)
		}
		lines = append(lines, fmt.Sprintf("%s (latest: %s)", curStr, latStr))
	}

	return strings.Join(lines, "\n")
}

func fieldValuesEqual(a, b FieldValue) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case FieldNumeric:
		return a.Number == b.Number
	case FieldText:
		if len(a.Text) != len(b.Text) {
			return false
		}
		ax := append([]string(nil), a.Text...)
		bx := append([]string(nil), b.Text...)
		sort.Strings(ax)
		sort.Strings(bx)
		for i := range ax {
			if ax[i] != bx[i] {
				return false
			}
		}
		return true
	}
	return false
}

// formatNumber renders n as an integer if it has no fractional part,
// otherwise as a decimal.
func formatNumber(n float64) string {
	if n == float64(int64(n)) {
		return strconv.FormatInt(int64(n), 10)
	}
	return strconv.FormatFloat(n, 'g', -1, 64)
}

// quoteIfNeeded wraps item in double quotes if it contains a character
// that would otherwise confuse a reader scanning the rendered list:
// separators (`,`), sentence punctuation (`. ! ? ;`), or `=`. `+` and `-`
// are left unquoted because they appear naturally in compound words.
func quoteIfNeeded(item string) string {
	if strings.ContainsAny(item, ",.!?;=") {
		return `"` + item + `"`
	}
	return item
}
