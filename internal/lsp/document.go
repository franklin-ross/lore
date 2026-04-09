package lsp

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) didOpen(_ *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	s.docs[params.TextDocument.URI] = params.TextDocument.Text
	s.reparse()
	return nil
}

func (s *Server) didChange(_ *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	// Full sync mode: last content change is the full document.
	if len(params.ContentChanges) > 0 {
		last := params.ContentChanges[len(params.ContentChanges)-1]
		if change, ok := last.(protocol.TextDocumentContentChangeEventWhole); ok {
			s.docs[params.TextDocument.URI] = change.Text
		}
	}
	return nil
}

func (s *Server) didSave(_ *glsp.Context, _ *protocol.DidSaveTextDocumentParams) error {
	s.reparse()
	return nil
}

func (s *Server) didClose(_ *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	delete(s.docs, params.TextDocument.URI)
	return nil
}
