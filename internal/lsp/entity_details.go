package lsp

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"

	"lore/internal/lore"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// MethodLoreEntityDetails is the LSP request the wiki view invokes to fetch
// everything it needs to render one entity: its identity, resolved state,
// every authored description block with source spans, and the inbound /
// outbound reference graph.
const MethodLoreEntityDetails = "lore/entityDetails"

// EntityDetailsParams identifies the entity to look up. Entity is the
// canonical name, optionally `Name (type)` for disambiguation. TextDocument
// scopes the lookup to one project — required when the same name exists in
// multiple campaigns; the request returns no entity when the URI is set
// but doesn't belong to any known project.
type EntityDetailsParams struct {
	Entity       string                           `json:"entity"`
	TextDocument *protocol.TextDocumentIdentifier `json:"textDocument,omitempty"`
}

// EntityFieldEntry is one row in the resolved fields table. Value is
// already formatted for display (numbers normalised, text-sets joined).
type EntityFieldEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ContextSegment is one chunk of display text with an optional palette
// colour. ColourIndex is -1 for plain text and a non-negative palette
// index for entity-name spans. Concatenating Text across segments
// reproduces the original source string exactly.
type ContextSegment struct {
	Text        string `json:"text"`
	ColourIndex int32  `json:"colourIndex"`
}

// EntityDescriptionBlock is one authored definition of the entity. The
// description prose is shipped as colourised segments so the wiki can
// highlight entity-name mentions inside the body using the same palette
// as the surrounding editor. Location points at the description's first
// line so the client can render a "jump to source" link.
type EntityDescriptionBlock struct {
	Segments  []ContextSegment  `json:"segments"`
	Location  protocol.Location `json:"location"`
	StartLine uint32            `json:"startLine"`
	EndLine   uint32            `json:"endLine"`
}

// EntityRefItem is a single reference occurrence. Segments is the
// trimmed-context preview, already split into entity-name and plain
// chunks for inline colouring.
type EntityRefItem struct {
	Segments []ContextSegment  `json:"segments"`
	Location protocol.Location `json:"location"`
}

// EntityRefGroup buckets references by their related entity. For inbound
// groups Source names the entity whose definition contains the references
// (empty Source == free-text mentions outside any entity definition). For
// outbound groups Source names the entity being mentioned. ColourIndex
// is the related entity's palette index so the wiki view can colour the
// group heading; -1 when unresolved (free text, or related entity not
// found in this project).
type EntityRefGroup struct {
	Source      string          `json:"source"`
	ColourIndex int32           `json:"colourIndex"`
	Refs        []EntityRefItem `json:"refs"`
}

// EntityStateEventItem mirrors lore.StateEvent for the wire: the op as a
// short string, the resolved value, and a Location for the source span.
type EntityStateEventItem struct {
	Op       string            `json:"op"`
	Target   string            `json:"target"`
	Value    string            `json:"value,omitempty"`
	Location protocol.Location `json:"location"`
}

// EntityDetailsResult is the response body. Found is false when the lookup
// missed (no entity by that name in scope); other fields stay zero-valued
// in that case so the client can render an empty-state message.
type EntityDetailsResult struct {
	Found        bool                     `json:"found"`
	Name         string                   `json:"name,omitempty"`
	Type         string                   `json:"type,omitempty"`
	ColourIndex  uint32                   `json:"colourIndex"`
	Aliases      []string                 `json:"aliases,omitempty"`
	Tags         []string                 `json:"tags,omitempty"`
	Fields       []EntityFieldEntry       `json:"fields,omitempty"`
	Descriptions []EntityDescriptionBlock `json:"descriptions,omitempty"`
	InboundRefs  []EntityRefGroup         `json:"inboundRefs,omitempty"`
	OutboundRefs []EntityRefGroup         `json:"outboundRefs,omitempty"`
	StateHistory []EntityStateEventItem   `json:"stateHistory,omitempty"`
}

// entityDetails resolves the entity in scope and assembles the response.
// Lookup goes through World.FindEntity so disambiguation syntax works, and
// AmbiguousError is collapsed to the first match (the caller already chose
// from a list — at the wiki layer we just render something).
func (s *Server) entityDetails(p *EntityDetailsParams) (*EntityDetailsResult, error) {
	ps := s.entityDetailsScope(p)
	if ps == nil {
		return &EntityDetailsResult{}, nil
	}
	ent, err := ps.world().FindEntity(p.Entity)
	if err != nil {
		var amb *lore.AmbiguousError
		if asAmbiguous(err, &amb) && len(amb.Matches) > 0 {
			ent = amb.Matches[0]
		} else {
			return &EntityDetailsResult{}, nil
		}
	}
	return buildEntityDetails(ps, ent), nil
}

// asAmbiguous is a tiny errors.As wrapper kept local so the package
// doesn't have to pull in errors just for one site.
func asAmbiguous(err error, target **lore.AmbiguousError) bool {
	if e, ok := err.(*lore.AmbiguousError); ok {
		*target = e
		return true
	}
	return false
}

// entityDetailsScope picks the project to search. A non-empty TextDocument
// URI pins the lookup to its owning project; an unset URI falls back to
// the first project so command-palette-style "look up X" still works.
func (s *Server) entityDetailsScope(p *EntityDetailsParams) *projectState {
	if p != nil && p.TextDocument != nil && p.TextDocument.URI != "" {
		ps, _ := s.projectForURI(p.TextDocument.URI)
		return ps
	}
	for _, ps := range s.projects {
		return ps
	}
	return nil
}

func buildEntityDetails(ps *projectState, ent *lore.Entity) *EntityDetailsResult {
	out := &EntityDetailsResult{
		Found:       true,
		Name:        ent.Name,
		Type:        ent.Type,
		ColourIndex: entityColourIndex(ent),
		Aliases:     append([]string(nil), ent.Aliases...),
		Tags:        activeTags(ent.Tags),
		Fields:      buildFieldEntries(ent.Fields),
	}
	out.Descriptions = buildDescriptionBlocks(ps, ent)
	out.InboundRefs = buildInboundRefs(ps, ent)
	out.OutboundRefs = buildOutboundRefs(ps, ent)
	out.StateHistory = buildStateHistory(ps, ent)
	return out
}

func buildFieldEntries(fields map[string]lore.FieldValue) []EntityFieldEntry {
	if len(fields) == 0 {
		return nil
	}
	names := make([]string, 0, len(fields))
	for n := range fields {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]EntityFieldEntry, len(names))
	for i, n := range names {
		out[i] = EntityFieldEntry{Name: n, Value: lore.FormatFieldValue(fields[n])}
	}
	return out
}

func buildDescriptionBlocks(ps *projectState, ent *lore.Entity) []EntityDescriptionBlock {
	if len(ent.Descriptions) == 0 {
		return nil
	}
	world := ps.world()
	out := make([]EntityDescriptionBlock, 0, len(ent.Descriptions))
	for _, d := range ent.Descriptions {
		text := d.CleanText
		if text == "" {
			text = d.Text
		}
		out = append(out, EntityDescriptionBlock{
			Segments:  buildContextSegments(world, text),
			Location:  protocol.Location{URI: ps.fileToURI(d.File), Range: lineRange(d.Line)},
			StartLine: uint32(d.Line),
			EndLine:   uint32(d.EndLine),
		})
	}
	return out
}

// buildInboundRefs groups every recorded mention of `ent` by the source
// entity that owns the mention. Refs from outside any entity definition
// (free-text prose) collapse into a single group with empty Source.
// Group order: source entities alphabetically, free text last.
func buildInboundRefs(ps *projectState, ent *lore.Entity) []EntityRefGroup {
	world := ps.world()
	refs := world.GetReferences(ent.Name)
	if len(refs) == 0 {
		return nil
	}
	bySource := make(map[string][]EntityRefItem)
	for _, r := range refs {
		bySource[r.SourceEntity] = append(bySource[r.SourceEntity], EntityRefItem{
			Segments: buildContextSegments(world, r.Context),
			Location: protocol.Location{URI: ps.fileToURI(r.File), Range: lineRange(r.Line)},
		})
	}
	return sortedGroups(world, bySource)
}

// buildOutboundRefs walks the reference index in reverse: every entry
// where SourceEntity equals our entity is a reference *from* this entity
// to the keyed target. Groups by target entity name. Self-references —
// where the entity's own header / aside text mentions its canonical name
// or any of its aliases — are dropped, so the wiki's Mentions section
// shows only refs the entity makes to *other* entities.
func buildOutboundRefs(ps *projectState, ent *lore.Entity) []EntityRefGroup {
	world := ps.world()
	byTarget := make(map[string][]EntityRefItem)
	for target, refs := range world.References {
		if isSameEntityName(ent, target) {
			continue
		}
		for _, r := range refs {
			if r.SourceEntity != ent.Name {
				continue
			}
			byTarget[target] = append(byTarget[target], EntityRefItem{
				Segments: buildContextSegments(world, r.Context),
				Location: protocol.Location{URI: ps.fileToURI(r.File), Range: lineRange(r.Line)},
			})
		}
	}
	if len(byTarget) == 0 {
		return nil
	}
	return sortedGroups(ps.world(), byTarget)
}

// sortedGroups turns a name→refs map into a deterministic slice. Empty
// keys ("free text") sort to the end so populated source/target buckets
// lead the list. ColourIndex resolves the related entity's palette slot
// (-1 if the name doesn't match any entity in the project, e.g. free text).
func sortedGroups(world *lore.World, m map[string][]EntityRefItem) []EntityRefGroup {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if (names[i] == "") != (names[j] == "") {
			return names[i] != ""
		}
		return names[i] < names[j]
	})
	out := make([]EntityRefGroup, len(names))
	for i, n := range names {
		out[i] = EntityRefGroup{
			Source:      n,
			ColourIndex: lookupColourIndex(world, n),
			Refs:        m[n],
		}
	}
	return out
}

// lookupColourIndex finds entity `name` in `world` and returns its palette
// index, or -1 if no entity owns that name.
func lookupColourIndex(world *lore.World, name string) int32 {
	if name == "" {
		return -1
	}
	ent, err := world.FindEntity(name)
	if err != nil || ent == nil {
		return -1
	}
	return int32(entityColourIndex(ent))
}

func buildStateHistory(ps *projectState, ent *lore.Entity) []EntityStateEventItem {
	if len(ent.StateHistory) == 0 {
		return nil
	}
	out := make([]EntityStateEventItem, 0, len(ent.StateHistory))
	for _, ev := range ent.StateHistory {
		item := EntityStateEventItem{
			Op:     stateOpName(ev.Op),
			Target: ev.Target,
			Location: protocol.Location{
				URI:   ps.fileToURI(ev.Span.File),
				Range: lineRange(ev.Span.Line),
			},
		}
		if ev.Value != nil {
			item.Value = lore.FormatFieldValue(*ev.Value)
		}
		out = append(out, item)
	}
	return out
}

// isSameEntityName reports whether `name` matches the entity's canonical
// name or any of its aliases (case-insensitive). Used to filter
// self-references out of the outbound list.
func isSameEntityName(ent *lore.Entity, name string) bool {
	if name == "" {
		return false
	}
	if ent.Name == name {
		return true
	}
	return slices.Contains(ent.Aliases, name)
}

func stateOpName(op lore.StateOp) string {
	switch op {
	case lore.StateOpAdd:
		return "add"
	case lore.StateOpRemove:
		return "remove"
	case lore.StateOpSet:
		return "set"
	case lore.StateOpIncrement:
		return "increment"
	}
	return "unknown"
}

// buildContextSegments splits `text` into alternating plain and
// entity-coloured chunks for the wiki webview. Matches use the same
// global containment dedup as findReferences (longer canonical name
// suppresses sub-spans, alias inside name collapses), so the highlights
// agree with what shows up in the buffer's semantic tokens.
//
// Returns a single plain segment when the world has no match index or
// the text contains no entity mentions, so the caller never has to
// guard against an empty slice.
func buildContextSegments(world *lore.World, text string) []ContextSegment {
	if text == "" {
		return nil
	}
	if world == nil || world.Match == nil || len(world.Match.Entities) == 0 {
		return []ContextSegment{{Text: text, ColourIndex: -1}}
	}
	lower := strings.ToLower(text)

	type cand struct {
		start, end int
		entityIdx  int
	}
	var all []cand
	for i := range world.Match.Entities {
		em := &world.Match.Entities[i]
		if em.LowerName != "" {
			for _, p := range lore.FindWordMatches(lower, em.LowerName) {
				all = append(all, cand{p, p + len(em.LowerName), i})
			}
		}
		for _, la := range em.LowerAliases {
			for _, p := range lore.FindWordMatches(lower, la) {
				all = append(all, cand{p, p + len(la), i})
			}
		}
	}
	if len(all) == 0 {
		return []ContextSegment{{Text: text, ColourIndex: -1}}
	}

	sort.Slice(all, func(a, b int) bool {
		if all[a].start != all[b].start {
			return all[a].start < all[b].start
		}
		if (all[a].end - all[a].start) != (all[b].end - all[b].start) {
			return (all[a].end - all[a].start) > (all[b].end - all[b].start)
		}
		return all[a].entityIdx < all[b].entityIdx
	})
	kept := all[:0]
	for _, m := range all {
		skip := false
		for _, k := range kept {
			if k.start > m.start || m.end > k.end {
				continue
			}
			equal := k.start == m.start && k.end == m.end
			if equal {
				if k.entityIdx == m.entityIdx {
					skip = true
					break
				}
				continue
			}
			skip = true
			break
		}
		if !skip {
			kept = append(kept, m)
		}
	}
	// Re-sort kept by start ascending — needed because the dedup above
	// preserved sort-by-start order but the equal-span case could have
	// kept multiple entries at the same position; emitting in start
	// order guarantees the output reads left-to-right.
	sort.Slice(kept, func(i, j int) bool { return kept[i].start < kept[j].start })

	out := make([]ContextSegment, 0, len(kept)*2+1)
	prev := 0
	for _, k := range kept {
		if k.start > prev {
			out = append(out, ContextSegment{Text: text[prev:k.start], ColourIndex: -1})
		}
		colour := int32(entityColourIndex(&world.Entities[k.entityIdx]))
		out = append(out, ContextSegment{Text: text[k.start:k.end], ColourIndex: colour})
		prev = k.end
	}
	if prev < len(text) {
		out = append(out, ContextSegment{Text: text[prev:], ColourIndex: -1})
	}
	return out
}

// decodeEntityDetails unmarshals the request payload. Used by loreHandler.
func decodeEntityDetails(raw json.RawMessage) (*EntityDetailsParams, error) {
	if len(raw) == 0 {
		return &EntityDetailsParams{}, nil
	}
	p := &EntityDetailsParams{}
	if err := json.Unmarshal(raw, p); err != nil {
		return nil, err
	}
	return p, nil
}
