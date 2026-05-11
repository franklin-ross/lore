package lsp

import (
	"encoding/json"
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
// index for entity-name spans. Entity is the navigation target for
// coloured spans (the canonical name, with " (type)" appended when the
// bare name is ambiguous, so clicks open the right wiki). Ambiguous
// flags candidates emitted as part of an ambiguous bare-name match so
// the frontend can mark them visually. Concatenating Text across
// non-ambiguous segments reproduces the original source string exactly;
// ambiguous candidates duplicate the same Text once per candidate.
type ContextSegment struct {
	Text        string
	Entity      string // navigation target (empty for plain segments)
	Ambiguous   bool   // true for each candidate of an ambiguous bare-name match
	ColourIndex int32  // -1 = no colour (omitted from JSON)
}

// MarshalJSON writes the segment with colourIndex omitted when the
// sentinel -1 is set, and entity/ambiguous omitted when zero-valued.
func (s ContextSegment) MarshalJSON() ([]byte, error) {
	if s.ColourIndex < 0 {
		return json.Marshal(struct {
			Text string `json:"text"`
		}{s.Text})
	}
	return json.Marshal(struct {
		Text        string `json:"text"`
		Entity      string `json:"entity,omitempty"`
		Ambiguous   bool   `json:"ambiguous,omitempty"`
		ColourIndex int32  `json:"colourIndex"`
	}{s.Text, s.Entity, s.Ambiguous, s.ColourIndex})
}

// EntityDescriptionBlock is one authored definition of the entity. The
// description body is shipped as a markdown tree (Content) so the wiki
// can render block-level constructs — blockquotes, lists, headings,
// code, tables, emphasis — without parsing markdown itself. Entity
// colouring is woven into "text" leaves of the tree, so name mentions
// inside the body still pick up the palette colour. Location points at
// the description's first line so the client can render a "jump to
// source" link.
type EntityDescriptionBlock struct {
	Content   []MarkdownNode    `json:"content"`
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
// (empty Source == free-text mentions outside any entity definition).
// For outbound groups Source names the entity being mentioned. Aliases are
// the resolved entity's aliases (empty for free-text and unresolved
// sources) so the wiki can render them inline next to the heading.
// ColourIndex is -1 when the related name doesn't resolve to an entity
// in this project (free text, or stale name); the custom marshaller
// drops the field from JSON in that case.
type EntityRefGroup struct {
	Source      string
	Aliases     []string
	Tags        []string
	ColourIndex int32 // -1 = no colour (omitted from JSON)
	Refs        []EntityRefItem
}

// MarshalJSON writes the group with colourIndex omitted for free-text
// and unresolved sources.
func (g EntityRefGroup) MarshalJSON() ([]byte, error) {
	if g.ColourIndex < 0 {
		return json.Marshal(struct {
			Source string          `json:"source"`
			Refs   []EntityRefItem `json:"refs"`
		}{g.Source, g.Refs})
	}
	return json.Marshal(struct {
		Source      string          `json:"source"`
		Aliases     []string        `json:"aliases,omitempty"`
		Tags        []string        `json:"tags,omitempty"`
		ColourIndex int32           `json:"colourIndex"`
		Refs        []EntityRefItem `json:"refs"`
	}{g.Source, g.Aliases, g.Tags, g.ColourIndex, g.Refs})
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
	world := ps.world()
	return &EntityDetailsResult{
		Found:        true,
		Name:         ent.Name,
		Type:         ent.Type,
		ColourIndex:  entityColourIndex(ent),
		Aliases:      append([]string(nil), ent.Aliases...),
		Tags:         activeTags(ent.Tags),
		Fields:       buildFieldEntries(ent.Fields),
		Descriptions: buildDescriptionBlocks(ps, world, ent),
		InboundRefs:  buildInboundRefs(ps, world, ent),
		OutboundRefs: buildOutboundRefs(ps, world, ent),
		StateHistory: buildStateHistory(ps, ent),
	}
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

func buildDescriptionBlocks(ps *projectState, world *lore.World, ent *lore.Entity) []EntityDescriptionBlock {
	if len(ent.Descriptions) == 0 {
		return nil
	}
	out := make([]EntityDescriptionBlock, 0, len(ent.Descriptions))
	for _, d := range ent.Descriptions {
		text := d.CleanText
		if text == "" {
			text = d.Text
		}
		// Location targets the entity name span on the definition line so
		// the type-page row click selects the name rather than the whole
		// block. The entity-page description jump button passes only the
		// line, so its behaviour is unchanged. Inline asides start with
		// `(` before the name; skip it so the selection lands on the
		// first letter, not the paren. When a `(type)` suffix follows the
		// name on the same line, extend the selection to cover it so the
		// disambiguator stays visually attached to the name.
		nameStart := d.StartColumn
		if d.IsAside {
			nameStart++
		}
		nameEnd := nameStart + len(ent.Name)
		if ent.Type != "" {
			line := ps.lineText(d.File, d.Line)
			suffix := " (" + ent.Type + ")"
			if nameEnd+len(suffix) <= len(line) && line[nameEnd:nameEnd+len(suffix)] == suffix {
				nameEnd += len(suffix)
			}
		}
		out = append(out, EntityDescriptionBlock{
			Content:   buildDescriptionContent(world, text),
			Location:  ps.locAtMatch(d.File, d.Line, nameStart, nameEnd),
			StartLine: uint32(d.Line),
			EndLine:   uint32(d.EndLine),
		})
	}
	return out
}

// buildInboundRefs groups every recorded mention of `ent` by the source
// entity that owns the mention. Refs from outside any entity definition
// (free-text prose) collapse into a single group with empty Source.
// When the source's bare name is shared by multiple entities the type
// is appended ("Barovia (town)") so each owner gets its own group.
// Group order: source entities alphabetically, free text last.
func buildInboundRefs(ps *projectState, world *lore.World, ent *lore.Entity) []EntityRefGroup {
	refs := world.GetReferences(ent.Name)
	if len(refs) == 0 {
		return nil
	}
	bySource := make(map[string][]EntityRefItem)
	for _, r := range refs {
		// Drop refs whose target is a different entity sharing this bare name.
		if r.TargetType != "" && ent.Type != "" && !strings.EqualFold(r.TargetType, ent.Type) {
			continue
		}
		// Self-reference: this ref's source IS this entity. The header line
		// always re-states the entity's own name so the scanner records a
		// ref against it; surface it under "Mentioned by" would mean every
		// entity claims to mention itself. Skip when the source bare name
		// matches and (when a type is recorded) the types agree, so a
		// different entity sharing only the bare name still appears.
		if strings.EqualFold(r.SourceEntity, ent.Name) {
			if r.SourceType == "" || ent.Type == "" || strings.EqualFold(r.SourceType, ent.Type) {
				continue
			}
		}
		key := entityLabel(world, r.SourceEntity, r.SourceType)
		bySource[key] = append(bySource[key], buildRefItem(ps, world, r))
	}
	if len(bySource) == 0 {
		return nil
	}
	return sortedGroups(world, bySource)
}

// buildOutboundRefs walks the reference index in reverse: every entry
// where SourceEntity equals our entity is a reference *from* this entity
// to the keyed target. Groups by target entity name (with type appended
// when shared). Self-references are dropped so the wiki's Mentions
// section shows only refs the entity makes to *other* entities.
func buildOutboundRefs(ps *projectState, world *lore.World, ent *lore.Entity) []EntityRefGroup {
	byTarget := make(map[string][]EntityRefItem)
	for target, refs := range world.References {
		for _, r := range refs {
			if !strings.EqualFold(r.SourceEntity, ent.Name) {
				continue
			}
			if r.SourceType != "" && ent.Type != "" && !strings.EqualFold(r.SourceType, ent.Type) {
				continue
			}
			// Self-reference: this ref's target IS this entity.
			if strings.EqualFold(target, ent.Name) {
				if r.TargetType == "" || ent.Type == "" || strings.EqualFold(r.TargetType, ent.Type) {
					continue
				}
			}
			key := entityLabel(world, target, r.TargetType)
			byTarget[key] = append(byTarget[key], buildRefItem(ps, world, r))
		}
	}
	if len(byTarget) == 0 {
		return nil
	}
	return sortedGroups(world, byTarget)
}

// entityLabel returns the display label for an entity reference. Type is
// appended ("Name (type)") only when the bare name is shared by more than
// one entity in the world — otherwise the bare name is unambiguous and
// the suffix would be noise.
func entityLabel(world *lore.World, name, typ string) string {
	if name == "" || typ == "" {
		return name
	}
	count := 0
	for i := range world.Entities {
		if strings.EqualFold(world.Entities[i].Name, name) {
			count++
			if count > 1 {
				return name + " (" + typ + ")"
			}
		}
	}
	return name
}

// buildRefItem packages one Reference for the wiki: trims the source
// line to a few words before the match, then colourises the preview so
// entity names inside it appear in their palette colours. Location points
// at the matched substring so clicking the row selects exactly the name
// span rather than the whole line.
func buildRefItem(ps *projectState, world *lore.World, r lore.Reference) EntityRefItem {
	preview := trimContextBeforeMatch(r.Context, r.MatchOffset, 4)
	return EntityRefItem{
		Segments: buildContextSegments(world, preview),
		Location: ps.locAtMatch(r.File, r.Line, r.MatchStart, r.MatchEnd),
	}
}

// sortedGroups turns a name→refs map into a deterministic slice. Empty
// keys ("free text") sort to the end so populated source/target buckets
// lead the list. ColourIndex is non-nil only when the related name
// resolves to a known entity in this project.
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
		idx, aliases, tags := lookupGroupMeta(world, n)
		out[i] = EntityRefGroup{
			Source:      n,
			Aliases:     aliases,
			Tags:        tags,
			ColourIndex: idx,
			Refs:        m[n],
		}
	}
	return out
}

// lookupGroupMeta returns the palette index, aliases, and active tags for
// the entity named `name`. All fall back to empty when no entity owns that
// name (e.g. a free-text group, or a stale label that no longer resolves).
func lookupGroupMeta(world *lore.World, name string) (int32, []string, []string) {
	if name == "" {
		return -1, nil, nil
	}
	ent, err := world.FindEntity(name)
	if err != nil || ent == nil {
		return -1, nil, nil
	}
	return int32(entityColourIndex(ent)), append([]string(nil), ent.Aliases...), activeTags(ent.Tags)
}

func buildStateHistory(ps *projectState, ent *lore.Entity) []EntityStateEventItem {
	if len(ent.StateHistory) == 0 {
		return nil
	}
	out := make([]EntityStateEventItem, 0, len(ent.StateHistory))
	for _, ev := range ent.StateHistory {
		item := EntityStateEventItem{
			Op:       stateOpName(ev.Op),
			Target:   ev.Target,
			Location: locationForStateEvent(ps, ent, ev),
		}
		if ev.Value != nil {
			item.Value = lore.FormatFieldValue(*ev.Value)
		}
		out = append(out, item)
	}
	return out
}

// locationForStateEvent uses the directive's pre-translated Span — the
// merge layer maps joined-description offsets back to absolute file line +
// byte columns via translateSpans, so we go straight from those to a
// precise LSP Range. ent is unused but kept in the signature for future
// callers that want to disambiguate spans by owning description.
func locationForStateEvent(ps *projectState, _ *lore.Entity, ev lore.StateEvent) protocol.Location {
	return ps.locAtMatch(ev.Span.File, ev.Span.Line, ev.Span.StartByte, ev.Span.EndByte)
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
// entity-coloured chunks for the wiki webview. Match resolution is
// delegated to lore.ScanEntities so the highlights agree exactly with
// what the reference scanner records and the colouriser paints.
//
// Equal-span matches (a bare name shared by multiple entities, with no
// `(type)` suffix in the source) are emitted as one segment per
// candidate, each flagged Ambiguous and carrying its disambiguated
// label so a click navigates to that specific entity.
func buildContextSegments(world *lore.World, text string) []ContextSegment {
	if text == "" {
		return nil
	}
	matches := lore.ScanEntities(world, text, false)
	if len(matches) == 0 {
		return []ContextSegment{{Text: text, ColourIndex: -1}}
	}
	out := make([]ContextSegment, 0, len(matches)*2+1)
	prev := 0
	for i := 0; i < len(matches); {
		m := matches[i]
		j := i + 1
		for j < len(matches) && matches[j].Start == m.Start && matches[j].End == m.End {
			j++
		}
		ambiguous := j-i > 1
		if m.Start > prev {
			out = append(out, ContextSegment{Text: text[prev:m.Start], ColourIndex: -1})
		}
		for k := i; k < j; k++ {
			if ambiguous && k > i {
				out = append(out, ContextSegment{Text: "/", ColourIndex: -1})
			}
			ent := &world.Entities[matches[k].EntityIdx]
			idx := int32(entityColourIndex(ent))
			label := ent.Name
			if ambiguous && ent.Type != "" {
				label = ent.Name + " (" + ent.Type + ")"
			}
			out = append(out, ContextSegment{
				Text:        text[m.Start:m.End],
				Entity:      label,
				Ambiguous:   ambiguous,
				ColourIndex: idx,
			})
		}
		prev = m.End
		i = j
	}
	if prev < len(text) {
		out = append(out, ContextSegment{Text: text[prev:], ColourIndex: -1})
	}
	return out
}

// trimContextBeforeMatch returns `text` starting at most `wordsBefore`
// whitespace-separated tokens before `matchOffset`, prefixed with "… "
// when leading bytes were dropped. Several refs in the same long
// sentence would otherwise render as visually identical wiki rows;
// cropping each preview around its match makes them distinguishable.
// Punctuation stays attached to the word it sits beside (only space
// and tab count as separators).
func trimContextBeforeMatch(text string, matchOffset, wordsBefore int) string {
	if matchOffset <= 0 || wordsBefore <= 0 {
		return text
	}
	var wordStarts []int
	inWord := false
	for i := 0; i < len(text); i++ {
		isWord := text[i] != ' ' && text[i] != '\t'
		if isWord && !inWord {
			wordStarts = append(wordStarts, i)
		}
		inWord = isWord
	}
	matchWord := -1
	for i, s := range wordStarts {
		if s > matchOffset {
			break
		}
		matchWord = i
	}
	if matchWord < 0 {
		return text
	}
	cutWord := matchWord - wordsBefore
	if cutWord <= 0 {
		return text
	}
	cutAt := wordStarts[cutWord]
	if cutAt == 0 {
		return text
	}
	return "… " + text[cutAt:]
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
