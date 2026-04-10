package lore

import (
	"reflect"
	"testing"
	"testing/fstest"
)

func TestMatcherFindOrdersByPatternThenAlpha(t *testing.T) {
	fsys := fstest.MapFS{
		"sessions/02.md":  &fstest.MapFile{Data: []byte("")},
		"sessions/01.md":  &fstest.MapFile{Data: []byte("")},
		"glossary/zed.md": &fstest.MapFile{Data: []byte("")},
		"glossary/abe.md": &fstest.MapFile{Data: []byte("")},
	}

	m := Matcher{Patterns: []string{"glossary/**.md", "sessions/**.md"}}
	got, err := m.Find(fsys)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"glossary/abe.md", "glossary/zed.md", "sessions/01.md", "sessions/02.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestMatcherFindFirstMatchWins(t *testing.T) {
	fsys := fstest.MapFS{
		"shared/note.md": &fstest.MapFile{Data: []byte("")},
		"other/note.md":  &fstest.MapFile{Data: []byte("")},
	}

	// shared/note.md matches both patterns; should land under the first one only.
	m := Matcher{Patterns: []string{"shared/**.md", "**/*.md"}}
	got, err := m.Find(fsys)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"shared/note.md", "other/note.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestMatcherFindHonoursIgnore(t *testing.T) {
	fsys := fstest.MapFS{
		"session.md":     &fstest.MapFile{Data: []byte("")},
		"archive/old.md": &fstest.MapFile{Data: []byte("")},
	}

	m := Matcher{
		Patterns: []string{"**/*.md"},
		Ignore:   []string{"archive/**"},
	}
	got, err := m.Find(fsys)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"session.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestMatcherFindIgnoreBareDirName(t *testing.T) {
	// Preserves the existing quirk: bare ignore patterns also match top-level dirs.
	fsys := fstest.MapFS{
		"keep.md":        &fstest.MapFile{Data: []byte("")},
		"archive/old.md": &fstest.MapFile{Data: []byte("")},
	}

	m := Matcher{
		Patterns: []string{"**/*.md"},
		Ignore:   []string{"archive"},
	}
	got, err := m.Find(fsys)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"keep.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestMatcherSortAndFilterDropsNonMatchingAndSorts(t *testing.T) {
	m := Matcher{
		Patterns: []string{"glossary/**.md", "sessions/**.md"},
		Ignore:   []string{"sessions/draft-**"},
	}

	in := []string{
		"sessions/02.md",
		"random/scratch.md",
		"glossary/zed.md",
		"sessions/draft-x.md",
		"glossary/abe.md",
		"notes.txt",
	}

	got := m.SortAndFilter(in)
	want := []string{"glossary/abe.md", "glossary/zed.md", "sessions/02.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestMatcherSortAndFilterReturnsNewSlice(t *testing.T) {
	m := Matcher{Patterns: []string{"**/*.md"}}
	in := []string{"b.md", "a.md"}
	_ = m.SortAndFilter(in)
	// Input must not be mutated.
	if in[0] != "b.md" || in[1] != "a.md" {
		t.Fatalf("Filter mutated input: %v", in)
	}
}
