package lsp

import (
	"encoding/json"

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
	Range        protocol.Range `json:"range"`
	ColourIndex  uint32         `json:"colourIndex"`
}

// DefinitionRangesResult is the response body for MethodLoreDefinitionRanges.
type DefinitionRangesResult struct {
	Ranges []DefinitionRange `json:"ranges"`
}

// definitionRanges produces every definition span in the requested document.
// It walks each entity's descriptions, filters to those originating in this
// file, and converts the 1-based / byte-column lore positions to LSP's
// 0-based coordinates. Colour index reuses entityColourIndex so the
// decoration colour matches the entity's semantic-token colour.
func (s *Server) definitionRanges(params *DefinitionRangesParams) (*DefinitionRangesResult, error) {
	world := s.world()
	if world == nil {
		return &DefinitionRangesResult{}, nil
	}
	relPath := s.uriToRelPath(params.TextDocument.URI)

	out := DefinitionRangesResult{}
	for i := range world.Entities {
		ent := &world.Entities[i]
		colour := entityColourIndex(ent)
		for _, desc := range ent.Descriptions {
			if desc.File != relPath {
				continue
			}
			out.Ranges = append(out.Ranges, DefinitionRange{
				Range: protocol.Range{
					Start: protocol.Position{
						Line:      uint32(desc.Line - 1),
						Character: uint32(desc.StartColumn),
					},
					End: protocol.Position{
						Line:      uint32(desc.EndLine - 1),
						Character: uint32(desc.EndColumn),
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

func (h *loreHandler) Handle(ctx *glsp.Context) (any, bool, bool, error) {
	if ctx.Method == MethodLoreDefinitionRanges {
		var p DefinitionRangesParams
		if err := json.Unmarshal(ctx.Params, &p); err != nil {
			return nil, true, false, nil
		}
		result, err := h.server.definitionRanges(&p)
		return result, true, true, err
	}
	if ctx.Method == MethodLoreEntityList {
		p, err := decodeEntityList(ctx.Params)
		if err != nil {
			return nil, true, false, nil
		}
		result, err := h.server.entityList(p)
		return result, true, true, err
	}
	return h.inner.Handle(ctx)
}
