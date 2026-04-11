package lsp

import (
	"io/fs"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) didOpen(_ *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	rel := s.uriToRelPath(params.TextDocument.URI)
	s.markOpen(rel)
	s.index.SetFile(rel, params.TextDocument.Text)
	s.publishDiagnostics(rel, params.TextDocument.URI)
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
	rel := s.uriToRelPath(params.TextDocument.URI)
	s.index.SetFile(rel, change.Text)
	s.publishDiagnostics(rel, params.TextDocument.URI)
	return nil
}

func (s *Server) didSave(_ *glsp.Context, params *protocol.DidSaveTextDocumentParams) error {
	// Buffer contents and on-disk file are identical after a save; the index
	// already reflects the latest keystroke via didChange. Re-publish
	// diagnostics anyway in case the client wants a fresh push.
	rel := s.uriToRelPath(params.TextDocument.URI)
	s.publishDiagnostics(rel, params.TextDocument.URI)
	return nil
}

func (s *Server) didClose(_ *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	// The editor is no longer authoritative for this file. Always re-read
	// from disk so we pick up anything that changed outside the editor while
	// the buffer was open — the watcher ignores those changes for open
	// buffers, so this close is our reconcile point.
	rel := s.uriToRelPath(params.TextDocument.URI)
	s.markClosed(rel)
	if s.project == nil {
		s.index.RemoveFile(rel)
		return nil
	}
	data, err := fs.ReadFile(s.project.FS, rel)
	if err != nil {
		s.index.RemoveFile(rel)
		return nil
	}
	s.index.SetFile(rel, string(data))
	return nil
}
