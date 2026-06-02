package lore

import (
	"fmt"
	"strings"
)

// FormatRelationsBlock renders an entity's resolved relations as plain text,
// one line per canonical relation. Returns "" when there are no relations.
//
// Outgoing edges group by canonical relation with an adaptive header: a shared
// surface label (pluralised) when every item agrees, otherwise the canonical
// plural with per-item annotations on the deviating labels. Generic incoming
// edges render as named-incoming lines ("Other → label").
//
//	children → Mary (daughter), Tim
//	spouse → Arwen
//	Sarah → bestie
func FormatRelationsBlock(groups []RelationGroup, vocab *RelationVocab) string {
	var lines []string
	for _, g := range groups {
		var outgoing []RelationItem
		var incoming []RelationItem
		for _, it := range g.Items {
			if it.Incoming {
				incoming = append(incoming, it)
			} else {
				outgoing = append(outgoing, it)
			}
		}
		if len(outgoing) > 0 {
			lines = append(lines, formatOutgoingGroup(g.Canonical, outgoing, vocab))
		}
		for _, it := range incoming {
			lines = append(lines, fmt.Sprintf("%s → %s", it.Other, it.Surface))
		}
	}
	return strings.Join(lines, "\n")
}

// formatOutgoingGroup renders one canonical relation's outgoing items with the
// adaptive header rule.
func formatOutgoingGroup(canonical string, items []RelationItem, vocab *RelationVocab) string {
	shared := items[0].Surface
	allShared := true
	for _, it := range items[1:] {
		if it.Surface != shared {
			allShared = false
			break
		}
	}

	var header string
	switch {
	case len(items) == 1:
		header = items[0].Surface
	case allShared:
		header = pluraliseSurface(shared, canonical, vocab)
	default:
		header = vocab.Plural(canonical)
	}

	annotate := len(items) > 1 && !allShared
	parts := make([]string, len(items))
	for i, it := range items {
		if annotate && canonKey(it.Surface) != canonical {
			parts[i] = fmt.Sprintf("%s (%s)", it.Other, it.Surface)
		} else {
			parts[i] = it.Other
		}
	}
	return fmt.Sprintf("%s → %s", header, strings.Join(parts, ", "))
}

// pluraliseSurface renders the header for a group whose items all share one
// surface label. The canonical relation uses its configurable plural; a shared
// alias gets a trailing "s" unless it already ends in one — so "daughter"
// becomes "daughters" but an already-plural alias like "members" is left as-is
// rather than doubling to "memberss".
func pluraliseSurface(surface, canonical string, vocab *RelationVocab) string {
	if canonKey(surface) == canonical {
		return vocab.Plural(canonical)
	}
	if strings.HasSuffix(surface, "s") {
		return surface
	}
	return surface + "s"
}
