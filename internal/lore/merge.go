package lore

import (
	"sort"
	"strings"
)

// FileParse is the per-file result of scanning a markdown file into header
// candidates. It holds enough to drive both definition merging and reference
// scanning at the world level without re-reading the source.
type FileParse struct {
	Path        string
	Content     string
	Definitions []Definition
}

// Definition is a header candidate found in a file, along with its joined
// multi-line description. Typed definitions always become entities; untyped
// ones only do if they match a known entity at merge time.
type Definition struct {
	Line        int // 1-based line of the header
	Header      Header
	Description string
}

// ParseFile runs the per-file parse: walks lines, pulls out header candidates,
// and joins each header with its continuation lines until the next blank line.
// It does not consult any other file or world state.
func ParseFile(path, content string) *FileParse {
	fp := &FileParse{Path: path, Content: content}
	lines := strings.Split(content, "\n")

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || trimmed[0] == '#' {
			continue
		}

		header, ok := ParseHeader(trimmed)
		if !ok {
			continue
		}

		headerLine := i + 1

		var desc strings.Builder
		desc.WriteString(header.DescStart)
		for i+1 < len(lines) {
			i++
			next := strings.TrimSpace(lines[i])
			if next == "" {
				break
			}
			if desc.Len() > 0 {
				desc.WriteByte(' ')
			}
			desc.WriteString(next)
		}

		fp.Definitions = append(fp.Definitions, Definition{
			Line:        headerLine,
			Header:      header,
			Description: desc.String(),
		})
	}

	return fp
}

// Merge folds a set of per-file parses into a single World. Files are sorted
// by path to keep output order deterministic and chronologically meaningful
// (numeric prefixes win). Merge runs in three phases:
//
//  1. Establish entities from every typed header across every file.
//  2. Attach descriptions in file-then-scan order, resolving untyped headers
//     against the entity set built in phase 1.
//  3. Scan every line of every file for cross-references to known entities.
func Merge(files []*FileParse) *World {
	sorted := make([]*FileParse, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	world := NewWorld()

	// Phase 1: create entities from typed definitions.
	for _, fp := range sorted {
		for _, def := range fp.Definitions {
			if def.Header.Type == "" {
				continue
			}
			ent := findOrCreateEntity(world, def.Header.Name, def.Header.Type)
			if ent.Type == "" {
				ent.Type = def.Header.Type
			}
			for _, alias := range def.Header.Aliases {
				if !ent.HasAlias(alias) {
					ent.Aliases = append(ent.Aliases, alias)
				}
			}
		}
	}

	// Phase 2: append descriptions in scan order, attaching untyped headers to
	// whichever entity matches their name or alias.
	for _, fp := range sorted {
		for _, def := range fp.Definitions {
			var ent *Entity
			if def.Header.Type != "" {
				ent = findOrCreateEntity(world, def.Header.Name, def.Header.Type)
			} else {
				match, err := world.FindEntity(def.Header.Name)
				if err != nil {
					// Untyped header that matches no entity is just a colon
					// line in free text — skip it.
					continue
				}
				ent = match
			}
			if def.Description == "" {
				continue
			}
			ent.Descriptions = append(ent.Descriptions, Description{
				Text: def.Description,
				File: fp.Path,
				Line: def.Line,
			})
		}
	}

	// Phase 3: cross-reference scan over raw content.
	for _, fp := range sorted {
		findReferences(world, fp.Content, fp.Path)
	}

	return world
}

// findOrCreateEntity looks up an entity by name or alias, respecting type for
// disambiguation. If no match exists it appends a new entity to the world.
func findOrCreateEntity(world *World, name, entityType string) *Entity {
	for i := range world.Entities {
		ent := &world.Entities[i]
		if !strings.EqualFold(ent.Name, name) && !ent.NameMatchesAlias(name) {
			continue
		}
		if entityType == "" || ent.Type == "" || strings.EqualFold(ent.Type, entityType) {
			return ent
		}
	}
	world.Entities = append(world.Entities, Entity{Name: name})
	return &world.Entities[len(world.Entities)-1]
}

// findReferences scans file content for mentions of known entities and adds
// them to the world's reference index.
func findReferences(world *World, content, file string) {
	lines := strings.Split(content, "\n")

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		disambigMatched := make(map[int]bool)

		// Pass 1: disambiguated references like "Barovia (town)".
		for i := range world.Entities {
			ent := &world.Entities[i]
			if ent.Type == "" {
				continue
			}
			if containsDisambiguatedRef(trimmed, ent.Name, ent.Type) {
				disambigMatched[i] = true
				world.AddReference(ent.Name, Reference{
					File:         file,
					Line:         lineNum,
					SourceEntity: findEntityAtLine(world, file, lineNum),
					Context:      trimmed,
				})
			}
		}

		// Pass 2: plain name or alias matching, excluding anything already
		// counted as a disambiguated reference on this line.
		for i := range world.Entities {
			if disambigMatched[i] {
				continue
			}
			ent := &world.Entities[i]

			matched := false
			if ContainsIgnoreCase(trimmed, ent.Name) {
				if !isOnlyDisambiguated(trimmed, ent.Name, world) {
					matched = true
				}
			}
			if !matched {
				for _, alias := range ent.Aliases {
					if ContainsIgnoreCase(trimmed, alias) {
						matched = true
						break
					}
				}
			}
			if matched {
				world.AddReference(ent.Name, Reference{
					File:         file,
					Line:         lineNum,
					SourceEntity: findEntityAtLine(world, file, lineNum),
					Context:      trimmed,
				})
			}
		}
	}
}

// containsDisambiguatedRef checks if text contains "name (type)".
func containsDisambiguatedRef(haystack, name, entityType string) bool {
	lower := strings.ToLower(haystack)
	target := strings.ToLower(name) + " (" + strings.ToLower(entityType) + ")"
	return strings.Contains(lower, target)
}

// isOnlyDisambiguated reports whether every occurrence of name in text is
// already followed by a "(type)" suffix for some known entity type — i.e.
// there are no bare references on this line.
func isOnlyDisambiguated(text, name string, world *World) bool {
	lower := strings.ToLower(text)
	lowerName := strings.ToLower(name)

	idx := 0
	for {
		pos := strings.Index(lower[idx:], lowerName)
		if pos < 0 {
			return true
		}
		pos += idx

		after := pos + len(lowerName)
		if after+2 < len(lower) && lower[after] == ' ' && lower[after+1] == '(' {
			if closePos := strings.Index(lower[after+2:], ")"); closePos >= 0 {
				candidateType := lower[after+2 : after+2+closePos]
				if isKnownEntityType(world, candidateType) {
					idx = after + 2 + closePos + 1
					continue
				}
			}
		}
		return false
	}
}

func isKnownEntityType(world *World, candidateType string) bool {
	for _, ent := range world.Entities {
		if strings.EqualFold(ent.Type, candidateType) {
			return true
		}
	}
	return false
}

// findEntityAtLine returns the name of the entity whose header sits on the
// given file:line, if any.
func findEntityAtLine(world *World, file string, line int) string {
	for _, ent := range world.Entities {
		for _, desc := range ent.Descriptions {
			if desc.File == file && desc.Line == line {
				return ent.Name
			}
		}
	}
	return ""
}
