package lore

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Entity is a named thing in the world: a character, location, quest, etc.
type Entity struct {
	Name         string
	Type         string // non-empty for any entity reachable via a World returned by Merge
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
// text, in source order. LexIssues holds any diagnostics produced while
// parsing directives from the description.
type Description struct {
	Text      string
	File      string
	Line      int
	Events    []StateEvent
	LexIssues []StateIssue
}

// Reference records a mention of an entity in a file.
type Reference struct {
	File         string
	Line         int
	SourceEntity string // name of the entity whose definition contains this ref; empty for free text
	Context      string // the line of text containing the reference
}

// SearchResult is a match from a full-text search across entity descriptions.
type SearchResult struct {
	File    string
	Line    int
	Context string
}

// Issue is a diagnostic reported by the check command.
type Issue struct {
	File    string
	Line    int
	Message string
}

// Disambiguation holds a parsed "Name (type)" lookup string.
type Disambiguation struct {
	Name string
	Type string
}

// ParseDisambiguation extracts name and type from "Name (type)" syntax.
// Returns ok=false if the input doesn't match the pattern.
func ParseDisambiguation(input string) (Disambiguation, bool) {
	open := strings.LastIndex(input, "(")
	if open < 0 {
		return Disambiguation{}, false
	}
	close := strings.LastIndex(input, ")")
	if close <= open {
		return Disambiguation{}, false
	}
	if strings.TrimSpace(input[close+1:]) != "" {
		return Disambiguation{}, false
	}
	name := strings.TrimSpace(input[:open])
	typ := strings.TrimSpace(input[open+1 : close])
	if name == "" || typ == "" {
		return Disambiguation{}, false
	}
	return Disambiguation{Name: name, Type: typ}, true
}

// ContainsIgnoreCase reports whether haystack contains needle, ignoring case.
func ContainsIgnoreCase(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

// FindWordMatches returns the byte offsets at which needle appears in
// haystack as a standalone word. BOTH arguments must already be lowercased;
// callers that scan many needles against the same haystack (or vice versa)
// should ToLower each value once and reuse the result instead of paying for
// repeated allocations inside this function.
//
// Word boundaries are the string edges or any character that isn't a letter,
// digit, or underscore — so "pip" won't match inside "piping", but will
// match in "a pip-sized dog".
func FindWordMatches(lowerHaystack, lowerNeedle string) []int {
	if lowerNeedle == "" {
		return nil
	}
	var out []int
	start := 0
	for {
		idx := strings.Index(lowerHaystack[start:], lowerNeedle)
		if idx < 0 {
			return out
		}
		pos := start + idx
		end := pos + len(lowerNeedle)
		if isWordBoundaryBefore(lowerHaystack, pos) && isWordBoundaryAfter(lowerHaystack, end) {
			out = append(out, pos)
		}
		start = pos + 1
	}
}

// HasWordMatch reports whether needle appears in haystack as a standalone
// word. Like FindWordMatches, both arguments must already be lowercased.
func HasWordMatch(lowerHaystack, lowerNeedle string) bool {
	return len(FindWordMatches(lowerHaystack, lowerNeedle)) > 0
}

func isWordBoundaryBefore(s string, pos int) bool {
	if pos == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(s[:pos])
	return !isWordRune(r)
}

func isWordBoundaryAfter(s string, pos int) bool {
	if pos >= len(s) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s[pos:])
	return !isWordRune(r)
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// NameMatchesAlias reports whether any of the entity's aliases match name (case-insensitive).
func (e *Entity) NameMatchesAlias(name string) bool {
	for _, alias := range e.Aliases {
		if strings.EqualFold(alias, name) {
			return true
		}
	}
	return false
}

// HasAlias reports whether the alias already exists (case-insensitive).
func (e *Entity) HasAlias(alias string) bool {
	for _, a := range e.Aliases {
		if strings.EqualFold(a, alias) {
			return true
		}
	}
	return false
}
