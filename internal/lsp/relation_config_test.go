package lsp

import (
	"testing"
	"testing/fstest"

	"lore/internal/lore"
)

func loadIndexWithConfig(t *testing.T, cfg lore.Config, files map[string]string) *Index {
	t.Helper()
	fsys := make(fstest.MapFS)
	for name, content := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(content)}
	}
	cfg.Files = []string{"**/*.md"}
	matcher := lore.Matcher{Patterns: cfg.Files}
	paths, err := matcher.Find(fsys)
	if err != nil {
		t.Fatal(err)
	}
	proj := &lore.Project{FS: fsys, Config: cfg, Matcher: matcher, FilePaths: paths}
	idx := NewIndex()
	if err := idx.LoadProject(proj); err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestIndexWorldSurfacesRelationConflicts(t *testing.T) {
	cfg := lore.Config{Relations: map[string]lore.RelationConfig{
		"aunt":  {Reciprocal: "nibling"},
		"uncle": {Reciprocal: "nibling"},
	}}
	idx := loadIndexWithConfig(t, cfg, map[string]string{"x.md": "A (person): hi\n"})
	if len(idx.World().RelationIssues) == 0 {
		t.Fatal("expected relation config conflict to surface on the LSP world")
	}
}

func TestIndexWorldUsesConfigVocab(t *testing.T) {
	cfg := lore.Config{Relations: map[string]lore.RelationConfig{
		"mentor": {Reciprocal: "student", Aliases: []string{"teacher"}},
	}}
	idx := loadIndexWithConfig(t, cfg, map[string]string{
		"x.md": "Gandalf (person): teacher -> Frodo\n\nFrodo (person): a hobbit\n",
	})
	w := idx.World()
	frodo, err := w.FindEntity("Frodo")
	if err != nil {
		t.Fatal(err)
	}
	groups := w.ResolveRelations(w.Vocab, frodo)
	found := false
	for _, g := range groups {
		if g.Canonical == "student" {
			found = true
		}
	}
	if !found {
		t.Fatalf("config relation (mentor/student) not applied in LSP world: %+v", groups)
	}
}
