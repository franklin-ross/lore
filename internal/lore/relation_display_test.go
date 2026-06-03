package lore

import "testing"

func TestFormatRelationsSharedSurfacePluralised(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	groups := []RelationGroup{{
		Canonical: "child",
		Items: []RelationItem{
			{Other: "Mary", Surface: "daughter"},
			{Other: "Sarah", Surface: "daughter"},
		},
	}}
	got := FormatRelationsBlock(groups, v)
	want := "daughters → Mary, Sarah"
	if got != want {
		t.Fatalf("got %q; want %q", got, want)
	}
}

func TestFormatRelationsMixedSurfacesAnnotateDeviations(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	groups := []RelationGroup{{
		Canonical: "child",
		Items: []RelationItem{
			{Other: "Sarah", Surface: "daughter"},
			{Other: "Tim", Surface: "child"},
		},
	}}
	got := FormatRelationsBlock(groups, v)
	want := "children → Sarah (daughter), Tim"
	if got != want {
		t.Fatalf("got %q; want %q", got, want)
	}
}

func TestFormatRelationsSingleItemUsesSurface(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	groups := []RelationGroup{{
		Canonical: "parent",
		Items:     []RelationItem{{Other: "Doug", Surface: "father"}},
	}}
	if got := FormatRelationsBlock(groups, v); got != "father → Doug" {
		t.Fatalf("got %q; want %q", got, "father → Doug")
	}
}

func TestFormatRelationsIncoming(t *testing.T) {
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	groups := []RelationGroup{{
		Canonical: "bestie",
		Items:     []RelationItem{{Other: "Sarah", Surface: "bestie", Incoming: true}},
	}}
	if got := FormatRelationsBlock(groups, v); got != "Sarah → bestie" {
		t.Fatalf("got %q; want %q", got, "Sarah → bestie")
	}
}

func TestFormatRelationsEndToEnd(t *testing.T) {
	world := setupTestWorld(t, "Doug (person): daughter -> Sarah; child -> Tim\n\nSarah (person): x.\n\nTim (person): y.\n")
	v := NewRelationVocab(BuiltinRelations(), BuiltinPlurals())
	doug, _ := world.FindEntity("Doug")
	got := FormatRelationsBlock(world.ResolveRelations(v, doug), v)
	want := "children → Sarah (daughter), Tim"
	if got != want {
		t.Fatalf("got %q; want %q", got, want)
	}
}
