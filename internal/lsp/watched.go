package lsp

import (
	"io/fs"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// registerFileWatchers asks the client to notify us about out-of-editor file
// changes via workspace/didChangeWatchedFiles. The watcher covers every glob
// from the project config plus lore.toml itself so config edits trigger a
// full reload.
//
// client/registerCapability is a *request*, so s.call blocks until the client
// responds. Handler dispatch is serial — if we blocked here, the reader
// goroutine couldn't deliver the response to us and we'd deadlock. So we
// fire the registration on its own goroutine and let `initialized` return
// immediately. The goroutine only reads fields that are frozen after
// `initialize` and never mutates server state.
func (s *Server) registerFileWatchers() {
	if s.call == nil || s.project == nil {
		return
	}

	watchers := make([]protocol.FileSystemWatcher, 0, len(s.project.Config.Files)+1)
	for _, pattern := range s.project.Config.Files {
		watchers = append(watchers, protocol.FileSystemWatcher{GlobPattern: pattern})
	}
	watchers = append(watchers, protocol.FileSystemWatcher{GlobPattern: "lore.toml"})

	params := protocol.RegistrationParams{
		Registrations: []protocol.Registration{{
			ID:     "lore-watched-files",
			Method: string(protocol.MethodWorkspaceDidChangeWatchedFiles),
			RegisterOptions: protocol.DidChangeWatchedFilesRegistrationOptions{
				Watchers: watchers,
			},
		}},
	}

	call := s.call
	go call(string(protocol.ServerClientRegisterCapability), params, nil)
}

// didChangeWatchedFiles reconciles the index with out-of-editor changes. For
// each event we:
//   - reload the project wholesale if lore.toml changed;
//   - otherwise, refresh the file from disk unless it's currently open in an
//     editor buffer (in which case the editor is authoritative until
//     didClose, which re-reads from disk).
func (s *Server) didChangeWatchedFiles(_ *glsp.Context, params *protocol.DidChangeWatchedFilesParams) error {
	if s.project == nil {
		return nil
	}

	reloadProject := false
	for _, evt := range params.Changes {
		rel := s.uriToRelPath(evt.URI)

		if rel == "lore.toml" {
			reloadProject = true
			continue
		}

		if !s.project.Matcher.Matches(rel) {
			continue
		}

		if s.isOpen(rel) {
			// Editor owns this path. We'll reconcile on didClose, which always
			// re-reads from disk.
			continue
		}

		switch evt.Type {
		case protocol.FileChangeTypeDeleted:
			s.index.RemoveFile(rel)
		case protocol.FileChangeTypeCreated, protocol.FileChangeTypeChanged:
			data, err := fs.ReadFile(s.project.FS, rel)
			if err != nil {
				// File vanished between the event and our read — treat as delete.
				s.index.RemoveFile(rel)
				continue
			}
			s.index.SetFile(rel, string(data))
		}
	}

	if reloadProject {
		s.loadProject()
	}
	return nil
}
