package lsp

import (
	"strings"
	"testing"

	"lore/internal/lore"
)

func TestRelationLabelAt(t *testing.T) {
	cases := []struct {
		name string
		line string
		col  int
		want string // expected label, or "" for no match
	}{
		{"on the label", "Sarah: father -> Doug", 9, "father"},
		{"on the subject", "Sarah: father -> Doug", 2, ""},
		{"on the target", "Sarah: father -> Doug", 18, ""},
		{"removal arrow", "Sarah: father -/> Doug", 9, "father"},
		{"second directive", "Sarah: friend -> Mary; father -> Doug", 25, "father"},
		{"target before next directive", "Sarah: friend -> Mary; father -> Doug", 18, ""},
		{"continuation line", "  father -> Doug", 4, "father"},
		{"prose arrow is not a label", "the door -> opens", 5, ""},
		{"genitive compound", "Sarah: daughter-of -> Doug", 11, "daughter-of"},
		// Spaces around the arrow are optional — the label must still resolve.
		{"tight arrow", "Sarah: father->Doug", 9, "father"},
		{"tight removal arrow", "Sarah: father-/>Doug", 9, "father"},
		{"tight genitive compound", "Sarah: daughter-of->Doug", 11, "daughter-of"},
		{"no space either side", "Sarah:father->Doug", 8, "father"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end, ok := relationLabelAt(c.line, c.col)
			got := ""
			if ok {
				got = c.line[start:end]
			}
			if got != c.want {
				t.Errorf("relationLabelAt(%q, %d) = %q; want %q", c.line, c.col, got, c.want)
			}
		})
	}
}

func TestRelationLabelHoverMarkdown(t *testing.T) {
	v := lore.NewRelationVocab(lore.BuiltinRelations(), lore.BuiltinPlurals())
	cases := []struct{ label, want string }{
		{"father", "### father (relation)"},
		{"father", "**Canonical:** `parent`"},
		{"father", "**Reciprocal:** `child`"},
		{"parent", "**Aliases:** `father`"},
		{"parent", "**Reciprocal:** `child`"},
		{"friend", "**Reciprocal:** `friend` (symmetric)"},
		{"daughter-of", "**Canonical:** `parent`"},
		{"bestie", "(generic relation)"},
	}
	for _, c := range cases {
		got := relationLabelHoverMarkdown(v, c.label)
		if !strings.Contains(got, c.want) {
			t.Errorf("hover(%q) = %q; want substring %q", c.label, got, c.want)
		}
	}
}
