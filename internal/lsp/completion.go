package lsp

import (
	"fmt"
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) completion(_ *glsp.Context, _ *protocol.CompletionParams) (any, error) {
	world := s.world()
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
	}, nil
}
