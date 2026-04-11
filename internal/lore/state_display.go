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
