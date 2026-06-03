package lsp

import (
	"os"
	"path/filepath"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// setupMultiProject lays out a workspace with two sibling lore.toml-rooted
// campaigns (and a third nested inside one of them) so we can verify each
// project sees only its own entities.
func setupMultiProject(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"lore.toml":                        `files = ["**/*.md"]` + "\n",
		"shared.md":                        "Shared (character): Workspace-level NPC.\n",
		"campaigns/strahd/lore.toml":       `files = ["**/*.md"]` + "\n",
		"campaigns/strahd/glossary.md":     "Strahd (character): Vampire lord.\n",
		"campaigns/lostmines/lore.toml":    `files = ["**/*.md"]` + "\n",
		"campaigns/lostmines/glossary.md":  "Sildar (character): Fighter.\n",
		"campaigns/strahd/sub/lore.toml":   `files = ["**/*.md"]` + "\n",
		"campaigns/strahd/sub/glossary.md": "Subentity (item): Sub-project artefact.\n",
	}
	for rel, content := range files {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	s := NewServer()
	s.root = dir
	s.discoverAllProjects()
	return s, dir
}

func TestMultiProjectDiscoversEveryConfig(t *testing.T) {
	s, dir := setupMultiProject(t)

	want := []string{
		dir,
		filepath.Join(dir, "campaigns/strahd"),
		filepath.Join(dir, "campaigns/lostmines"),
		filepath.Join(dir, "campaigns/strahd/sub"),
	}
	for _, root := range want {
		if _, ok := s.projects[root]; !ok {
			t.Errorf("missing project root %q; have %v", root, s.configRoots())
		}
	}
}

func TestMultiProjectFilesScopedToNearestConfig(t *testing.T) {
	s, dir := setupMultiProject(t)

	cases := []struct {
		project string
		want    []string
		notWant []string
	}{
		{
			project: dir,
			want:    []string{"Shared"},
			notWant: []string{"Strahd", "Sildar", "Subentity"},
		},
		{
			project: filepath.Join(dir, "campaigns/strahd"),
			want:    []string{"Strahd"},
			notWant: []string{"Shared", "Sildar", "Subentity"},
		},
		{
			project: filepath.Join(dir, "campaigns/lostmines"),
			want:    []string{"Sildar"},
			notWant: []string{"Shared", "Strahd", "Subentity"},
		},
		{
			project: filepath.Join(dir, "campaigns/strahd/sub"),
			want:    []string{"Subentity"},
			notWant: []string{"Shared", "Strahd", "Sildar"},
		},
	}

	for _, tc := range cases {
		ps, ok := s.projects[tc.project]
		if !ok {
			t.Fatalf("no project at %s", tc.project)
		}
		world := ps.world()
		names := make(map[string]bool, len(world.Entities))
		for i := range world.Entities {
			names[world.Entities[i].Name] = true
		}
		for _, n := range tc.want {
			if !names[n] {
				t.Errorf("project %s missing %q; have %v", tc.project, n, names)
			}
		}
		for _, n := range tc.notWant {
			if names[n] {
				t.Errorf("project %s leaked %q; have %v", tc.project, n, names)
			}
		}
	}
}

func TestEntityListScopedByActiveURI(t *testing.T) {
	s, dir := setupMultiProject(t)

	cases := []struct {
		uri  string
		want string
	}{
		{uri: "file://" + filepath.Join(dir, "campaigns/strahd/glossary.md"), want: "Strahd"},
		{uri: "file://" + filepath.Join(dir, "campaigns/lostmines/glossary.md"), want: "Sildar"},
		{uri: "file://" + filepath.Join(dir, "shared.md"), want: "Shared"},
		{uri: "file://" + filepath.Join(dir, "campaigns/strahd/sub/glossary.md"), want: "Subentity"},
	}
	for _, tc := range cases {
		params := &EntityListParams{
			TextDocument: &protocol.TextDocumentIdentifier{URI: tc.uri},
		}
		got, err := s.entityList(params)
		if err != nil {
			t.Fatalf("%s: %v", tc.uri, err)
		}
		if len(got.Entities) != 1 || got.Entities[0].Name != tc.want {
			names := make([]string, 0, len(got.Entities))
			for _, e := range got.Entities {
				names = append(names, e.Name)
			}
			t.Errorf("%s: want only %q, got %v", tc.uri, tc.want, names)
		}
	}
}

func TestEntityListWithoutURIReturnsAllProjects(t *testing.T) {
	s, _ := setupMultiProject(t)

	got, err := s.entityList(&EntityListParams{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"Shared": true, "Strahd": true, "Sildar": true, "Subentity": true}
	for _, e := range got.Entities {
		delete(want, e.Name)
	}
	if len(want) != 0 {
		t.Errorf("missing %v in workspace-wide entityList", want)
	}
}

func TestDidOpenSkipsFilesOutsideMatcher(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lore.toml"),
		[]byte(`files = ["glossary/**/*.md"]`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "glossary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "glossary/chars.md"),
		[]byte("Sildar (character): Fighter.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewServer()
	s.root = dir
	s.discoverAllProjects()

	// Open a file outside the configured glob — must not enter the index.
	scratchURI := "file://" + filepath.Join(dir, "scratch.md")
	if err := s.didOpen(nil, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:  scratchURI,
			Text: "Goblin (character): Smelly.\n",
		},
	}); err != nil {
		t.Fatal(err)
	}

	ps := s.projects[dir]
	if _, err := ps.world().FindEntity("Goblin"); err == nil {
		t.Fatal("Goblin from out-of-glob file leaked into project index")
	}
	// Sanity: in-glob file is still there.
	if _, err := ps.world().FindEntity("Sildar"); err != nil {
		t.Fatalf("Sildar should still be present: %v", err)
	}
}

func TestNewLoreTomlCarvesOutSubproject(t *testing.T) {
	s, dir := setupMultiProject(t)

	// Add a brand-new sub-project under the workspace-root campaign.
	newRoot := filepath.Join(dir, "spinoff")
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newRoot, "lore.toml"),
		[]byte(`files = ["**/*.md"]`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newRoot, "char.md"),
		[]byte("Spinoff (character): New campaign NPC.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sendWatched(t, s, protocol.FileEvent{
		URI:  "file://" + filepath.Join(newRoot, "lore.toml"),
		Type: protocol.FileChangeTypeCreated,
	})

	if _, ok := s.projects[newRoot]; !ok {
		t.Fatalf("expected project at %s after lore.toml created; roots: %v", newRoot, s.configRoots())
	}
	ps := s.projects[newRoot]
	if _, err := ps.world().FindEntity("Spinoff"); err != nil {
		t.Fatal(err)
	}

	// The workspace-root project must no longer claim spinoff/char.md.
	root := s.projects[dir]
	if _, err := root.world().FindEntity("Spinoff"); err == nil {
		t.Fatal("workspace-root project should not see Spinoff after sub-project carve-out")
	}
}
