package lore

import "testing"

func TestParseDisambiguation(t *testing.T) {
	d, ok := ParseDisambiguation("Barovia (town)")
	if !ok {
		t.Fatal("expected ok")
	}
	if d.Name != "Barovia" || d.Type != "town" {
		t.Fatalf("got %+v", d)
	}

	if _, ok := ParseDisambiguation("just a name"); ok {
		t.Fatal("expected not ok for plain name")
	}
	if _, ok := ParseDisambiguation("name (type) trailing"); ok {
		t.Fatal("expected not ok with trailing text")
	}
	if _, ok := ParseDisambiguation("()"); ok {
		t.Fatal("expected not ok for empty parens")
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	if !ContainsIgnoreCase("Sildar Hallwinter is a fighter", "sildar") {
		t.Fatal("expected match")
	}
	if !ContainsIgnoreCase("We met STRAHD", "strahd") {
		t.Fatal("expected match")
	}
	if ContainsIgnoreCase("hello", "world") {
		t.Fatal("expected no match")
	}
}
