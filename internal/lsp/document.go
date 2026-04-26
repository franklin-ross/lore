package lsp

import (
	"io/fs"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) didOpen(_ *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	abs := uriToPath(&params.TextDocument.URI)
	s.markOpen(abs)

	ps, rel := s.findOwner(abs)
	if ps == nil {
		// File outside every project — nothing to index.
		return nil
	}
	if !ps.project.Matcher.Matches(rel) {
		// Buffer is in the project tree but excluded by the config's globs.
		// Don't index it; the project's view of the world stays clean.
		return nil
	}
	ps.index.SetFile(rel, params.TextDocument.Text)
	s.publishDiagnostics(ps, rel, params.TextDocument.URI)
	return nil
}

func (s *Server) didChange(_ *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	// Full sync mode: last content change is the full document.
	if len(params.ContentChanges) == 0 {
		return nil
	}
	last := params.ContentChanges[len(params.ContentChanges)-1]
	change, ok := last.(protocol.TextDocumentContentChangeEventWhole)
	if !ok {
		return nil
	}
	ps, rel := s.projectForURI(params.TextDocument.URI)
	if ps == nil || !ps.project.Matcher.Matches(rel) {
		return nil
	}
	ps.index.SetFile(rel, change.Text)
	s.publishDiagnostics(ps, rel, params.TextDocument.URI)
	return nil
}

func (s *Server) didSave(_ *glsp.Context, params *protocol.DidSaveTextDocumentParams) error {
	// Buffer contents and on-disk file are identical after a save; the index
	// already reflects the latest keystroke via didChange. Re-publish
	// diagnostics anyway in case the client wants a fresh push.
	ps, rel := s.projectForURI(params.TextDocument.URI)
	if ps == nil {
		return nil
	}
	s.publishDiagnostics(ps, rel, params.TextDocument.URI)
	return nil
}

func (s *Server) didClose(_ *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	// The editor is no longer authoritative for this file. Always re-read
	// from disk so we pick up anything that changed outside the editor while
	// the buffer was open — the watcher ignores those changes for open
	// buffers, so this close is our reconcile point.
	abs := uriToPath(&params.TextDocument.URI)
	s.markClosed(abs)

	ps, rel := s.findOwner(abs)
	if ps == nil {
		return nil
	}
	if !ps.project.Matcher.Matches(rel) {
		// File wasn't tracked while open either; nothing to drop.
		return nil
	}
	data, err := fs.ReadFile(ps.project.FS, rel)
	if err != nil {
		ps.index.RemoveFile(rel)
		return nil
	}
	ps.index.SetFile(rel, string(data))
	return nil
}
