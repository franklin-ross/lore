package lsp

import (
	"os"
	"path/filepath"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// writeFile writes content to an absolute path, creating parent dirs.
func writeFile(t *testing.T, abs, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// sendWatched drives the didChangeWatchedFiles handler directly.
func sendWatched(t *testing.T, s *Server, events ...protocol.FileEvent) {
	t.Helper()
	if err := s.didChangeWatchedFiles(nil, &protocol.DidChangeWatchedFilesParams{
		Changes: events,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestWatchedCreatedAddsNewFile(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"glossary.md": "Sildar (character): Fighter.\n",
	})

	// A new file lands on disk via git pull.
	writeFile(t, filepath.Join(s.root, "sessions/01.md"), "Klarg (character): Goblin chief.\n")
	sendWatched(t, s, protocol.FileEvent{
		URI:  uriFor("sessions/01.md"),
		Type: protocol.FileChangeTypeCreated,
	})

	mustFindEntity(t, s, "Klarg")
}

func TestWatchedChangedRefreshesFile(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"glossary.md": "Sildar (character): Fighter.\n",
	})

	writeFile(t, filepath.Join(s.root, "glossary.md"),
		"Sildar (character): Fighter.\n\nKlarg (character): Goblin chief.\n")
	sendWatched(t, s, protocol.FileEvent{
		URI:  uriFor("glossary.md"),
		Type: protocol.FileChangeTypeChanged,
	})

	mustFindEntity(t, s, "Klarg")
}

func TestWatchedDeletedRemovesFile(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"glossary.md": "Sildar (character): Fighter.\n",
	})

	if err := os.Remove(filepath.Join(s.root, "glossary.md")); err != nil {
		t.Fatal(err)
	}
	sendWatched(t, s, protocol.FileEvent{
		URI:  uriFor("glossary.md"),
		Type: protocol.FileChangeTypeDeleted,
	})

	assertNoEntity(t, s, "Sildar")
}

func TestWatchedIgnoresEventForOpenBuffer(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"session.md": "Sildar (character): Fighter.\n",
	})

	uri := uriFor("session.md")
	openDoc(t, s, uri, "Sildar (character): Fighter.\n")

	// User has unsaved edits adding Klarg in the buffer.
	changeDoc(t, s, uri, "Sildar (character): Fighter.\n\nKlarg (character): Goblin chief.\n")
	mustFindEntity(t, s, "Klarg")

	// Meanwhile the file changes on disk (git pull) with different content.
	writeFile(t, filepath.Join(s.root, "session.md"),
		"Sildar (character): Fighter.\n\nYeemik (character): Goblin lieutenant.\n")
	sendWatched(t, s, protocol.FileEvent{
		URI:  uri,
		Type: protocol.FileChangeTypeChanged,
	})

	// The buffer still owns the file, so the disk change must not leak in.
	mustFindEntity(t, s, "Klarg")
	assertNoEntity(t, s, "Yeemik")
}

// The important close-time reconcile: an external change is ignored while the
// file is open, then the user discards their edits by closing the buffer. On
// close we must pick up the disk content, not silently keep the stale pre-edit
// parse.
func TestWatchedCloseAfterExternalChangePicksUpDisk(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"session.md": "Sildar (character): Fighter.\n",
	})

	uri := uriFor("session.md")
	openDoc(t, s, uri, "Sildar (character): Fighter.\n")

	// External change while the buffer is open. This event is ignored
	// because the editor is authoritative.
	writeFile(t, filepath.Join(s.root, "session.md"),
		"Sildar (character): Fighter.\n\nYeemik (character): Goblin lieutenant.\n")
	sendWatched(t, s, protocol.FileEvent{
		URI:  uri,
		Type: protocol.FileChangeTypeChanged,
	})
	assertNoEntity(t, s, "Yeemik")

	// User closes the buffer without saving. didClose must re-read from
	// disk and surface the externally-added Yeemik.
	closeDoc(t, s, uri)
	mustFindEntity(t, s, "Yeemik")
}

func TestWatchedDeleteOfOpenBufferDeferredUntilClose(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"session.md": "Sildar (character): Fighter.\n",
	})

	uri := uriFor("session.md")
	openDoc(t, s, uri, "Sildar (character): Fighter.\n")

	// File is removed on disk while still open in the editor.
	if err := os.Remove(filepath.Join(s.root, "session.md")); err != nil {
		t.Fatal(err)
	}
	sendWatched(t, s, protocol.FileEvent{
		URI:  uri,
		Type: protocol.FileChangeTypeDeleted,
	})

	// Editor still holds the buffer — Sildar remains visible.
	mustFindEntity(t, s, "Sildar")

	// Close reconciles: disk read fails, file drops from the index.
	closeDoc(t, s, uri)
	assertNoEntity(t, s, "Sildar")
}

func TestWatchedIgnoresUntrackedPaths(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"glossary.md": "Sildar (character): Fighter.\n",
	})

	// A non-markdown file outside the patterns must not reach the index.
	writeFile(t, filepath.Join(s.root, "notes.txt"),
		"Klarg (character): Goblin chief.\n")
	sendWatched(t, s, protocol.FileEvent{
		URI:  uriFor("notes.txt"),
		Type: protocol.FileChangeTypeCreated,
	})

	assertNoEntity(t, s, "Klarg")
}

func TestWatchedLoreTomlChangeTriggersReload(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"lore.toml":         `files = ["glossary/**/*.md"]` + "\n",
		"glossary/chars.md": "Sildar (character): Fighter.\n",
		"sessions/01.md":    "Klarg (character): Goblin chief.\n",
	})

	// Only glossary is tracked initially.
	mustFindEntity(t, s, "Sildar")
	assertNoEntity(t, s, "Klarg")

	// Broaden the config on disk so sessions are also tracked, then notify.
	writeFile(t, filepath.Join(s.root, "lore.toml"), `files = ["**/*.md"]`+"\n")
	sendWatched(t, s, protocol.FileEvent{
		URI:  uriFor("lore.toml"),
		Type: protocol.FileChangeTypeChanged,
	})

	mustFindEntity(t, s, "Sildar")
	mustFindEntity(t, s, "Klarg")
}

func TestWatchedLoreTomlReloadPreservesOpenBuffer(t *testing.T) {
	s, uriFor := setupLifecycleServer(t, map[string]string{
		"session.md": "Sildar (character): Fighter.\n",
	})

	uri := uriFor("session.md")
	openDoc(t, s, uri, "Sildar (character): Fighter.\n")
	changeDoc(t, s, uri, "Sildar (character): Fighter.\n\nKlarg (character): Goblin chief.\n")
	mustFindEntity(t, s, "Klarg")

	// Rewriting lore.toml forces loadProject(), which rebuilds from disk.
	// The open buffer's unsaved Klarg must survive the reload.
	writeFile(t, filepath.Join(s.root, "lore.toml"), `files = ["**/*.md"]`+"\n")
	sendWatched(t, s, protocol.FileEvent{
		URI:  uriFor("lore.toml"),
		Type: protocol.FileChangeTypeChanged,
	})

	mustFindEntity(t, s, "Klarg")
}
