package lsp

import (
	"encoding/json"
	"sort"
	"strings"

	"lore/internal/lore"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// MethodLoreGraph is the LSP request the knowledge-graph view invokes to
// fetch the entity reference graph for a project (or the merged workspace
// graph when no project is specified). The response carries every entity
// as a node and one directed edge per definition reference (mention of
// one entity inside another's description block).
const MethodLoreGraph = "lore/graph"

// GraphParams scopes the request to a single project. When TextDocument
// is unset the request returns the merged workspace graph — same fallback
// the entity list uses so the graph can render before any markdown
// buffer is focused.
type GraphParams struct {
	TextDocument *protocol.TextDocumentIdentifier `json:"textDocument,omitempty"`
}

// GraphNode is one entity vertex. Label is the same disambiguation-aware
// string entityLabel produces for the wiki, so edge endpoints and node
// ids line up across requests. ColourIndex matches the entity's
// semantic-token colour index.
type GraphNode struct {
	Label       string `json:"label"`
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	ColourIndex uint32 `json:"colourIndex"`
}

// GraphDefEdge is a directed edge from one entity's definition block to
// another entity it references. Count aggregates duplicate mentions of
// the same target inside the same source.
type GraphDefEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}

type GraphResult struct {
	Nodes    []GraphNode    `json:"nodes"`
	DefEdges []GraphDefEdge `json:"defEdges"`
	// Message, when set, is a human-readable explanation for an empty
	// result that the client should surface in place of a blank canvas
	// (e.g. multiple projects with no active editor to disambiguate).
	Message string `json:"message,omitempty"`
}

// graph builds the project graph response.
func (s *Server) graph(params *GraphParams) (*GraphResult, error) {
	world := s.graphWorld(params)
	if world == nil {
		return &GraphResult{
			Nodes:    []GraphNode{},
			DefEdges: []GraphDefEdge{},
			Message:  noScopeMessage(s, params),
		}, nil
	}
	return buildGraphResult(world), nil
}

// noScopeMessage explains why graphWorld returned nil so the webview can
// show a useful hint instead of an empty canvas.
func noScopeMessage(s *Server, params *GraphParams) string {
	if len(s.projects) == 0 {
		return "No lore.toml found in this workspace."
	}
	if params != nil && params.TextDocument != nil && params.TextDocument.URI != "" {
		return "This file isn't in any lore project."
	}
	if len(s.projects) > 1 {
		return "Open a lore file to see its project's graph."
	}
	return ""
}

// graphWorld picks the world to graph: the URI's owning project when
// scoped. With no URI, the only sensible default is "the sole project" —
// merging unrelated projects produces a chimera (duplicate-named entities
// collide, refs cross campaigns) so we refuse and let the caller render
// "open a lore file to see its graph". Returns nil when no scope can be
// resolved.
func (s *Server) graphWorld(params *GraphParams) *lore.World {
	if params != nil && params.TextDocument != nil && params.TextDocument.URI != "" {
		ps, _ := s.projectForURI(params.TextDocument.URI)
		if ps == nil {
			return nil
		}
		return ps.world()
	}
	if len(s.projects) == 1 {
		for _, ps := range s.projects {
			return ps.world()
		}
	}
	return nil
}

func buildGraphResult(world *lore.World) *GraphResult {
	return &GraphResult{
		Nodes:    buildGraphNodes(world),
		DefEdges: buildDefEdges(world),
	}
}

func buildGraphNodes(world *lore.World) []GraphNode {
	nodes := make([]GraphNode, 0, len(world.Entities))
	for i := range world.Entities {
		ent := &world.Entities[i]
		nodes = append(nodes, GraphNode{
			Label:       entityLabel(world, ent.Name, ent.Type),
			Name:        ent.Name,
			Type:        ent.Type,
			ColourIndex: entityColourIndex(ent),
		})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Label < nodes[j].Label })
	return nodes
}

func buildDefEdges(world *lore.World) []GraphDefEdge {
	type pair struct{ from, to string }
	counts := make(map[pair]int)
	for target, refs := range world.References {
		for _, r := range refs {
			if r.SourceEntity == "" {
				continue
			}
			if strings.EqualFold(r.SourceEntity, target) {
				// Self-reference: header line restates the entity's own name.
				if r.SourceType == "" || r.TargetType == "" || strings.EqualFold(r.SourceType, r.TargetType) {
					continue
				}
			}
			from := entityLabel(world, r.SourceEntity, r.SourceType)
			to := entityLabel(world, target, r.TargetType)
			if from == "" || to == "" {
				continue
			}
			counts[pair{from, to}]++
		}
	}
	out := make([]GraphDefEdge, 0, len(counts))
	for p, c := range counts {
		out = append(out, GraphDefEdge{From: p.from, To: p.to, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

// decodeGraph unmarshals the request payload. Used by loreHandler.
func decodeGraph(raw json.RawMessage) (*GraphParams, error) {
	if len(raw) == 0 {
		return &GraphParams{}, nil
	}
	p := &GraphParams{}
	if err := json.Unmarshal(raw, p); err != nil {
		return nil, err
	}
	return p, nil
}
