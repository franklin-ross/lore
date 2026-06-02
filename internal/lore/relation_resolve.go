package lore

import (
	"fmt"
	"sort"
	"strings"
)

// RelationItem is one resolved relation on an entity: the other endpoint and
// the surface label the focus entity's side uses. Incoming marks a generic
// (undefined) edge seen from its object side, which renders as a named-incoming
// edge ("Other → surface") because the vocabulary has no reciprocal for it.
type RelationItem struct {
	Other    string
	Surface  string
	Incoming bool
}

// RelationGroup is the focus entity's relations under one canonical relation,
// ready for display grouping (adaptive header, deviation annotations).
type RelationGroup struct {
	Canonical string
	Items     []RelationItem
}

// relEdgeKey identifies an edge independent of which side or label declared it.
// For an asymmetric relation it is the primary (lexicographically smaller)
// canonical name plus the primary subject/object identity keys; for a symmetric
// relation the two endpoints are sorted; for a generic/directed edge it is the
// directed (relation, subject, object) triple.
type relEdgeKey struct {
	rel string
	a   string
	b   string
}

type relEdge struct {
	rel       string
	aName     string
	aType     string
	bName     string
	bType     string
	symmetric bool
	directed  bool
	labelA    string // surface label for endpoint A's side ("" = canonical)
	labelB    string // surface label for endpoint B's side ("" = canonical)
	present   bool
}

type edgeDecl struct {
	subjKey  string
	subjName string
	subjType string
	surface  string
	target   string
	op       StateOp
	span     StateSpan
}

// ResolveRelations folds every edge across the world into the focus entity's
// resolved relations, using the latest state (no cursor cutoff).
func (w *World) ResolveRelations(vocab *RelationVocab, focus *Entity) []RelationGroup {
	return w.ResolveRelationsAt(vocab, focus, w.FileOrder, "", 0)
}

// ResolveRelationsAt folds edge events from every entity in file order up to
// and including the cursor position (cursorFile, cursorLine), then returns the
// focus entity's surviving relations grouped by canonical relation. An empty
// cursorFile folds every event (latest state). The fold mirrors ResolveStateAt
// so relations resolve on the same timeline as tags and fields.
func (w *World) ResolveRelationsAt(vocab *RelationVocab, focus *Entity, fileOrder func(string) int, cursorFile string, cursorLine int) []RelationGroup {
	return w.projectFocus(vocab, focus, w.resolveEdgeMap(vocab, fileOrder, cursorFile, cursorLine))
}

// resolveEdgeMap folds every edge declaration into the resolved edge set,
// applying the cursor cutoff when cursorFile is non-empty.
func (w *World) resolveEdgeMap(vocab *RelationVocab, fileOrder func(string) int, cursorFile string, cursorLine int) map[relEdgeKey]*relEdge {
	decls := w.collectEdgeDecls()
	sort.SliceStable(decls, func(i, j int) bool {
		return spanBefore(decls[i].span, decls[j].span, fileOrder)
	})

	edges := make(map[relEdgeKey]*relEdge)
	for _, d := range decls {
		if cursorFile != "" && !spanAtOrBeforeCursor(d.span, fileOrder, cursorFile, cursorLine) {
			continue
		}
		key, e, slot := w.placeEdge(vocab, d)
		if key == (relEdgeKey{}) {
			continue
		}
		cur, ok := edges[key]
		if !ok {
			cur = e
			edges[key] = cur
		}
		switch d.op {
		case StateOpEdgeAdd:
			cur.present = true
			if slot == 0 {
				cur.labelA = d.surface
			} else {
				cur.labelB = d.surface
			}
		case StateOpEdgeRemove:
			cur.present = false
		}
	}
	return edges
}

// EdgeRemovalIssues folds every edge declaration in source order and returns a
// warning for each `-/>` that removes an edge which was never set — the
// relation analogue of "remove a tag that isn't set". Reciprocity is honoured:
// a removal phrased from the reciprocal label still cancels the canonical edge,
// so it doesn't warn.
func (w *World) EdgeRemovalIssues(vocab *RelationVocab) []StateIssue {
	if vocab == nil {
		return nil
	}
	decls := w.collectEdgeDecls()
	sort.SliceStable(decls, func(i, j int) bool {
		return spanBefore(decls[i].span, decls[j].span, w.FileOrder)
	})
	present := make(map[relEdgeKey]bool)
	var issues []StateIssue
	for _, d := range decls {
		key, _, _ := w.placeEdge(vocab, d)
		if key == (relEdgeKey{}) {
			continue
		}
		switch d.op {
		case StateOpEdgeAdd:
			present[key] = true
		case StateOpEdgeRemove:
			if !present[key] {
				issues = append(issues, StateIssue{
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("no %q relation to %q to remove", d.surface, d.target),
					Span:     d.span,
				})
			}
			present[key] = false
		}
	}
	return issues
}

// GraphRelation is a resolved relationship for graph rendering: a labelled,
// directed edge between two entities in canonical orientation. Label is the
// source entity's term for the target (its surface label, or the canonical
// relation when that side wasn't named). Symmetric marks relations whose
// direction isn't meaningful (spouse, sibling, friend).
type GraphRelation struct {
	FromName string
	FromType string
	ToName   string
	ToType   string
	Label    string
	Symmetric bool
}

// ResolveAllRelations returns every resolved relationship in the world (latest
// state), one per canonical edge, for the relation graph.
func (w *World) ResolveAllRelations(vocab *RelationVocab) []GraphRelation {
	edges := w.resolveEdgeMap(vocab, w.FileOrder, "", 0)
	out := make([]GraphRelation, 0, len(edges))
	for _, e := range edges {
		if !e.present {
			continue
		}
		label := e.labelA
		if label == "" {
			label = vocab.Display(e.rel)
		}
		out = append(out, GraphRelation{
			FromName:  e.aName,
			FromType:  e.aType,
			ToName:    e.bName,
			ToType:    e.bType,
			Label:     label,
			Symmetric: e.symmetric,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].FromName != out[j].FromName {
			return out[i].FromName < out[j].FromName
		}
		if out[i].ToName != out[j].ToName {
			return out[i].ToName < out[j].ToName
		}
		return out[i].Label < out[j].Label
	})
	return out
}

// collectEdgeDecls flattens every entity's edge events into declarations whose
// subject is the owning entity. One declaration is produced per target.
func (w *World) collectEdgeDecls() []edgeDecl {
	var decls []edgeDecl
	for i := range w.Entities {
		ent := &w.Entities[i]
		subjKey := entityIdentityKey(ent.Name, ent.Type)
		for _, ev := range ent.StateHistory {
			if ev.Op != StateOpEdgeAdd && ev.Op != StateOpEdgeRemove {
				continue
			}
			if ev.Value == nil {
				continue
			}
			for _, t := range ev.Value.Text {
				decls = append(decls, edgeDecl{
					subjKey:  subjKey,
					subjName: ent.Name,
					subjType: ent.Type,
					surface:  ev.Target,
					target:   t,
					op:       ev.Op,
					span:     ev.Span,
				})
			}
		}
	}
	return decls
}

// placeEdge computes the canonical key for a declaration and returns a freshly
// initialised relEdge plus the slot (0 = endpoint A, 1 = endpoint B) the
// declaration's surface label belongs to. The returned relEdge is only used
// when the key is new to the map.
func (w *World) placeEdge(vocab *RelationVocab, d edgeDecl) (relEdgeKey, *relEdge, int) {
	canon, known := vocab.Resolve(d.surface)
	if canon == "" {
		return relEdgeKey{}, nil, 0
	}
	tgtKey, tgtName, tgtType := w.resolveTargetIdentity(d.target)

	recip := ""
	if known {
		recip = vocab.Reciprocal(canon)
	}

	switch {
	case !known || recip == "":
		// Generic or no-reciprocal: directed edge, subject -> object.
		key := relEdgeKey{rel: canon, a: d.subjKey, b: tgtKey}
		return key, &relEdge{rel: canon, aName: d.subjName, aType: d.subjType, bName: tgtName, bType: tgtType, directed: true}, 0

	case recip == canon:
		// Symmetric: order endpoints so both declarations converge.
		aKey, aName, aType := d.subjKey, d.subjName, d.subjType
		bKey, bName, bType := tgtKey, tgtName, tgtType
		slot := 0
		if aKey > bKey {
			aKey, aName, aType, bKey, bName, bType = bKey, bName, bType, aKey, aName, aType
			slot = 1
		}
		key := relEdgeKey{rel: canon, a: aKey, b: bKey}
		return key, &relEdge{rel: canon, aName: aName, aType: aType, bName: bName, bType: bType, symmetric: true}, slot

	default:
		// Asymmetric: orient by the primary (smaller) canonical name. A is the
		// primary subject, B the primary object; the declared surface labels
		// whichever side the subject occupies in that orientation.
		primary := min(canon, recip)
		if canon == primary {
			key := relEdgeKey{rel: primary, a: d.subjKey, b: tgtKey}
			return key, &relEdge{rel: primary, aName: d.subjName, aType: d.subjType, bName: tgtName, bType: tgtType}, 0
		}
		// Reciprocal orientation: target is the primary subject, subject the
		// primary object, so the surface labels the object (B) side.
		key := relEdgeKey{rel: primary, a: tgtKey, b: d.subjKey}
		return key, &relEdge{rel: primary, aName: tgtName, aType: tgtType, bName: d.subjName, bType: d.subjType}, 1
	}
}

// projectFocus turns the resolved edge set into the focus entity's grouped
// relations.
func (w *World) projectFocus(vocab *RelationVocab, focus *Entity, edges map[relEdgeKey]*relEdge) []RelationGroup {
	focusKey := entityIdentityKey(focus.Name, focus.Type)
	byCanon := make(map[string][]RelationItem)

	addItem := func(canon string, item RelationItem) {
		byCanon[canon] = append(byCanon[canon], item)
	}

	for key, e := range edges {
		if !e.present {
			continue
		}
		switch {
		case e.directed:
			if key.a == focusKey {
				surface := e.labelA
				if surface == "" {
					surface = e.rel
				}
				addItem(e.rel, RelationItem{Other: e.bName, Surface: surface})
			}
			if key.b == focusKey {
				// Object side of a generic edge: named-incoming.
				addItem(e.rel, RelationItem{Other: e.aName, Surface: e.labelA, Incoming: true})
			}
		case e.symmetric:
			if key.a == focusKey {
				addItem(e.rel, RelationItem{Other: e.bName, Surface: orDefault(e.labelA, vocab.Display(e.rel))})
			}
			if key.b == focusKey {
				addItem(e.rel, RelationItem{Other: e.aName, Surface: orDefault(e.labelB, vocab.Display(e.rel))})
			}
		default: // asymmetric
			if key.a == focusKey {
				addItem(e.rel, RelationItem{Other: e.bName, Surface: orDefault(e.labelA, vocab.Display(e.rel))})
			}
			if key.b == focusKey {
				recip := vocab.Reciprocal(e.rel)
				addItem(recip, RelationItem{Other: e.aName, Surface: orDefault(e.labelB, vocab.Display(recip))})
			}
		}
	}

	canons := make([]string, 0, len(byCanon))
	for c := range byCanon {
		canons = append(canons, c)
	}
	sort.Strings(canons)

	groups := make([]RelationGroup, 0, len(canons))
	for _, c := range canons {
		items := byCanon[c]
		sort.SliceStable(items, func(i, j int) bool { return items[i].Other < items[j].Other })
		groups = append(groups, RelationGroup{Canonical: c, Items: items})
	}
	return groups
}

// resolveTargetIdentity resolves a raw target reference to a canonical entity
// identity key and display name. Unresolved or ambiguous targets fall back to
// the raw text so a relation to a not-yet-defined entity still renders.
func (w *World) resolveTargetIdentity(raw string) (key, name, typ string) {
	if ent, err := w.FindEntity(strings.TrimSpace(raw)); err == nil {
		return entityIdentityKey(ent.Name, ent.Type), ent.Name, ent.Type
	}
	trimmed := strings.TrimSpace(raw)
	return "raw:" + strings.ToLower(trimmed), trimmed, ""
}

// entityIdentityKey builds a stable identity key for an entity, including type
// so two entities sharing a bare name don't collapse into one edge endpoint.
func entityIdentityKey(name, typ string) string {
	return strings.ToLower(name) + "\x00" + strings.ToLower(typ)
}

func orDefault(surface, fallback string) string {
	if surface == "" {
		return fallback
	}
	return surface
}

// spanBefore orders two spans by file order then line then byte, matching the
// fold order Merge uses for state.
func spanBefore(a, b StateSpan, fileOrder func(string) int) bool {
	ai, bi := orderIndex(a.File, fileOrder), orderIndex(b.File, fileOrder)
	if ai != bi {
		return ai < bi
	}
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.StartByte < b.StartByte
}

// spanAtOrBeforeCursor reports whether span falls at or before the cursor in
// file-then-line order, mirroring the cutoff in ResolveStateAt.
func spanAtOrBeforeCursor(span StateSpan, fileOrder func(string) int, cursorFile string, cursorLine int) bool {
	cursorIdx := -1
	if fileOrder != nil {
		cursorIdx = fileOrder(cursorFile)
	}
	if fileOrder == nil || cursorIdx < 0 {
		if span.File < cursorFile {
			return true
		}
		return span.File == cursorFile && span.Line <= cursorLine
	}
	evIdx := fileOrder(span.File)
	if evIdx < 0 || evIdx < cursorIdx {
		return true
	}
	return span.File == cursorFile && span.Line <= cursorLine
}

func orderIndex(file string, fileOrder func(string) int) int {
	if fileOrder == nil {
		return 0
	}
	if i := fileOrder(file); i >= 0 {
		return i
	}
	return -1
}
