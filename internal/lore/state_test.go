package lore

import "testing"

func TestFieldValueNumeric(t *testing.T) {
	v := FieldValue{Kind: FieldNumeric, Number: 42}
	if v.Kind != FieldNumeric {
		t.Fatalf("kind = %v", v.Kind)
	}
	if v.Number != 42 {
		t.Fatalf("number = %v", v.Number)
	}
}

func TestFieldValueText(t *testing.T) {
	v := FieldValue{Kind: FieldText, Text: []string{"sword", "shield"}}
	if len(v.Text) != 2 {
		t.Fatalf("text len = %d", len(v.Text))
	}
	if v.Text[0] != "sword" || v.Text[1] != "shield" {
		t.Fatalf("text = %v", v.Text)
	}
}

func TestStateEventConstruction(t *testing.T) {
	ev := StateEvent{
		Op:     StateOpSet,
		Target: "status",
		Value:  &FieldValue{Kind: FieldText, Text: []string{"alive"}},
		Span:   StateSpan{File: "test.md", Line: 5, StartByte: 10, EndByte: 22},
	}
	if ev.Op != StateOpSet {
		t.Fatalf("op = %v", ev.Op)
	}
	if ev.Target != "status" {
		t.Fatalf("target = %q", ev.Target)
	}
}

func TestEntityHasStateFields(t *testing.T) {
	var e Entity
	e.Tags = map[string]bool{"injured": true}
	e.Fields = map[string]FieldValue{
		"population": {Kind: FieldNumeric, Number: 100},
	}
	e.StateHistory = []StateEvent{{Op: StateOpAdd, Target: "injured"}}
	if !e.Tags["injured"] {
		t.Fatal("tag not set")
	}
	if e.Fields["population"].Number != 100 {
		t.Fatal("field not set")
	}
	if len(e.StateHistory) != 1 {
		t.Fatal("history not set")
	}
}
