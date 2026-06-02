package lore

import (
	"reflect"
	"testing"
)

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

func TestEdgeAddSingleTarget(t *testing.T) {
	edges := edgeEvents("father -> Doug")
	if len(edges) != 1 {
		t.Fatalf("got %d edges, want 1: %+v", len(edges), edges)
	}
	e := edges[0]
	if e.Op != StateOpEdgeAdd {
		t.Fatalf("op = %v; want EdgeAdd", e.Op)
	}
	if e.Target != "father" {
		t.Fatalf("label = %q; want father", e.Target)
	}
	if e.Value == nil || !reflect.DeepEqual(e.Value.Text, []string{"Doug"}) {
		t.Fatalf("targets = %+v; want [Doug]", e.Value)
	}
}

func TestEdgeAddMultipleTargets(t *testing.T) {
	edges := edgeEvents("members -> Aragorn, Bilbo, Frodo")
	if len(edges) != 1 {
		t.Fatalf("got %d edges, want 1", len(edges))
	}
	want := []string{"Aragorn", "Bilbo", "Frodo"}
	if !reflect.DeepEqual(edges[0].Value.Text, want) {
		t.Fatalf("targets = %v; want %v", edges[0].Value.Text, want)
	}
}

func TestEdgeRemove(t *testing.T) {
	edges := edgeEvents("friend -/> Mary")
	if len(edges) != 1 {
		t.Fatalf("got %d edges, want 1", len(edges))
	}
	if edges[0].Op != StateOpEdgeRemove {
		t.Fatalf("op = %v; want EdgeRemove", edges[0].Op)
	}
	if !reflect.DeepEqual(edges[0].Value.Text, []string{"Mary"}) {
		t.Fatalf("targets = %v; want [Mary]", edges[0].Value.Text)
	}
}

func TestEdgeTargetWithDisambiguation(t *testing.T) {
	edges := edgeEvents("ruler -> Barovia (nation)")
	if len(edges) != 1 {
		t.Fatalf("got %d edges, want 1", len(edges))
	}
	if !reflect.DeepEqual(edges[0].Value.Text, []string{"Barovia (nation)"}) {
		t.Fatalf("targets = %v; want [Barovia (nation)]", edges[0].Value.Text)
	}
}

func TestEdgeDoesNotShadowFieldRemove(t *testing.T) {
	// `-=` is still a field remove, not an edge.
	events, _ := ParseDirectives("inventory -= longsword", "f.md", 1)
	if len(events) != 1 || events[0].Op != StateOpRemove {
		t.Fatalf("expected a single field-remove event, got %+v", events)
	}
}

func TestEdgeTerminator(t *testing.T) {
	edges := edgeEvents("father -> Doug; mood = happy")
	if len(edges) != 1 || !reflect.DeepEqual(edges[0].Value.Text, []string{"Doug"}) {
		t.Fatalf("edge targets = %+v; want [Doug] only", edges)
	}
}

func TestEdgeMissingTargetWarns(t *testing.T) {
	events, issues := ParseDirectives("father -> .", "f.md", 1)
	for _, ev := range events {
		if ev.Op == StateOpEdgeAdd {
			t.Fatalf("expected no edge event for empty target")
		}
	}
	if len(issues) == 0 {
		t.Fatalf("expected a diagnostic for missing target")
	}
}

func TestEdgeNotAField(t *testing.T) {
	// An edge event must not pollute resolved fields.
	events, _ := ParseDirectives("father -> Doug", "f.md", 1)
	_, fields, _ := ResolveState(events)
	if len(fields) != 0 {
		t.Fatalf("edge leaked into fields: %+v", fields)
	}
}
