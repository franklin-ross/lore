package lore

import "testing"

func TestDirectiveSubSpans(t *testing.T) {
	check := func(text string, wantName, wantOp [2]int) {
		t.Helper()
		events, _ := ParseDirectives(text, "f.md", 1)
		if len(events) != 1 {
			t.Fatalf("%q: got %d events, want 1", text, len(events))
		}
		e := events[0]
		if e.NameSpan.StartByte != wantName[0] || e.NameSpan.EndByte != wantName[1] {
			t.Errorf("%q name span = [%d,%d); want %v", text, e.NameSpan.StartByte, e.NameSpan.EndByte, wantName)
		}
		if e.OpSpan.StartByte != wantOp[0] || e.OpSpan.EndByte != wantOp[1] {
			t.Errorf("%q op span = [%d,%d); want %v", text, e.OpSpan.StartByte, e.OpSpan.EndByte, wantOp)
		}
	}

	check("gold += 5", [2]int{0, 4}, [2]int{5, 7})      // field name + operator
	check("gold = 5", [2]int{0, 4}, [2]int{5, 6})       // single `=`
	check("inv -= sword", [2]int{0, 3}, [2]int{4, 6})   // `-=`
	check("+injured", [2]int{1, 8}, [2]int{0, 1})       // tag: name after sigil
	check("father -> Doug", [2]int{0, 6}, [2]int{7, 9}) // relation label + arrow
	check("friend -/> Mary", [2]int{0, 6}, [2]int{7, 10})
}

func TestDirectiveValueSpan(t *testing.T) {
	num, _ := ParseDirectives("gold = 875", "f.md", 1)
	if num[0].Value.Kind != FieldNumeric {
		t.Fatalf("gold = 875 should be numeric")
	}
	if num[0].ValueSpan.StartByte != 7 || num[0].ValueSpan.EndByte != 10 {
		t.Errorf("numeric value span = [%d,%d); want [7,10)", num[0].ValueSpan.StartByte, num[0].ValueSpan.EndByte)
	}

	txt, _ := ParseDirectives("status = tense", "f.md", 1)
	if txt[0].Value.Kind != FieldText {
		t.Fatalf("status = tense should be text")
	}
	if txt[0].ValueSpan.StartByte != 9 || txt[0].ValueSpan.EndByte != 14 {
		t.Errorf("text value span = [%d,%d); want [9,14)", txt[0].ValueSpan.StartByte, txt[0].ValueSpan.EndByte)
	}

	// Tags carry no value span.
	tag, _ := ParseDirectives("+injured", "f.md", 1)
	if tag[0].ValueSpan.EndByte > tag[0].ValueSpan.StartByte {
		t.Errorf("tag should have empty value span, got [%d,%d)", tag[0].ValueSpan.StartByte, tag[0].ValueSpan.EndByte)
	}
}
