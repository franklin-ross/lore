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
// Token modifiers are used as a bitmask to encode the colour index, one bit
// per colour. 26 colours map cleanly to the letters A–Z used in modifier
// names (loreColourA..loreColourZ) and fit well inside LSP's 32-bit
// tokenModifiers field.
const paletteSize = 26

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
	uri := params.TextDocument.URI
	ps, _ := s.projectForURI(uri)
	if ps == nil {
		return &protocol.SemanticTokens{Data: []uint32{}}, nil
	}
	world := ps.world()
	if world.Match == nil {
		return &protocol.SemanticTokens{Data: []uint32{}}, nil
	}

	if ps.semanticCacheWorld != world {
		// World pointer changed (any Index mutation forces a rebuild via
		// Index.World), so every cached entry is stale. Drop them all
		// rather than tracking per-URI generations.
		ps.semanticCacheWorld = world
		ps.semanticCache = nil
	}
	if cached, ok := ps.semanticCache[uri]; ok {
		return &protocol.SemanticTokens{Data: cached}, nil
	}

	content := s.getDocumentContent(uri)
	lines := strings.Split(content, "\n")

	// Rendered colour bits stay stable for the life of the world; compute
	// once outside the line loop.
	modBits := make([]uint32, len(world.Entities))
	for i := range world.Entities {
		modBits[i] = uint32(1) << entityColourIndex(&world.Entities[i])
	}

	var tokens []rawToken
	for lineIdx, line := range lines {
		before := len(tokens)
		// One scan per line via the shared scanner: same word-boundary
		// rules, disambiguator handling, and span dedup the colouriser /
		// reference index use, so highlights stay consistent with hover
		// and wiki output. The scanner uses first-byte bucketing so the
		// per-line cost is roughly proportional to text length rather
		// than (text × entities).
		for _, sp := range lore.ScanEntities(world, line, false) {
			tokens = append(tokens, rawToken{
				line:      uint32(lineIdx),
				startChar: uint32(sp.Start),
				length:    uint32(sp.End - sp.Start),
				tokenType: 0,
				modifiers: modBits[sp.EntityIdx],
			})
		}
		// Match positions are byte offsets into the line; LSP semantic
		// tokens are encoded in UTF-16 code units. On pure-ASCII lines
		// (the common case for English prose) the two agree, so skip
		// the per-token rune walk. Only lines containing non-ASCII
		// pay the conversion cost.
		if before < len(tokens) && !isASCII(line) {
			for j := before; j < len(tokens); j++ {
				startByte := int(tokens[j].startChar)
				endByte := startByte + int(tokens[j].length)
				startU16 := utf16UnitsForBytes(line, startByte)
				endU16 := utf16UnitsForBytes(line, endByte)
				tokens[j].startChar = startU16
				tokens[j].length = endU16 - startU16
			}
		}
	}

	tokens = resolveOverlaps(tokens)

	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].line != tokens[j].line {
			return tokens[i].line < tokens[j].line
		}
		return tokens[i].startChar < tokens[j].startChar
	})

	data := encodeTokens(tokens)
	if ps.semanticCache == nil {
		ps.semanticCache = make(map[string][]uint32)
	}
	ps.semanticCache[uri] = data
	return &protocol.SemanticTokens{Data: data}, nil
}

// resolveOverlaps drops tokens whose span overlaps a longer token on the same
// line. LSP forbids overlapping semantic tokens; without this, e.g. a "Vallaki"
// match at column 0 and a "Vallaki Cathedral" match at column 0 both emit and
// the client (VSCode) keeps only the shorter one, miscolouring the tail.
// Longest-wins matches the hover/definition behaviour in findEntityAtPosition.
func resolveOverlaps(tokens []rawToken) []rawToken {
	if len(tokens) < 2 {
		return tokens
	}
	// Sort by line, then length desc, then start asc, so the longest token at
	// any contested position is considered first and claims its range.
	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].line != tokens[j].line {
			return tokens[i].line < tokens[j].line
		}
		if tokens[i].length != tokens[j].length {
			return tokens[i].length > tokens[j].length
		}
		return tokens[i].startChar < tokens[j].startChar
	})

	kept := tokens[:0]
	for _, tok := range tokens {
		end := tok.startChar + tok.length
		overlaps := false
		for _, k := range kept {
			if k.line != tok.line {
				continue
			}
			kEnd := k.startChar + k.length
			if tok.startChar < kEnd && k.startChar < end {
				overlaps = true
				break
			}
		}
		if !overlaps {
			kept = append(kept, tok)
		}
	}
	return kept
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
