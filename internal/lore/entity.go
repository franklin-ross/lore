package lore

import "strings"

// Entity is a named thing in the world: a character, location, quest, etc.
type Entity struct {
	Name         string
	Type         string // empty if untyped
	Aliases      []string
	Descriptions []Description
}

// Description is a block of prose attached to an entity, with its source location.
type Description struct {
	Text string
	File string
	Line int
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
