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

	// The trigger characters exist to open directive popups; if none of the
	// directive matchers fired, suppress so the author isn't bombarded with
	// the entity list every time they type a space, comma, or stray sigil
	// in prose. The editor's quick-suggestions still reopens the entity
	// list once they begin typing a name.
	if params.Context != nil &&
		params.Context.TriggerKind == protocol.CompletionTriggerKindTriggerCharacter {
		return &protocol.CompletionList{}, nil
	}

	return entityCompletions(ps.world()), nil
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

// entityCompletions returns the existing entity-name suggestion list for the
// given project's world.
func entityCompletions(world *lore.World) *protocol.CompletionList {
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
			items = append(items, protocol.CompletionItem{
				Label:         display,
				Kind:          &kind,
				Detail:        ptrStr(ent.Type),
				Documentation: doc,
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
