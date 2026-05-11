package lore

import (
	"errors"
	"fmt"
	"strings"
)

// World holds the parsed entity graph and reference index. Every element of
// Entities has a non-empty Type after Merge returns — untyped header lines are
// either attached to an existing typed entity or dropped.
type World struct {
	Entities   []Entity
	References map[string][]Reference
	// Match is a pre-lowered lookup cache built by Merge. It stays in lock
	// step with Entities — rebuilt whenever Merge runs — so callers that
	// scan text for entity mentions don't need to re-lowercase on every
	// iteration of their hot loop.
	Match *MatchIndex
	// Files retains the raw content of every parsed file so Search can scan
	// free text alongside entity descriptions — narrative prose between
	// definitions is part of the searchable surface per docs/design.md.
	Files []FileSource
	// FileIssues holds diagnostics not tied to a specific entity — most
	// notably untyped colon-line typos: a `Sildar Hallwinder:` block that
	// matches no known entity but is close to one. Phase 2 of Merge would
	// silently drop these (they look like prose); FileIssues lets the LSP
	// and `check` surface them so authors notice the gap.
	FileIssues []StateIssue
}

// FileSource is one parsed file's path and raw content, kept on the World so
// full-text search can scan prose outside any entity definition.
type FileSource struct {
	Path    string
	Content string
}

// NewWorld creates an empty World.
func NewWorld() *World {
	return &World{
		References: make(map[string][]Reference),
	}
}

// AddReference records a reference to the named entity.
func (w *World) AddReference(entityName string, ref Reference) {
	w.References[entityName] = append(w.References[entityName], ref)
}

var ErrNotFound = errors.New("entity not found")

// AmbiguousError is returned when a bare name matches multiple entities.
type AmbiguousError struct {
	Matches []*Entity
}

func (e *AmbiguousError) Error() string {
	names := make([]string, len(e.Matches))
	for i, m := range e.Matches {
		if m.Type != "" {
			names[i] = fmt.Sprintf("%s (%s)", m.Name, m.Type)
		} else {
			names[i] = m.Name
		}
	}
	return fmt.Sprintf("ambiguous: %s", strings.Join(names, ", "))
}

// FindEntity looks up an entity by name or alias, supporting disambiguation syntax.
func (w *World) FindEntity(name string) (*Entity, error) {
	// Try disambiguation syntax first: "Barovia (town)".
	if disambig, ok := ParseDisambiguation(name); ok {
		for i := range w.Entities {
			ent := &w.Entities[i]
			if ent.Type == "" || !strings.EqualFold(ent.Type, disambig.Type) {
				continue
			}
			if strings.EqualFold(ent.Name, disambig.Name) || ent.NameMatchesAlias(disambig.Name) {
				return ent, nil
			}
		}
		return nil, ErrNotFound
	}

	// Collect all matches to detect ambiguity.
	var matches []*Entity
	for i := range w.Entities {
		ent := &w.Entities[i]
		if strings.EqualFold(ent.Name, name) || ent.NameMatchesAlias(name) {
			matches = append(matches, ent)
		}
	}

	switch len(matches) {
	case 0:
		return nil, ErrNotFound
	case 1:
		return matches[0], nil
	default:
		return nil, &AmbiguousError{Matches: matches}
	}
}

// GetReferences returns all references to an entity (case-insensitive name lookup),
// excluding self-references (where the entity's own definition mentions itself).
func (w *World) GetReferences(name string) []Reference {
	var raw []Reference
	if refs, ok := w.References[name]; ok {
		raw = refs
	} else {
		for key, refs := range w.References {
			if strings.EqualFold(key, name) {
				raw = refs
				break
			}
		}
	}
	if len(raw) == 0 {
		return nil
	}

	// Find the entity so we can match against its canonical name and aliases.
	ent, err := w.FindEntity(name)
	if err != nil {
		return raw
	}

	filtered := make([]Reference, 0, len(raw))
	for _, ref := range raw {
		if ref.SourceEntity != "" && isSameEntity(ent, ref.SourceEntity, ref.SourceType) {
			continue
		}
		filtered = append(filtered, ref)
	}
	return filtered
}

// isSameEntity reports whether (sourceName, sourceType) refers to the
// same entity. When sourceType is non-empty it must also match — so
// cross-references between two entities sharing a bare name (e.g.
// "Barovia (town)" mentioning "Barovia (country)") aren't mistaken for
// self-references.
func isSameEntity(ent *Entity, sourceName, sourceType string) bool {
	if !strings.EqualFold(ent.Name, sourceName) && !ent.NameMatchesAlias(sourceName) {
		return false
	}
	if sourceType == "" || ent.Type == "" {
		return true
	}
	return strings.EqualFold(ent.Type, sourceType)
}

// Search finds lines containing the query text (case-insensitive) across
// every parsed file. Both entity descriptions and free text prose are
// scanned, since narrative outside any definition is part of the
// searchable surface (docs/design.md, docs/format.md). Returns one result
// per matching line, sorted by file then line.
func (w *World) Search(query string) []SearchResult {
	if query == "" {
		return nil
	}
	var results []SearchResult
	for _, f := range w.Files {
		lines := strings.Split(f.Content, "\n")
		for i, line := range lines {
			if !ContainsIgnoreCase(line, query) {
				continue
			}
			results = append(results, SearchResult{
				File:    f.Path,
				Line:    i + 1,
				Context: strings.TrimSpace(line),
			})
		}
	}
	return results
}

// Check reports issues such as undefined references and entity state problems.
func (w *World) Check() []Issue {
	var issues []Issue
	for _, ent := range w.Entities {
		for _, si := range ent.StateIssues {
			issues = append(issues, Issue{
				File:    si.Span.File,
				Line:    si.Span.Line,
				Message: si.Message,
			})
		}
	}
	for _, fi := range w.FileIssues {
		issues = append(issues, Issue{
			File:    fi.Span.File,
			Line:    fi.Span.Line,
			Message: fi.Message,
		})
	}
	return issues
}
