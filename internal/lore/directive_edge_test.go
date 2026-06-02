package lore

func edgeEvents(text string) []StateEvent {
	events, _ := ParseDirectives(text, "f.md", 1)
	var edges []StateEvent
	for _, ev := range events {
		if ev.Op == StateOpEdgeAdd || ev.Op == StateOpEdgeRemove {
			edges = append(edges, ev)
		}
	}
	return edges
}
