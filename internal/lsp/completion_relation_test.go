package lsp

import "testing"

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
	}
	for prefix, want := range cases {
		if got := parseRelationTargetContext(prefix); got != want {
			t.Errorf("parseRelationTargetContext(%q) = %v; want %v", prefix, got, want)
		}
	}
}
