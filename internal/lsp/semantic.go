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
	if world.Match == nil {
		return &protocol.SemanticTokens{Data: []uint32{}}, nil
	}

	content := s.getDocumentContent(params.TextDocument.URI)
	lines := strings.Split(content, "\n")

	// Rendered colour bits stay stable for the life of the world; compute
	// once outside the line loop.
	modBits := make([]uint32, len(world.Entities))
	for i := range world.Entities {
		modBits[i] = uint32(1) << entityColourIndex(&world.Entities[i])
	}

	var tokens []rawToken
	for lineIdx, line := range lines {
		lowerLine := strings.ToLower(line)
		for i := range world.Match.Entities {
			em := &world.Match.Entities[i]
			appendNameMatches(&tokens, lineIdx, lowerLine, em, world.Match.Types, modBits[i])
			for _, la := range em.LowerAliases {
				appendMatches(&tokens, lineIdx, lowerLine, la, modBits[i])
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

// appendNameMatches scans for occurrences of an entity's primary name and
// emits tokens, honouring `(type)` disambiguators the same way the reference
// scanner does. When a name occurrence is immediately followed by a matching
// `(type)` suffix, only the entity with that type emits a token there; an
// occurrence that carries a suffix for some *other* known type is skipped.
// Bare-name occurrences (no disambiguator) still emit for every entity
// sharing the name, so the resulting overlap reflects the genuine ambiguity.
func appendNameMatches(out *[]rawToken, lineIdx int, lowerLine string, em *lore.EntityMatch, allTypes map[string]struct{}, modBits uint32) {
	if em.LowerName == "" {
		return
	}
	for _, col := range lore.FindWordMatches(lowerLine, em.LowerName) {
		end := col + len(em.LowerName)
		if em.LowerType != "" && lore.MatchesTypeSuffix(lowerLine, end, em.LowerType) >= 0 {
			*out = append(*out, rawToken{
				line:      uint32(lineIdx),
				startChar: uint32(col),
				length:    uint32(len(em.LowerName)),
				tokenType: 0,
				modifiers: modBits,
			})
			continue
		}
		if lore.MatchesAnyTypeSuffix(lowerLine, end, allTypes) >= 0 {
			// Disambiguator points at a different entity — skip.
			continue
		}
		*out = append(*out, rawToken{
			line:      uint32(lineIdx),
			startChar: uint32(col),
			length:    uint32(len(em.LowerName)),
			tokenType: 0,
			modifiers: modBits,
		})
	}
}

// appendMatches scans one already-lowered line for a single needle and emits
// a rawToken per hit. Needle length is also the token length because byte
// offsets line up between the original and the lowered line for ASCII, and
// upper-case multi-byte characters have the same byte length under
// Go's strings.ToLower for every rune we care about here.
func appendMatches(out *[]rawToken, lineIdx int, lowerLine, lowerNeedle string, modBits uint32) {
	if lowerNeedle == "" {
		return
	}
	for _, col := range lore.FindWordMatches(lowerLine, lowerNeedle) {
		*out = append(*out, rawToken{
			line:      uint32(lineIdx),
			startChar: uint32(col),
			length:    uint32(len(lowerNeedle)),
			tokenType: 0,
			modifiers: modBits,
		})
	}
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
