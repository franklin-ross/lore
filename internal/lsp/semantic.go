package lsp

import (
	"hash/fnv"
	"sort"
	"strings"

	"lore/internal/lore"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// paletteSize is the number of distinct colours in the entity palette.
// Token modifiers are used as a bitmask to encode the colour index.
// With 4 modifier bits we get 16 colours (indices 0-15).
const paletteSize = 16

// semanticTokensLegend returns the legend for our semantic tokens.
// We use a single token type "loreEntity" and modifier bits to encode colour index.
func semanticTokensLegend() protocol.SemanticTokensLegend {
	modifiers := make([]string, paletteSize)
	for i := range modifiers {
		modifiers[i] = modifierName(i)
	}
	return protocol.SemanticTokensLegend{
		TokenTypes:     []string{"loreEntity"},
		TokenModifiers: modifiers,
	}
}

func modifierName(index int) string {
	return "loreColour" + string(rune('A'+index))
}

// entityColourIndex computes a stable colour index for an entity based on its
// longest alias concatenated with its type.
func entityColourIndex(ent *lore.Entity) uint32 {
	longest := ent.Name
	for _, alias := range ent.Aliases {
		if len(alias) > len(longest) {
			longest = alias
		}
	}
	key := longest + ":" + ent.Type
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(key)))
	return h.Sum32() % paletteSize
}

type rawToken struct {
	line      uint32
	startChar uint32
	length    uint32
	tokenType uint32
	modifiers uint32
}

func (s *Server) semanticTokensFull(_ *glsp.Context, params *protocol.SemanticTokensParams) (*protocol.SemanticTokens, error) {
	world := s.world()
	content := s.getDocumentContent(params.TextDocument.URI)
	lines := strings.Split(content, "\n")

	var tokens []rawToken
	for i := range world.Entities {
		ent := &world.Entities[i]
		colourIdx := entityColourIndex(ent)
		modBits := uint32(1) << colourIdx
		names := allNames(ent)
		for lineIdx, line := range lines {
			for _, name := range names {
				for _, col := range findAllIgnoreCase(line, name) {
					tokens = append(tokens, rawToken{
						line:      uint32(lineIdx),
						startChar: uint32(col),
						length:    uint32(len(name)),
						tokenType: 0, // index into token types: "loreEntity"
						modifiers: modBits,
					})
				}
			}
		}
	}

	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].line != tokens[j].line {
			return tokens[i].line < tokens[j].line
		}
		return tokens[i].startChar < tokens[j].startChar
	})

	data := encodeTokens(tokens)
	return &protocol.SemanticTokens{Data: data}, nil
}

// encodeTokens converts absolute token positions to the delta-encoded format
// required by the LSP semantic tokens specification.
func encodeTokens(tokens []rawToken) []uint32 {
	data := make([]uint32, 0, len(tokens)*5)
	var prevLine, prevChar uint32
	for _, tok := range tokens {
		deltaLine := tok.line - prevLine
		var deltaChar uint32
		if deltaLine == 0 {
			deltaChar = tok.startChar - prevChar
		} else {
			deltaChar = tok.startChar
		}
		data = append(data, deltaLine, deltaChar, tok.length, tok.tokenType, tok.modifiers)
		prevLine = tok.line
		prevChar = tok.startChar
	}
	return data
}
