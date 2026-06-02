package lsp

import "testing"

func TestParseRelationLabelContext(t *testing.T) {
	cases := map[string]bool{
		"Sarah: ":            true,  // start of description
		"Sarah: fa":          true,  // typing the label
		"x; fr":              true,  // after a `;` separator
		"Sarah: father is":   false, // not the leading word
		"Sarah: father -> ":  false, // after arrow (target context handles this)
		"Sarah: father -> (Doug: da": true, // first label inside an aside-in-target

		"plain prose":        false, // no separator before the word
		"":                   false,
	}
	for prefix, want := range cases {
		if got := parseRelationLabelContext(prefix); got != want {
			t.Errorf("parseRelationLabelContext(%q) = %v; want %v", prefix, got, want)
		}
	}
}

func TestParseRelationTargetContext(t *testing.T) {
	cases := map[string]bool{
		"Sarah: father -> ":       true,  // right after the arrow
		"Sarah: father -> Do":     true,  // partial target name
		"Party: members -> A, B": true,   // mid target list
		"Sarah: friend -/> ":      true,  // removal arrow
		"Sarah: father->Doug":     true,  // no spaces
		"He went -> there":        true,  // bareword label before arrow (accepted)
		"Sarah: father -> Doug. ": false, // terminator after arrow
		"Sarah: gold = 5":         false, // not a relation
		"plain prose":             false, // no arrow
		"a > b":                   false, // comparison, no '-'
		"-> Doug":                 false, // no label before arrow
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
