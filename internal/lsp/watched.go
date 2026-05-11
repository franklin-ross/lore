package lsp

import (
	"io/fs"
	"path/filepath"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// registerFileWatchers asks the client to notify us about out-of-editor file
// changes via workspace/didChangeWatchedFiles. We watch every project's globs
// (scoped to that project's subtree) plus a workspace-wide `**/lore.toml`
// pattern so config additions and removals trigger a full reload.
//
// client/registerCapability is a *request*, so s.call blocks until the client
// responds. Handler dispatch is serial — if we blocked here, the reader
// goroutine couldn't deliver the response to us and we'd deadlock. So we
// fire the registration on its own goroutine and let `initialized` return
// immediately. The goroutine only reads fields that are frozen after
// `initialize` and never mutates server state.
func (s *Server) registerFileWatchers() {
	if s.call == nil {
		return
	}

	var watchers []protocol.FileSystemWatcher
	for _, root := range s.configRoots() {
		ps := s.projects[root]
		prefix := projectGlobPrefix(s.root, ps.root)
		for _, pattern := range ps.project.Config.Files {
			watchers = append(watchers, protocol.FileSystemWatcher{
				GlobPattern: prefix + pattern,
			})
		}
	}
	// Watch lore.toml unconditionally — including when no project exists
	// yet — so creating one triggers a reload.
	watchers = append(watchers, protocol.FileSystemWatcher{GlobPattern: "**/lore.toml"})

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

// projectGlobPrefix returns "" if the project is at the workspace root,
// otherwise the workspace-root-relative path with a trailing "/" so it can
// be prepended to a project-relative glob pattern.
func projectGlobPrefix(workspaceRoot, projectRoot string) string {
	rel, err := filepath.Rel(workspaceRoot, projectRoot)
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel) + "/"
}

// didChangeWatchedFiles reconciles the index with out-of-editor changes. For
// each event we:
//   - reload all projects wholesale if any lore.toml changed (cheap, and
//     covers the case where a brand new lore.toml carved out a sub-project
//     from an existing one);
//   - otherwise, refresh the file in its owning project's index unless it's
//     currently open in an editor buffer (in which case the editor is
//     authoritative until didClose, which re-reads from disk).
func (s *Server) didChangeWatchedFiles(_ *glsp.Context, params *protocol.DidChangeWatchedFilesParams) error {
	reloadProjects := false
	for _, evt := range params.Changes {
		abs := uriToPath(&evt.URI)

		if filepath.Base(abs) == "lore.toml" {
			reloadProjects = true
			continue
		}

		ps, rel := s.findOwner(abs)
		if ps == nil || !ps.project.Matcher.Matches(rel) {
			continue
		}

		if s.isOpen(abs) {
			// Editor owns this path. We'll reconcile on didClose, which always
			// re-reads from disk.
			continue
		}

		switch evt.Type {
		case protocol.FileChangeTypeDeleted:
			ps.index.RemoveFile(rel)
		case protocol.FileChangeTypeCreated, protocol.FileChangeTypeChanged:
			data, err := fs.ReadFile(ps.project.FS, rel)
			if err != nil {
				// File vanished between the event and our read — treat as delete.
				ps.index.RemoveFile(rel)
				continue
			}
			ps.index.SetFile(rel, string(data))
		}
	}

	if reloadProjects {
		s.discoverAllProjects()
		s.refreshSemanticTokens()
		s.notifyProjectsChanged()
	}
	return nil
}

// refreshSemanticTokens asks the client to re-request semantic tokens for
// every open document. We send this after a project layout change because
// VSCode caches tokens until the document next changes — without a nudge,
// files that just transitioned in or out of a project would keep their
// stale entity-name colouring until the user edited them. The call is a
// request (response is null); we fire it on its own goroutine because
// handler dispatch is serial and blocking on a round-trip here would
// deadlock the reader.
func (s *Server) refreshSemanticTokens() {
	if s.call == nil {
		return
	}
	call := s.call
	go call(string(protocol.MethodWorkspaceSemanticTokensRefresh), nil, nil)
}

// notifyProjectsChanged tells the client to re-pull anything derived from
// the project layout (definition-range decorations, wiki, graph). The
// client watches lore.toml independently for its own context-key cache,
// but its watcher and our didChangeWatchedFiles handler race — if the
// client refreshes off its own event, those round-trips can complete
// before discoverAllProjects has run. Driving refresh from the server
// side guarantees the new world is in place first.
func (s *Server) notifyProjectsChanged() {
	if s.notify == nil {
		return
	}
	s.notify("lore/projectsChanged", nil)
}
