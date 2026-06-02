package lore

import (
	"fmt"
	"sort"
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
		lines = append(lines, fmt.Sprintf("%s → %s", g.Header, outItemsString(g.Items)))
	}
	for _, it := range r.Incoming {
		lines = append(lines, fmt.Sprintf("%s → %s", it.Other, it.Label))
	}
	return strings.Join(lines, "\n")
}

// outItemsString renders a group's outgoing items as a comma-joined list,
// appending each item's deviation annotation in parentheses.
func outItemsString(items []RelationOutItem) string {
	parts := make([]string, len(items))
	for i, it := range items {
		if it.Annotation != "" {
			parts[i] = fmt.Sprintf("%s (%s)", it.Other, it.Annotation)
		} else {
			parts[i] = it.Other
		}
	}
	return strings.Join(parts, ", ")
}

// FormatRelationsBlockMerged renders relations resolved at the cursor as the
// primary view, annotating each canonical group whose membership differs at
// latest with a "(latest: …)" suffix — the relation analogue of
// FormatStateBlockMerged. A group present only at latest shows "(none)" on the
// cursor side; a generic incoming edge added or removed since the cursor is
// tagged "(latest: added)" / "(latest: removed)".
func FormatRelationsBlockMerged(cur, latest []RelationGroup, vocab *RelationVocab) string {
	curByCanon := indexGroups(cur)
	latByCanon := indexGroups(latest)

	canons := unionKeys(curByCanon, latByCanon)
	var lines []string
	for _, canon := range canons {
		curOut, curIn := splitItems(curByCanon[canon])
		latOut, latIn := splitItems(latByCanon[canon])

		if relItemsEqual(curOut, latOut) {
			if len(curOut) > 0 {
				g := buildOutGroup(canon, curOut, vocab)
				lines = append(lines, fmt.Sprintf("%s → %s", g.Header, outItemsString(g.Items)))
			}
		} else {
			header, curStr := vocab.Display(canon), "(none)"
			if len(curOut) > 0 {
				g := buildOutGroup(canon, curOut, vocab)
				header, curStr = g.Header, outItemsString(g.Items)
			}
			latStr := "(none)"
			if len(latOut) > 0 {
				latStr = outItemsString(buildOutGroup(canon, latOut, vocab).Items)
			}
			lines = append(lines, fmt.Sprintf("%s → %s  (latest: %s)", header, curStr, latStr))
		}

		lines = append(lines, mergedIncomingLines(curIn, latIn)...)
	}
	return strings.Join(lines, "\n")
}

// mergedIncomingLines renders generic incoming edges for one canonical, marking
// those that exist only at the cursor (removed since) or only at latest (added
// since).
func mergedIncomingLines(curIn, latIn []RelationItem) []string {
	type key struct{ other, label string }
	cur := make(map[key]bool, len(curIn))
	for _, it := range curIn {
		cur[key{it.Other, it.Surface}] = true
	}
	lat := make(map[key]bool, len(latIn))
	for _, it := range latIn {
		lat[key{it.Other, it.Surface}] = true
	}
	seen := make(map[key]bool)
	var keys []key
	for k := range cur {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for k := range lat {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].other != keys[j].other {
			return keys[i].other < keys[j].other
		}
		return keys[i].label < keys[j].label
	})

	var lines []string
	for _, k := range keys {
		switch {
		case cur[k] && lat[k]:
			lines = append(lines, fmt.Sprintf("%s → %s", k.other, k.label))
		case cur[k]:
			lines = append(lines, fmt.Sprintf("%s → %s  (latest: removed)", k.other, k.label))
		default:
			lines = append(lines, fmt.Sprintf("%s → %s  (latest: added)", k.other, k.label))
		}
	}
	return lines
}

// indexGroups maps each canonical relation to its items.
func indexGroups(groups []RelationGroup) map[string][]RelationItem {
	m := make(map[string][]RelationItem, len(groups))
	for _, g := range groups {
		m[g.Canonical] = g.Items
	}
	return m
}

// unionKeys returns the sorted union of two maps' keys.
func unionKeys(a, b map[string][]RelationItem) []string {
	seen := make(map[string]bool)
	var keys []string
	for k := range a {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for k := range b {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// splitItems partitions a group's items into outgoing and generic-incoming.
func splitItems(items []RelationItem) (out, in []RelationItem) {
	for _, it := range items {
		if it.Incoming {
			in = append(in, it)
		} else {
			out = append(out, it)
		}
	}
	return out, in
}

// relItemsEqual reports whether two item slices hold the same set of
// (Other, Surface) pairs, ignoring order.
func relItemsEqual(a, b []RelationItem) bool {
	if len(a) != len(b) {
		return false
	}
	type key struct{ other, surface string }
	counts := make(map[key]int, len(a))
	for _, it := range a {
		counts[key{it.Other, it.Surface}]++
	}
	for _, it := range b {
		counts[key{it.Other, it.Surface}]--
	}
	for _, n := range counts {
		if n != 0 {
			return false
		}
	}
	return true
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
	// The surface may already be a plural the author typed — the canonical's
	// own plural ("children"), or any s-ending ("members"). Leave those as-is
	// rather than doubling them. Other endings are unambiguous singulars, so
	// the regular rules apply.
	if canonKey(surface) == canonKey(vocab.Plural(canonical)) {
		return surface
	}
	if strings.HasSuffix(surface, "s") {
		return surface
	}
	return pluralise(surface)
}
