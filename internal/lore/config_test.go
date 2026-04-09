package lore

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestCollectFilesGlob(t *testing.T) {
	fsys := fstest.MapFS{
		"session.md":    &fstest.MapFile{Data: []byte("# Session 1\n")},
		"notes/npcs.md": &fstest.MapFile{Data: []byte("# NPCs\n")},
		"readme.txt":    &fstest.MapFile{Data: []byte("not markdown\n")},
	}

	paths, err := CollectFiles(fsys, Config{Files: []string{"**/*.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(paths), paths)
	}
	for _, p := range paths {
		if !strings.HasSuffix(p, ".md") {
			t.Fatalf("unexpected file: %s", p)
		}
	}
}

func TestCollectFilesIgnorePatterns(t *testing.T) {
	fsys := fstest.MapFS{
		"session.md":     &fstest.MapFile{Data: []byte("")},
		"archive/old.md": &fstest.MapFile{Data: []byte("")},
	}

	paths, err := CollectFiles(fsys, Config{
		Files:  []string{"**/*.md"},
		Ignore: []string{"archive/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(paths), paths)
	}
	if paths[0] != "session.md" {
		t.Fatalf("expected session.md, got %s", paths[0])
	}
}

func TestCollectFilesMultiplePatterns(t *testing.T) {
	fsys := fstest.MapFS{
		"session.md": &fstest.MapFile{Data: []byte("")},
		"world.lore": &fstest.MapFile{Data: []byte("")},
		"notes.txt":  &fstest.MapFile{Data: []byte("")},
	}

	paths, err := CollectFiles(fsys, Config{Files: []string{"**/*.md", "**/*.lore"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(paths), paths)
	}
}

func TestCollectFilesDeeplyNested(t *testing.T) {
	fsys := fstest.MapFS{
		"campaigns/strahd/sessions/01.md": &fstest.MapFile{Data: []byte("")},
		"campaigns/strahd/glossary.md":    &fstest.MapFile{Data: []byte("")},
		"campaigns/notes.md":              &fstest.MapFile{Data: []byte("")},
	}

	paths, err := CollectFiles(fsys, Config{Files: []string{"**/*.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 {
		t.Fatalf("expected 3 files, got %d", len(paths))
	}
	if paths[0] != "campaigns/notes.md" {
		t.Fatalf("expected campaigns/notes.md first, got %s", paths[0])
	}
}

func TestCollectFilesNoMatches(t *testing.T) {
	fsys := fstest.MapFS{
		"readme.txt": &fstest.MapFile{Data: []byte("")},
	}

	paths, err := CollectFiles(fsys, Config{Files: []string{"**/*.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("expected 0 files, got %d", len(paths))
	}
}
