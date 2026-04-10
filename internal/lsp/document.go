package lsp

import (
	"io/fs"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) didOpen(_ *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	rel := s.uriToRelPath(params.TextDocument.URI)
	s.index.SetFile(rel, params.TextDocument.Text)
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
	return nil
}

func (s *Server) didSave(_ *glsp.Context, _ *protocol.DidSaveTextDocumentParams) error {
	// Buffer contents and on-disk file are identical after a save; the index
	// already reflects the latest keystroke via didChange, so nothing to do.
	return nil
}

func (s *Server) didClose(_ *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	// Re-read the saved file from disk so the index stops reflecting any
	// unsaved edits that got discarded when the buffer closed.
	rel := s.uriToRelPath(params.TextDocument.URI)
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
