package lsp

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) completion(_ *glsp.Context, _ *protocol.CompletionParams) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.world == nil {
		return nil, nil
	}

	kind := protocol.CompletionItemKindText
	var items []protocol.CompletionItem

	for i := range s.world.Entities {
		ent := &s.world.Entities[i]
		doc := ""
		if len(ent.Descriptions) > 0 {
			doc = truncate(ent.Descriptions[0].Text, 200)
		}

		// Add canonical name.
		items = append(items, protocol.CompletionItem{
			Label:         ent.Name,
			Kind:          &kind,
			Detail:        ptrStr(ent.Type),
			Documentation: doc,
		})

		// Add each alias as a separate completion item.
		for _, alias := range ent.Aliases {
			items = append(items, protocol.CompletionItem{
				Label:         alias,
				Kind:          &kind,
				Detail:        ptrStr(ent.Type),
				Documentation: doc,
			})
		}
	}

	return &protocol.CompletionList{
		IsIncomplete: false,
		Items:        items,
	}, nil
}
