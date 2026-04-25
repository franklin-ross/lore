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
//
// The colon split happens at paren depth zero so a line like
// `We saw her (Mad Mary (npc): old lady) wave.` reads as prose rather than a
// header — the `:` inside the inline aside doesn't count.
func ParseHeader(line string) (Header, bool) {
	colon := indexHeaderColon(line)
	if colon < 0 {
		return Header{}, false
	}
	headerPart := line[:colon]
	descStart := strings.TrimSpace(line[colon+1:])

	// Typed header: a `(type)` annotation must sit adjacent to a name or alias
	// boundary — the start or end of a `|`-segment. Parens floating in the
	// middle of a segment are prose, not a type, and disqualify the whole line
	// from typed parsing (so a sentence like "We waited (it rained): nobody
	// came" isn't misread as an entity definition).
	if typ, name, aliases, ok := parseTypedHeader(headerPart); ok {
		if name == "" {
			return Header{}, false
		}
		return Header{Name: name, Type: typ, Aliases: aliases, DescStart: descStart}, true
	}

	// Untyped header: the whole part before the colon is the lookup name.
	// We don't split on `|` here — aliases are declared on typed headers only.
	name := strings.TrimSpace(headerPart)
	if name == "" {
		return Header{}, false
	}
	return Header{Name: name, DescStart: descStart}, true
}

// indexHeaderColon returns the byte offset of the first ':' in line that is
// outside any parenthesised group, or -1 if no such colon exists.
func indexHeaderColon(line string) int {
	depth := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// parseTypedHeader walks the `|`-separated segments of a header and pulls out
// a single `(type)` annotation that sits adjacent to a segment edge. Returns
// ok=false if the header has no type annotation, or if any segment contains
// parens that aren't adjacent to its edges.
func parseTypedHeader(headerPart string) (typ, name string, aliases []string, ok bool) {
	segments := strings.Split(headerPart, "|")
	cleaned := make([]string, len(segments))
	for i, seg := range segments {
		t, rest, status := extractEdgeType(seg)
		if status == typeMidSegment {
			return "", "", nil, false
		}
		if status == typeFound {
			if typ != "" {
				// More than one `(type)` annotation isn't allowed.
				return "", "", nil, false
			}
			typ = t
		}
		cleaned[i] = rest
	}
	if typ == "" {
		return "", "", nil, false
	}
	for _, seg := range cleaned {
		trimmed := strings.TrimSpace(seg)
		if trimmed == "" {
			continue
		}
		if name == "" {
			name = trimmed
		} else {
			aliases = append(aliases, trimmed)
		}
	}
	return typ, name, aliases, true
}

type typeStatus int

const (
	typeAbsent typeStatus = iota
	typeFound
	typeMidSegment
)

// extractEdgeType inspects a single `|`-segment for a `(type)` annotation
// adjacent to its leading or trailing edge. It returns the type (if any), the
// segment with the type removed, and a status indicating absent / found /
// mid-segment (which makes the whole header invalid as a typed definition).
func extractEdgeType(segment string) (string, string, typeStatus) {
	trimmed := strings.TrimSpace(segment)
	open := strings.Index(trimmed, "(")
	if open < 0 {
		return "", segment, typeAbsent
	}
	close := strings.Index(trimmed, ")")
	if close < 0 || close < open {
		return "", segment, typeMidSegment
	}
	// Reject extra parens — only one annotation per segment.
	if strings.Count(trimmed, "(") > 1 || strings.Count(trimmed, ")") > 1 {
		return "", segment, typeMidSegment
	}
	typ := strings.TrimSpace(trimmed[open+1 : close])
	if typ == "" {
		return "", segment, typeMidSegment
	}
	atStart := open == 0
	atEnd := close == len(trimmed)-1
	if !atStart && !atEnd {
		return "", segment, typeMidSegment
	}
	var rest string
	if atStart {
		rest = trimmed[close+1:]
	} else {
		rest = trimmed[:open]
	}
	return typ, rest, typeFound
}
