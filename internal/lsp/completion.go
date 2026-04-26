package lsp

import (
	"fmt"
	"sort"
	"strings"

	"lore/internal/lore"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) completion(_ *glsp.Context, params *protocol.CompletionParams) (any, error) {
	ps, _ := s.projectForURI(params.TextDocument.URI)
	if ps == nil {
		return &protocol.CompletionList{}, nil
	}
	// Inspect the character immediately before the cursor to decide which
	// completion kind to offer.
	line := s.getLine(params.TextDocument.URI, params.Position.Line)
	char := int(params.Position.Character)
	if char > 0 && char <= len(line) {
		prev := line[char-1]
		if prev == '+' {
			return tagCompletionsAllKnown(ps.world()), nil
		}
		if prev == '-' {
			return tagCompletionsActive(ps.world()), nil
		}
	}
	return entityCompletions(ps.world()), nil
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

// tagCompletionsActive returns a CompletionList of tags currently active on
// any entity in the given world (after full resolution).
func tagCompletionsActive(world *lore.World) *protocol.CompletionList {
	seen := make(map[string]struct{})
	for i := range world.Entities {
		for tag := range world.Entities[i].Tags {
			seen[tag] = struct{}{}
		}
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
	return &protocol.CompletionList{Items: items}
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
