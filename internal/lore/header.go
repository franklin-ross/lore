package lore

import "strings"

// Header is a parsed entity header line: the `Name (type) | Alias: desc` bit.
// Type is empty when the line has no `(type)` annotation.
type Header struct {
	Name      string
	Type      string
	Aliases   []string
	DescStart string
}

// ParseHeader parses a trimmed header line. Returns ok=false if the line has
// no colon or no non-empty name before it. Both typed and untyped headers are
// accepted; callers decide whether to treat untyped headers as entity
// definitions based on surrounding context.
func ParseHeader(line string) (Header, bool) {
	headerPart, descStart, ok := strings.Cut(line, ":")
	if !ok {
		return Header{}, false
	}
	descStart = strings.TrimSpace(descStart)

	// Typed header: extract (type) from anywhere in the header part.
	if open := strings.Index(headerPart, "("); open >= 0 {
		if rel := strings.Index(headerPart[open:], ")"); rel >= 0 {
			close := open + rel
			typ := strings.TrimSpace(headerPart[open+1 : close])
			if typ != "" {
				before := headerPart[:open]
				after := ""
				if close+1 < len(headerPart) {
					after = headerPart[close+1:]
				}
				name, aliases := splitNameAliases(before, after)
				if name == "" {
					return Header{}, false
				}
				return Header{Name: name, Type: typ, Aliases: aliases, DescStart: descStart}, true
			}
		}
	}

	// Untyped header: the whole part before the colon is the lookup name.
	// We don't split on `|` here — aliases are declared on typed headers only.
	name := strings.TrimSpace(headerPart)
	if name == "" {
		return Header{}, false
	}
	return Header{Name: name, DescStart: descStart}, true
}

// splitNameAliases splits `before | name | alias1 | alias2` (or the pieces
// around a `(type)` annotation) into a canonical name and alias list.
func splitNameAliases(before, after string) (string, []string) {
	var canonical string
	var aliases []string
	collect := func(segment string) {
		trimmed := strings.TrimSpace(segment)
		if trimmed == "" {
			return
		}
		if canonical == "" {
			canonical = trimmed
			return
		}
		aliases = append(aliases, trimmed)
	}
	for _, seg := range strings.Split(before, "|") {
		collect(seg)
	}
	for _, seg := range strings.Split(after, "|") {
		collect(seg)
	}
	return canonical, aliases
}
