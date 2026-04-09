package lore

import (
	"errors"
	"fmt"
	"strings"
)

// World holds the parsed entity graph and reference index.
type World struct {
	Entities   []Entity
	References map[string][]Reference
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
		if ref.SourceEntity != "" && isSameEntity(ent, ref.SourceEntity) {
			continue
		}
		filtered = append(filtered, ref)
	}
	return filtered
}

// isSameEntity reports whether sourceName matches the entity's canonical name or any alias.
func isSameEntity(ent *Entity, sourceName string) bool {
	return strings.EqualFold(ent.Name, sourceName) || ent.NameMatchesAlias(sourceName)
}

// Search finds entity descriptions containing the query text (case-insensitive).
func (w *World) Search(query string) []SearchResult {
	var results []SearchResult
	for _, ent := range w.Entities {
		for _, desc := range ent.Descriptions {
			if ContainsIgnoreCase(desc.Text, query) {
				results = append(results, SearchResult{
					File:    desc.File,
					Line:    desc.Line,
					Context: desc.Text,
				})
			}
		}
	}
	return results
}

// Check reports issues such as undefined references. Currently a placeholder.
func (w *World) Check() []Issue {
	return nil
}
