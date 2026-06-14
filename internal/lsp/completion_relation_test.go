package lsp

import (
	"strings"
	"testing"

	"lore/internal/lore"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestDescribeRelationLabel(t *testing.T) {
	v := lore.NewRelationVocab(lore.BuiltinRelations(), lore.BuiltinPlurals())
	cases := []struct {
		label, wantDetail, wantDocSub string
	}{
		{"parent", "relation", "reciprocal: child"},
		{"father", "alias of parent", "alias of `parent`"},
		{"daughter-of", "alias of parent", "reciprocal: child"},
		{"owns", "alias of possession", "reciprocal: owner"},
		{"friend", "relation", "symmetric"},
	}
	for _, c := range cases {
		detail, doc := describeRelationLabel(v, c.label)
		if detail != c.wantDetail {
			t.Errorf("%s: detail = %q; want %q", c.label, detail, c.wantDetail)
		}
		if !strings.Contains(doc, c.wantDocSub) {
			t.Errorf("%s: doc = %q; want substring %q", c.label, doc, c.wantDocSub)
		}
	}
}

func TestMatchedNameSuffixLen(t *testing.T) {
	cases := []struct {
		prefix, name string
		want         int
	}{
		// Multi-word name completed from a later word: the whole partial name
		// ("Her Do") is the matched span, not just the final word.
		{"Sarah: father -> Her Do", "Her Doktor", len("Her Do")},
		{"Her Do", "Her Doktor", len("Her Do")},
		// Single word still works.
		{"Sarah: father -> Do", "Doug", len("Do")},
		// Case-insensitive.
		{"her do", "Her Doktor", len("her do")},
		// Just after the arrow, nothing typed toward the name yet.
		{"Sarah: father -> ", "Doug", 0},
		// Prefix has a longer earlier suffix that does not prefix the name, so
		// the shorter matching suffix wins.
		{"x Her", "Her Doktor", len("Her")},
		// No overlap at all.
		{"prose ", "Doug", 0},
		// Multibyte: the matched suffix starts on a rune boundary.
		{"Æther Do", "Æther Doktor", len("Æther Do")},
	}
	for _, c := range cases {
		if got := matchedNameSuffixLen(c.prefix, c.name); got != c.want {
			t.Errorf("matchedNameSuffixLen(%q, %q) = %d; want %d", c.prefix, c.name, got, c.want)
		}
	}
}

func TestEntityCompletionsReplaceMultiWordName(t *testing.T) {
	idx := loadIndexWithConfig(t, lore.Config{}, map[string]string{
		"x.md": "Her Doktor (person): a\n",
	})

	// Cursor sits after "Her Do" in a relation target slot.
	line := "Sarah: father -> Her Do"
	pos := protocol.Position{Line: 3, Character: uint32(len(line))}
	list := entityCompletions(idx.World(), line, pos)

	var item *protocol.CompletionItem
	for i := range list.Items {
		if list.Items[i].Label == "Her Doktor" {
			item = &list.Items[i]
		}
	}
	if item == nil {
		t.Fatalf("no completion for Her Doktor; got %+v", list.Items)
	}
	edit, ok := item.TextEdit.(protocol.TextEdit)
	if !ok {
		t.Fatalf("TextEdit = %T; want protocol.TextEdit", item.TextEdit)
	}
	// The edit must replace the whole "Her Do" span, not just "Do", so the
	// result is "Her Doktor" rather than "Her Her Doktor".
	wantStart := uint32(len("Sarah: father -> "))
	wantEnd := uint32(len(line))
	if edit.Range.Start.Character != wantStart || edit.Range.End.Character != wantEnd {
		t.Errorf("range = %d..%d; want %d..%d",
			edit.Range.Start.Character, edit.Range.End.Character, wantStart, wantEnd)
	}
	if edit.Range.Start.Line != 3 || edit.Range.End.Line != 3 {
		t.Errorf("range lines = %d..%d; want 3..3", edit.Range.Start.Line, edit.Range.End.Line)
	}
	if edit.NewText != "Her Doktor" {
		t.Errorf("NewText = %q; want %q", edit.NewText, "Her Doktor")
	}
}

func TestParseRelationLabelContext(t *testing.T) {
	cases := map[string]bool{
		"Sarah: ":                    true,  // start of description
		"Sarah: fa":                  true,  // typing the label
		"x; fr":                      true,  // after a `;` separator
		"Sarah: father is":           false, // not the leading word
		"Sarah: father -> ":          false, // after arrow (target context handles this)
		"Sarah: father -> (Doug: da": true,  // first label inside an aside-in-target

		"plain prose": false, // no separator before the word
		"":            false,
	}
	for prefix, want := range cases {
		if got := parseRelationLabelContext(prefix); got != want {
			t.Errorf("parseRelationLabelContext(%q) = %v; want %v", prefix, got, want)
		}
	}
}

func TestParseRelationTargetContext(t *testing.T) {
	cases := map[string]bool{
		"Sarah: father -> ":               true,  // right after the arrow
		"Sarah: father -> Do":             true,  // partial target name
		"Party: members -> A, B":          true,  // mid target list
		"Sarah: friend -/> ":              true,  // removal arrow
		"Sarah: father->Doug":             true,  // no spaces
		"He went -> there":                true,  // bareword label before arrow (accepted)
		"Sarah: father -> Doug. ":         false, // terminator after arrow
		"Sarah: gold = 5":                 false, // not a relation
		"plain prose":                     false, // no arrow
		"a > b":                           false, // comparison, no '-'
		"-> Doug":                         false, // no label before arrow
		"Sarah: father -> (Doug: da":      false, // cursor inside an aside, not the outer target
		"father -> Barovia (nation), Ot":  true,  // second target after a disambiguated one
		"Sarah: father -> (Doug: x -> Sa": true,  // inside the aside's own target slot
	}
	for prefix, want := range cases {
		if _, _, got := parseRelationTargetContext(prefix); got != want {
			t.Errorf("parseRelationTargetContext(%q) ok = %v; want %v", prefix, got, want)
		}
	}
}

func TestParseRelationTargetContextLabelAndRemove(t *testing.T) {
	cases := []struct {
		prefix string
		label  string
		remove bool
	}{
		{"Sarah: father -> Do", "father", false},
		{"Sarah: friend -/> Ma", "friend", true},
		{"Party: members -/> ", "members", true},
	}
	for _, c := range cases {
		label, remove, ok := parseRelationTargetContext(c.prefix)
		if !ok || label != c.label || remove != c.remove {
			t.Errorf("parseRelationTargetContext(%q) = (%q,%v,%v); want (%q,%v,true)",
				c.prefix, label, remove, ok, c.label, c.remove)
		}
	}
}
