package lsp

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"lore/internal/lore"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) completion(_ *glsp.Context, params *protocol.CompletionParams) (any, error) {
	ps, rel := s.projectForURI(params.TextDocument.URI)
	if ps == nil {
		return &protocol.CompletionList{}, nil
	}
	line := s.getLine(params.TextDocument.URI, params.Position.Line)
	char := min(int(params.Position.Character), len(line))
	prefix := line[:char]

	if field, op, ok := parseFieldListContext(prefix); ok {
		cursorLine := int(params.Position.Line) + 1
		ent := findOwningEntity(ps.world(), rel, cursorLine)
		if ent == nil {
			return &protocol.CompletionList{}, nil
		}
		switch op {
		case lore.StateOpRemove:
			return listFieldActiveCompletions(ps.world(), ent, rel, cursorLine, field), nil
		case lore.StateOpIncrement:
			return listFieldKnownCompletions(ent, field), nil
		}
	}

	if op, ok := parseTagSigilContext(prefix); ok {
		if op == lore.StateOpAdd {
			return tagCompletionsAllKnown(ps.world()), nil
		}
		// `-tag` removes a tag from the entity whose description owns the
		// cursor. Suggesting tags from elsewhere in the world would offer
		// removals that can't apply, so scope to the owning entity's
		// active set at this point in time.
		cursorLine := int(params.Position.Line) + 1
		ent := findOwningEntity(ps.world(), rel, cursorLine)
		if ent == nil {
			return &protocol.CompletionList{}, nil
		}
		return entityActiveTagCompletions(ps.world(), ent, rel, cursorLine), nil
	}

	// Inside a relation directive's target list. For `->` (add) offer every
	// entity; for `-/>` (remove) offer only the entities currently connected
	// via that relation, mirroring how `-=`/`-tag` list removable values.
	if label, remove, ok := parseRelationTargetContext(prefix); ok {
		if !remove {
			return entityCompletions(ps.world(), line, params.Position), nil
		}
		cursorLine := int(params.Position.Line) + 1
		ent := findOwningEntity(ps.world(), rel, cursorLine)
		if ent == nil {
			return &protocol.CompletionList{}, nil
		}
		return relationRemovalCompletions(ps.world(), ent, rel, cursorLine, label, line, params.Position), nil
	}

	// The trigger characters exist to open directive popups; if none of the
	// directive matchers fired, suppress so the author isn't bombarded with
	// the entity list every time they type a space, comma, or stray sigil
	// in prose. The editor's quick-suggestions still reopens the entity
	// list once they begin typing a name.
	if params.Context != nil &&
		params.Context.TriggerKind == protocol.CompletionTriggerKindTriggerCharacter {
		return &protocol.CompletionList{}, nil
	}

	// At the start of a directive (the first word after the entity header `:`
	// or a `;` separator) a relation label is plausible, so offer the
	// vocabulary alongside entities. Only the leading word qualifies, so prose
	// further along the line isn't affected.
	if parseRelationLabelContext(prefix) {
		return labelSlotCompletions(ps.world(), line, params.Position), nil
	}

	return entityCompletions(ps.world(), line, params.Position), nil
}

// parseRelationLabelContext reports whether the cursor is typing the first
// word after a `:` or `;` — the position a directive (and thus a relation
// label) begins. Spaces between the separator and the word are allowed.
func parseRelationLabelContext(prefix string) bool {
	j := len(prefix)
	for j > 0 {
		r, w := utf8.DecodeLastRuneInString(prefix[:j])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			j -= w
			continue
		}
		break
	}
	for j > 0 && (prefix[j-1] == ' ' || prefix[j-1] == '\t') {
		j--
	}
	if j == 0 {
		return false
	}
	c := prefix[j-1]
	return c == ':' || c == ';'
}

// relationRemovalCompletions offers the entities currently connected to ent
// via the given relation label, resolved at the cursor — the set a `-/>`
// removal can actually act on. Incoming generic edges are excluded: they're
// declared on the other side and a same-label remove here wouldn't match.
func relationRemovalCompletions(world *lore.World, ent *lore.Entity, file string, cursorLine int, label, line string, pos protocol.Position) *protocol.CompletionList {
	if world.Vocab == nil {
		return &protocol.CompletionList{}
	}
	canon, _ := world.Vocab.Resolve(label)
	groups := world.ResolveRelationsAt(world.Vocab, ent, world.FileOrder, file, cursorLine)
	kind := protocol.CompletionItemKindReference
	seen := make(map[string]bool)
	var items []protocol.CompletionItem
	for _, g := range groups {
		if g.Canonical != canon {
			continue
		}
		for _, it := range g.Items {
			if it.Incoming || seen[it.Other] {
				continue
			}
			seen[it.Other] = true
			items = append(items, protocol.CompletionItem{
				Label:    it.Other,
				Kind:     &kind,
				TextEdit: nameReplaceEdit(line, pos, it.Other, it.Other),
			})
		}
	}
	return &protocol.CompletionList{IsIncomplete: true, Items: items}
}

// labelSlotCompletions offers entity names plus relation-vocabulary labels for
// the directive label slot, so both a relation (`father -> …`) and a plain
// entity reference are one keystroke away.
func labelSlotCompletions(world *lore.World, line string, pos protocol.Position) *protocol.CompletionList {
	list := entityCompletions(world, line, pos)
	if world.Vocab == nil {
		return list
	}
	kind := protocol.CompletionItemKindKeyword
	for _, label := range world.Vocab.Labels() {
		detail, doc := describeRelationLabel(world.Vocab, label)
		list.Items = append(list.Items, protocol.CompletionItem{
			Label:  label,
			Kind:   &kind,
			Detail: ptrStr(detail),
			Documentation: protocol.MarkupContent{
				Kind:  protocol.MarkupKindMarkdown,
				Value: doc,
			},
		})
	}
	return list
}

// describeRelationLabel builds the completion detail and documentation for a
// relation label. detail names the canonical the label stands in for ("alias of
// parent", or "relation" when the label *is* the canonical); doc adds the
// reverse-side label so the author can see what the edge records.
func describeRelationLabel(vocab *lore.RelationVocab, label string) (detail, doc string) {
	canon, known := vocab.Resolve(label)
	if !known {
		return "relation", ""
	}
	canonDisplay := vocab.Display(canon)
	reciprocal := relationReciprocal(vocab, canon)

	if strings.EqualFold(canon, label) {
		return "relation", fmt.Sprintf("Relation `%s` — %s.", canonDisplay, reciprocal)
	}
	return "alias of " + canonDisplay, fmt.Sprintf("`%s`, alias of `%s` — %s.", label, canonDisplay, reciprocal)
}

// parseFieldListContext reports whether prefix ends inside the value of a
// `field +=` or `field -=` directive. It walks backward to find the most
// recent operator, rejects matches that have a top-level directive terminator
// between operator and cursor, and validates that a bareword identifier sits
// before the operator.
func parseFieldListContext(prefix string) (string, lore.StateOp, bool) {
	for i := len(prefix) - 2; i >= 0; i-- {
		if prefix[i+1] != '=' {
			continue
		}
		c := prefix[i]
		if c != '+' && c != '-' {
			continue
		}
		if hasDirectiveTerminator(prefix[i+2:]) {
			continue
		}
		j := i
		for j > 0 && (prefix[j-1] == ' ' || prefix[j-1] == '\t') {
			j--
		}
		end := j
		for j > 0 {
			r, w := utf8.DecodeLastRuneInString(prefix[:j])
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
				j -= w
				continue
			}
			break
		}
		if j == end {
			return "", 0, false
		}
		first, _ := utf8.DecodeRuneInString(prefix[j:])
		if !unicode.IsLetter(first) {
			return "", 0, false
		}
		if j > 0 {
			r, _ := utf8.DecodeLastRuneInString(prefix[:j])
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
				return "", 0, false
			}
		}
		op := lore.StateOpIncrement
		if c == '-' {
			op = lore.StateOpRemove
		}
		return prefix[j:end], op, true
	}
	return "", 0, false
}

// parseTagSigilContext reports whether prefix ends with a `+` or `-` tag
// sigil at a word boundary, optionally followed by a partially-typed tag
// name. Returns the corresponding StateOp (Add for `+`, Remove for `-`).
func parseTagSigilContext(prefix string) (lore.StateOp, bool) {
	n := len(prefix)
	if n == 0 {
		return 0, false
	}

	last := prefix[n-1]
	if last == '+' || last == '-' {
		if n == 1 || isTagBoundary(prefix, n-1) {
			return sigilOp(last), true
		}
		return 0, false
	}

	end := n
	for end > 0 {
		r, w := utf8.DecodeLastRuneInString(prefix[:end])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			end -= w
			continue
		}
		break
	}
	if end == 0 || end == n {
		return 0, false
	}
	c := prefix[end-1]
	if c != '+' && c != '-' {
		return 0, false
	}
	if end == 1 || isTagBoundary(prefix, end-1) {
		return sigilOp(c), true
	}
	return 0, false
}

// parseRelationTargetContext reports whether the cursor sits in the target
// list of a relation directive — after a `->` or `-/>` arrow with no directive
// terminator between the arrow and the cursor. The arrow must be preceded by a
// bareword label so a stray `->` in prose doesn't trigger entity completions.
func parseRelationTargetContext(prefix string) (label string, remove, ok bool) {
	depth := 0 // unmatched ')' seen while scanning back
	for i := len(prefix) - 1; i >= 0; i-- {
		switch prefix[i] {
		case '.', '!', '?', ';', '\n', '\r':
			return "", false, false
		case ')':
			depth++
		case '(':
			if depth == 0 {
				// An unmatched '(' means the cursor sits inside this group —
				// e.g. an inline aside `father -> (Doug: …)`. The outer arrow
				// no longer governs, so this isn't a target slot. A balanced
				// disambiguator like `Barovia (nation)` decrements depth and
				// scanning continues.
				return "", false, false
			}
			depth--
		case '>':
			if depth != 0 {
				continue
			}
			// Check `-/>` before `->`: both end in `>`.
			if i >= 2 && prefix[i-1] == '/' && prefix[i-2] == '-' {
				if lbl, lok := relationLabelBefore(prefix, i-2); lok {
					return lbl, true, true
				}
				return "", false, false
			}
			if i >= 1 && prefix[i-1] == '-' {
				if lbl, lok := relationLabelBefore(prefix, i-1); lok {
					return lbl, false, true
				}
				return "", false, false
			}
		}
	}
	return "", false, false
}

// relationLabelBefore returns the bareword label immediately before the arrow
// whose leading '-' is at arrowStart (spaces allowed between). ok is false when
// no label-shaped token precedes the arrow.
func relationLabelBefore(prefix string, arrowStart int) (string, bool) {
	j := arrowStart
	for j > 0 && (prefix[j-1] == ' ' || prefix[j-1] == '\t') {
		j--
	}
	end := j
	for j > 0 {
		r, w := utf8.DecodeLastRuneInString(prefix[:j])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			j -= w
			continue
		}
		break
	}
	if j == end {
		return "", false
	}
	r, _ := utf8.DecodeRuneInString(prefix[j:end])
	if !unicode.IsLetter(r) {
		return "", false
	}
	return prefix[j:end], true
}

// isTagBoundary mirrors the directive scanner's atWordBoundaryLeft check —
// the rune before pos must not be a tag-name character.
func isTagBoundary(s string, pos int) bool {
	if pos == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(s[:pos])
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-'
}

func sigilOp(c byte) lore.StateOp {
	if c == '+' {
		return lore.StateOpAdd
	}
	return lore.StateOpRemove
}

func hasDirectiveTerminator(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '.', '!', '?', ';', '\n', '\r':
			return true
		}
	}
	return false
}

// findOwningEntity returns the entity whose description spans cursorLine in
// relPath. When descriptions overlap (inline asides on the same line as a
// header, etc.) the innermost one — the latest-starting — wins.
func findOwningEntity(world *lore.World, relPath string, cursorLine int) *lore.Entity {
	var best *lore.Entity
	bestStart := 0
	for i := range world.Entities {
		ent := &world.Entities[i]
		for _, d := range ent.Descriptions {
			if d.File != relPath {
				continue
			}
			if cursorLine < d.Line || cursorLine > d.EndLine {
				continue
			}
			if best == nil || d.Line >= bestStart {
				bestStart = d.Line
				best = ent
			}
		}
	}
	return best
}

// tagCompletionsAllKnown returns a CompletionList of every tag name ever
// seen in any entity's state history within the given world, alphabetically
// sorted.
func tagCompletionsAllKnown(world *lore.World) *protocol.CompletionList {
	seen := make(map[string]struct{})
	for i := range world.Entities {
		for _, ev := range world.Entities[i].StateHistory {
			if ev.Value != nil {
				continue
			}
			if ev.Op == lore.StateOpAdd || ev.Op == lore.StateOpRemove {
				seen[ev.Target] = struct{}{}
			}
		}
	}
	return makeTagCompletionList(seen)
}

// entityActiveTagCompletions returns the tags currently set on ent at the
// cursor's position. Used for `-tag` directives so the suggestion list
// matches what the author can actually remove right now.
func entityActiveTagCompletions(world *lore.World, ent *lore.Entity, file string, cursorLine int) *protocol.CompletionList {
	tags, _, _ := lore.ResolveStateAt(ent.StateHistory, world.FileOrder, file, cursorLine)
	seen := make(map[string]struct{}, len(tags))
	for t := range tags {
		seen[t] = struct{}{}
	}
	return makeTagCompletionList(seen)
}

func makeTagCompletionList(seen map[string]struct{}) *protocol.CompletionList {
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	kind := protocol.CompletionItemKindKeyword
	items := make([]protocol.CompletionItem, 0, len(names))
	for _, n := range names {
		items = append(items, protocol.CompletionItem{
			Label: n,
			Kind:  &kind,
		})
	}
	// IsIncomplete keeps VSCode re-querying as the cursor advances inside a
	// directive — without it the popup snaps shut on a non-word character
	// like space and the user has to retrigger by hand.
	return &protocol.CompletionList{IsIncomplete: true, Items: items}
}

// listFieldActiveCompletions returns the items currently in the named field
// of ent, resolved to the cursor's position in (file, line). Used for
// `field -=` removals so the list shows what can actually be removed.
func listFieldActiveCompletions(world *lore.World, ent *lore.Entity, file string, cursorLine int, field string) *protocol.CompletionList {
	_, fields, _ := lore.ResolveStateAt(ent.StateHistory, world.FileOrder, file, cursorLine)
	fv, ok := fields[field]
	if !ok || fv.Kind != lore.FieldText {
		return &protocol.CompletionList{}
	}
	return makeListItemCompletions(fv.Text)
}

// listFieldKnownCompletions returns every distinct text item ever seen on
// the named field of ent. Used for `field +=` appends so the author can pick
// a previously-mentioned item without retyping it.
func listFieldKnownCompletions(ent *lore.Entity, field string) *protocol.CompletionList {
	seen := make(map[string]struct{})
	for _, ev := range ent.StateHistory {
		if ev.Target != field || ev.Value == nil || ev.Value.Kind != lore.FieldText {
			continue
		}
		for _, item := range ev.Value.Text {
			seen[item] = struct{}{}
		}
	}
	items := make([]string, 0, len(seen))
	for it := range seen {
		items = append(items, it)
	}
	return makeListItemCompletions(items)
}

func makeListItemCompletions(items []string) *protocol.CompletionList {
	sorted := append([]string(nil), items...)
	sort.Strings(sorted)
	kind := protocol.CompletionItemKindValue
	out := make([]protocol.CompletionItem, 0, len(sorted))
	for _, it := range sorted {
		insert := quoteListItemIfNeeded(it)
		item := protocol.CompletionItem{
			Label: it,
			Kind:  &kind,
		}
		if insert != it {
			text := insert
			item.InsertText = &text
		}
		out = append(out, item)
	}
	return &protocol.CompletionList{IsIncomplete: true, Items: out}
}

// quoteListItemIfNeeded mirrors lore.quoteIfNeeded: items containing list
// separators or `=` need to be quoted to round-trip through the directive
// parser unchanged.
func quoteListItemIfNeeded(item string) string {
	if strings.ContainsAny(item, ",.!?;=") {
		return `"` + item + `"`
	}
	return item
}

// nameReplaceEdit builds the TextEdit that swaps the partially typed entity
// name ending at the cursor for newText. Entity names may contain spaces, so
// the editor's default word-boundary completion — which replaces only the last
// whitespace-delimited word — would leave the earlier words in place: typing
// `Her Do` and accepting `Her Doktor` yields `Her Her Doktor`. The edit instead
// replaces the longest run of already-typed text that prefixes name, so the
// whole partial name is overwritten.
func nameReplaceEdit(line string, pos protocol.Position, name, newText string) protocol.TextEdit {
	char := min(int(pos.Character), len(line))
	start := char - matchedNameSuffixLen(line[:char], name)
	return protocol.TextEdit{
		Range: protocol.Range{
			Start: protocol.Position{Line: pos.Line, Character: utf16UnitsForBytes(line, start)},
			End:   protocol.Position{Line: pos.Line, Character: utf16UnitsForBytes(line, char)},
		},
		NewText: newText,
	}
}

// matchedNameSuffixLen returns the byte length of the longest suffix of prefix
// that is a case-insensitive prefix of name — the span of text the author has
// already typed toward this candidate. It walks rune boundaries longest-first
// so a multibyte sequence is never split. Zero means nothing typed matches, so
// the completion inserts at the cursor.
func matchedNameSuffixLen(prefix, name string) int {
	lowerName := strings.ToLower(name)
	for i := 0; i < len(prefix); {
		if strings.HasPrefix(lowerName, strings.ToLower(prefix[i:])) {
			return len(prefix) - i
		}
		_, w := utf8.DecodeRuneInString(prefix[i:])
		i += w
	}
	return 0
}

// entityCompletions returns the existing entity-name suggestion list for the
// given project's world.
func entityCompletions(world *lore.World, line string, pos protocol.Position) *protocol.CompletionList {
	kind := protocol.CompletionItemKindText

	// Count how many distinct entities expose each label (name or alias).
	// A label shared by two or more entities is ambiguous and must be
	// qualified with its type on suggestion.
	labelOwners := make(map[string]int)
	for i := range world.Entities {
		ent := &world.Entities[i]
		seen := make(map[string]bool, 1+len(ent.Aliases))
		for _, label := range append([]string{ent.Name}, ent.Aliases...) {
			key := strings.ToLower(label)
			if seen[key] {
				continue
			}
			seen[key] = true
			labelOwners[key]++
		}
	}

	var items []protocol.CompletionItem
	for i := range world.Entities {
		ent := &world.Entities[i]
		doc := ""
		if len(ent.Descriptions) > 0 {
			doc = truncate(ent.Descriptions[0].Text, 200)
		}

		emit := func(label string) {
			display := label
			if labelOwners[strings.ToLower(label)] > 1 {
				display = fmt.Sprintf("%s (%s)", label, ent.Type)
			}
			// Match the typed text against the bare name but insert the
			// (possibly type-qualified) display form, so an ambiguous label
			// still round-trips as `Name (type)`.
			items = append(items, protocol.CompletionItem{
				Label:         display,
				Kind:          &kind,
				Detail:        ptrStr(ent.Type),
				Documentation: doc,
				TextEdit:      nameReplaceEdit(line, pos, label, display),
			})
		}

		emit(ent.Name)
		for _, alias := range ent.Aliases {
			emit(alias)
		}
	}

	return &protocol.CompletionList{
		IsIncomplete: false,
		Items:        items,
	}
}
