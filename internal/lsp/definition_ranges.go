package lsp

import (
	"encoding/json"
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// MethodLoreDefinitionRanges is the LSP request method the client invokes to
// retrieve coloured ranges covering every entity definition in a document.
// The response is consumed by the editor extension to draw whole-span
// decorations (underline, faded background) over each definition.
const MethodLoreDefinitionRanges = "lore/definitionRanges"

// DefinitionRangesParams identifies the document the client wants definition
// ranges for. Mirrors the standard textDocument identifier shape so the
// client can serialise it the same way as built-in requests.
type DefinitionRangesParams struct {
	TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`
}

// DefinitionRange is one decorated extent — the LSP range covering a single
// header definition or inline aside, plus the palette index the extension
// should use for its colour. The index is 0..paletteSize-1 and matches the
// loreColour modifier bit position used in semantic tokens.
type DefinitionRange struct {
	Range       protocol.Range `json:"range"`
	ColourIndex uint32         `json:"colourIndex"`
}

// DefinitionRangesResult is the response body for MethodLoreDefinitionRanges.
type DefinitionRangesResult struct {
	Ranges []DefinitionRange `json:"ranges"`
}

// definitionRanges produces every definition span in the requested document.
// It resolves the document's owning project, walks each entity's descriptions,
// filters to those originating in this file, and converts the 1-based /
// byte-column lore positions to LSP's 0-based coordinates. Colour index
// reuses entityColourIndex so the decoration colour matches the entity's
// semantic-token colour.
func (s *Server) definitionRanges(params *DefinitionRangesParams) (*DefinitionRangesResult, error) {
	ps, relPath := s.projectForURI(params.TextDocument.URI)
	if ps == nil {
		return &DefinitionRangesResult{}, nil
	}
	world := ps.world()

	// StartColumn/EndColumn are stored as UTF-8 byte offsets, but the
	// client decorates ranges using LSP positions (UTF-16). Convert per
	// definition using the live document content.
	content := s.getDocumentContent(params.TextDocument.URI)
	lines := strings.Split(content, "\n")
	lineAt := func(idx int) string {
		if idx < 0 || idx >= len(lines) {
			return ""
		}
		return lines[idx]
	}

	out := DefinitionRangesResult{}
	for i := range world.Entities {
		ent := &world.Entities[i]
		colour := entityColourIndex(ent)
		for _, desc := range ent.Descriptions {
			if desc.File != relPath {
				continue
			}
			startLine := desc.Line - 1
			endLine := desc.EndLine - 1
			out.Ranges = append(out.Ranges, DefinitionRange{
				Range: protocol.Range{
					Start: protocol.Position{
						Line:      uint32(startLine),
						Character: utf16UnitsForBytes(lineAt(startLine), desc.StartColumn),
					},
					End: protocol.Position{
						Line:      uint32(endLine),
						Character: utf16UnitsForBytes(lineAt(endLine), desc.EndColumn),
					},
				},
				ColourIndex: colour,
			})
		}
	}
	return &out, nil
}

// loreHandler wraps the standard protocol.Handler so requests for our
// custom methods are dispatched before falling through to glsp's switch.
type loreHandler struct {
	inner  *protocol.Handler
	server *Server
}

// Handle dispatches one JSON-RPC request. The four return values come from
// the glsp.Handler interface (common.go in tliron/glsp):
//
//	result       — value marshalled back as the JSON-RPC `result`. nil for
//	               notifications, errors, or when the method isn't ours.
//	validMethod  — true when ctx.Method matched a handler in this dispatcher.
//	               false tells the caller to fall through; glsp turns an
//	               unmatched method into a "method not found" response.
//	validParams  — true when ctx.Params unmarshalled cleanly. false produces
//	               an "invalid params" response.
//	err          — handler-level failure (panics, internal errors), distinct
//	               from validation; non-nil becomes "internal error".
//
// Pattern per case: params parse fail returns (nil, true, false, nil) —
// valid method, bad params. Success returns (result, true, true, err) and
// lets the handler's err propagate. Unknown methods fall through to
// h.inner.Handle so glsp can serve the built-in LSP surface.
func (h *loreHandler) Handle(ctx *glsp.Context) (any, bool, bool, error) {
	switch ctx.Method {
	case MethodLoreDefinitionRanges:
		var p DefinitionRangesParams
		if err := json.Unmarshal(ctx.Params, &p); err != nil {
			return nil, true, false, nil
		}
		result, err := h.server.definitionRanges(&p)
		return result, true, true, err
	case MethodLoreEntityList:
		p, err := decodeEntityList(ctx.Params)
		if err != nil {
			return nil, true, false, nil
		}
		result, err := h.server.entityList(p)
		return result, true, true, err
	case MethodLoreEntityDetails:
		p, err := decodeEntityDetails(ctx.Params)
		if err != nil {
			return nil, true, false, nil
		}
		result, err := h.server.entityDetails(p)
		return result, true, true, err
	case MethodLoreTypeDetails:
		p, err := decodeTypeDetails(ctx.Params)
		if err != nil {
			return nil, true, false, nil
		}
		result, err := h.server.typeDetails(p)
		return result, true, true, err
	case MethodLoreLookup:
		p, err := decodeLookup(ctx.Params)
		if err != nil {
			return nil, true, false, nil
		}
		result, err := h.server.lookup(p)
		return result, true, true, err
	case MethodLoreGraph:
		p, err := decodeGraph(ctx.Params)
		if err != nil {
			return nil, true, false, nil
		}
		result, err := h.server.graph(p)
		return result, true, true, err
	}
	return h.inner.Handle(ctx)
}
