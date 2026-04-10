package lore

import (
	"testing"
	"testing/fstest"
)

// testFS builds an fstest.MapFS from a map of filename → content.
func testFS(files map[string]string) fstest.MapFS {
	m := make(fstest.MapFS, len(files))
	for name, content := range files {
		m[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return m
}

// setupTestProject creates an in-memory Project from the given files.
func setupTestProject(t *testing.T, files map[string]string) *Project {
	t.Helper()
	fsys := testFS(files)
	cfg := Config{Files: []string{"**/*.md"}}
	matcher := Matcher{Patterns: cfg.Files}
	paths, err := matcher.Find(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return &Project{FS: fsys, Config: cfg, Matcher: matcher, FilePaths: paths}
}

// setupTestWorld creates a World from a single content string.
func setupTestWorld(t *testing.T, content string) *World {
	t.Helper()
	return Merge([]*FileParse{ParseFile("test.md", content)})
}
