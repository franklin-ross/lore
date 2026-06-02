package lsp

import (
	"testing"

	"lore/internal/lore"
)

func TestBuildRelationEdgesGraph(t *testing.T) {
	idx := loadIndexWithConfig(t, lore.Config{}, map[string]string{
		"x.md": "Sarah (person): father -> Doug\n\nDoug (person): a\n",
	})
	res := buildGraphResult(idx.World())
	if len(res.RelationEdges) != 1 {
		t.Fatalf("want 1 relation edge, got %+v", res.RelationEdges)
	}
	e := res.RelationEdges[0]
	if e.From != "Doug" || e.To != "Sarah" || e.Label != "child" {
		t.Fatalf("relation edge = %+v; want Doug -> Sarah labelled child", e)
	}
}

func TestBuildEntityRelationsWiki(t *testing.T) {
	idx := loadIndexWithConfig(t, lore.Config{}, map[string]string{
		"x.md": "Doug (person): daughter -> Sarah; child -> Tim\n\n" +
			"Sarah (person): a\n\nTim (person): b\n\n" +
			"Mary (person): bestie -> Doug\n",
	})
	w := idx.World()
	doug, err := w.FindEntity("Doug")
	if err != nil {
		t.Fatal(err)
	}
	groups := buildEntityRelations(w, doug)

	var out, in *EntityRelationGroup
	for i := range groups {
		switch {
		case groups[i].Incoming:
			in = &groups[i]
		default:
			out = &groups[i]
		}
	}

	if out == nil || out.Header != "children" {
		t.Fatalf("outgoing group = %+v; want header \"children\"", out)
	}
	if len(out.Items) != 2 {
		t.Fatalf("children items = %+v; want 2", out.Items)
	}
	// Sarah was written as "daughter" — a deviation from canonical child.
	var sarah *EntityRelationRef
	for i := range out.Items {
		if out.Items[i].Label == "Sarah" {
			sarah = &out.Items[i]
		}
	}
	if sarah == nil || sarah.Annotation != "daughter" {
		t.Fatalf("Sarah item = %+v; want annotation \"daughter\"", sarah)
	}
	if sarah.ColourIndex < 0 {
		t.Fatalf("Sarah should resolve to an entity with a colour, got %d", sarah.ColourIndex)
	}

	if in == nil || in.Header != "bestie" || len(in.Items) != 1 || in.Items[0].Label != "Mary" {
		t.Fatalf("incoming group = %+v; want bestie from Mary", in)
	}
}
