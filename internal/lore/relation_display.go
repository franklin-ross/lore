package lore

import (
	"fmt"
	"strings"
)

// RelationRender is a display-ready projection of an entity's resolved
// relations, shared by the plain-text formatter and the structured (wiki)
// renderer so the adaptive-header and annotation rules live in one place.
type RelationRender struct {
	Outgoing []RelationOutGroup
	Incoming []RelationInItem
}

// RelationOutGroup is one canonical relation's outgoing items under an adaptive
// header (a shared surface label, pluralised, or the canonical plural).
type RelationOutGroup struct {
	Header string
	Items  []RelationOutItem
}

// RelationOutItem is one related entity and its deviation annotation — the
// surface label, set only when it differs from the group's canonical.
type RelationOutItem struct {
	Other      string
	Annotation string
}

// RelationInItem is a generic incoming edge: another entity points at this one
// via Label, which has no defined reciprocal to render forward.
type RelationInItem struct {
	Other string
	Label string
}

// BuildRelationRender turns resolved relation groups into a display-ready
// structure: outgoing items grouped under adaptive headers with deviation
// annotations, plus generic incoming edges.
func BuildRelationRender(groups []RelationGroup, vocab *RelationVocab) RelationRender {
	var r RelationRender
	for _, g := range groups {
		var outgoing []RelationItem
		for _, it := range g.Items {
			if it.Incoming {
				r.Incoming = append(r.Incoming, RelationInItem{Other: it.Other, Label: it.Surface})
			} else {
				outgoing = append(outgoing, it)
			}
		}
		if len(outgoing) > 0 {
			r.Outgoing = append(r.Outgoing, buildOutGroup(g.Canonical, outgoing, vocab))
		}
	}
	return r
}

func buildOutGroup(canonical string, items []RelationItem, vocab *RelationVocab) RelationOutGroup {
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
	out := make([]RelationOutItem, len(items))
	for i, it := range items {
		annotation := ""
		if annotate && canonKey(it.Surface) != canonical {
			annotation = it.Surface
		}
		out[i] = RelationOutItem{Other: it.Other, Annotation: annotation}
	}
	return RelationOutGroup{Header: header, Items: out}
}

// FormatRelationsBlock renders an entity's resolved relations as plain text,
// one line per group. Returns "" when there are no relations.
//
//	children → Mary (daughter), Tim
//	spouse → Arwen
//	Sarah → bestie
func FormatRelationsBlock(groups []RelationGroup, vocab *RelationVocab) string {
	r := BuildRelationRender(groups, vocab)
	var lines []string
	for _, g := range r.Outgoing {
		parts := make([]string, len(g.Items))
		for i, it := range g.Items {
			if it.Annotation != "" {
				parts[i] = fmt.Sprintf("%s (%s)", it.Other, it.Annotation)
			} else {
				parts[i] = it.Other
			}
		}
		lines = append(lines, fmt.Sprintf("%s → %s", g.Header, strings.Join(parts, ", ")))
	}
	for _, it := range r.Incoming {
		lines = append(lines, fmt.Sprintf("%s → %s", it.Other, it.Label))
	}
	return strings.Join(lines, "\n")
}

// pluraliseSurface renders the header for a group whose items all share one
// surface label. The canonical relation uses its configurable plural; a shared
// alias follows the regular pluralise rules ("daughter" → "daughters",
// "witch" → "witches"), except an already-plural alias like "members" is left
// as-is rather than doubling to "memberss".
func pluraliseSurface(surface, canonical string, vocab *RelationVocab) string {
	if canonKey(surface) == canonical {
		return vocab.Plural(canonical)
	}
	// A surface alias may already read as plural ("members"); leave any
	// s-ending as-is rather than risk doubling. Other endings are unambiguous
	// singulars, so the regular rules apply.
	if strings.HasSuffix(surface, "s") {
		return surface
	}
	return pluralise(surface)
}
