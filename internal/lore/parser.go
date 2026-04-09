package lore

import (
	"io/fs"
	"strings"
)

// Parse reads all files in the project and produces a World.
func Parse(project *Project) (*World, error) {
	type fileData struct {
		name    string
		content string
	}

	var files []fileData
	for _, rel := range project.FilePaths {
		data, err := fs.ReadFile(project.FS, rel)
		if err != nil {
			continue
		}
		files = append(files, fileData{name: rel, content: string(data)})
	}

	world := NewWorld()

	// First pass: extract entity definitions.
	for _, f := range files {
		parseEntities(world, f.content, f.name)
	}

	// Second pass: find references to known entities.
	for _, f := range files {
		findReferences(world, f.content, f.name)
	}

	return world, nil
}

// parseEntities extracts entity definitions from file content.
func parseEntities(world *World, content, file string) {
	lines := strings.Split(content, "\n")

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || trimmed[0] == '#' {
			continue
		}

		header, ok := parseEntityHeader(trimmed)
		if !ok {
			// Check if this is a known-entity re-definition without type.
			// e.g. "Sildar: Was captured at Cragmaw Hideout."
			header, ok = parseKnownEntityHeader(world, trimmed)
			if !ok {
				continue
			}
		}

		headerLine := i + 1

		// Collect description until blank line.
		var desc strings.Builder
		desc.WriteString(header.descriptionStart)

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

		ent := findOrCreateEntity(world, header.name, header.entityType)

		if header.entityType != "" && ent.Type == "" {
			ent.Type = header.entityType
		}

		for _, alias := range header.aliases {
			if !ent.HasAlias(alias) {
				ent.Aliases = append(ent.Aliases, alias)
			}
		}

		if text := desc.String(); text != "" {
			ent.Descriptions = append(ent.Descriptions, Description{
				Text: text,
				File: file,
				Line: headerLine,
			})
		}
	}
}

type entityHeader struct {
	name             string
	entityType       string // empty if no type annotation
	aliases          []string
	descriptionStart string
}

// parseEntityHeader parses an entity header line that includes a (type) annotation.
// Returns ok=false if the line isn't an entity definition.
func parseEntityHeader(line string) (entityHeader, bool) {
	colonPos := strings.Index(line, ":")
	if colonPos < 0 {
		return entityHeader{}, false
	}
	headerPart := line[:colonPos]
	descStart := strings.TrimSpace(line[colonPos+1:])

	// Extract (type) from anywhere in the header.
	open := strings.Index(headerPart, "(")
	if open < 0 {
		return entityHeader{}, false
	}
	close := strings.Index(headerPart[open:], ")")
	if close < 0 {
		return entityHeader{}, false
	}
	close += open

	entityType := strings.TrimSpace(headerPart[open+1 : close])
	if entityType == "" {
		return entityHeader{}, false
	}

	beforeType := headerPart[:open]
	afterType := ""
	if close+1 < len(headerPart) {
		afterType = headerPart[close+1:]
	}

	// Split on | to get name and aliases. Name is the first non-empty segment.
	var canonical string
	var aliases []string

	for _, segment := range strings.Split(beforeType, "|") {
		trimmed := strings.TrimSpace(segment)
		if trimmed == "" {
			continue
		}
		if canonical == "" {
			canonical = trimmed
		} else {
			aliases = append(aliases, trimmed)
		}
	}

	for _, segment := range strings.Split(afterType, "|") {
		trimmed := strings.TrimSpace(segment)
		if trimmed == "" {
			continue
		}
		if canonical == "" {
			canonical = trimmed
		} else {
			aliases = append(aliases, trimmed)
		}
	}

	if canonical == "" {
		return entityHeader{}, false
	}

	return entityHeader{
		name:             canonical,
		entityType:       entityType,
		aliases:          aliases,
		descriptionStart: descStart,
	}, true
}

// parseKnownEntityHeader checks if a line is a re-definition of an already known entity
// (by name or alias), without a (type) annotation. e.g. "Sildar: Was captured."
func parseKnownEntityHeader(world *World, line string) (entityHeader, bool) {
	colonPos := strings.Index(line, ":")
	if colonPos < 0 {
		return entityHeader{}, false
	}

	headerPart := strings.TrimSpace(line[:colonPos])
	descStart := strings.TrimSpace(line[colonPos+1:])

	if headerPart == "" {
		return entityHeader{}, false
	}

	// Check if any known entity matches this name.
	for _, ent := range world.Entities {
		if strings.EqualFold(ent.Name, headerPart) || ent.NameMatchesAlias(headerPart) {
			return entityHeader{
				name:             headerPart,
				descriptionStart: descStart,
			}, true
		}
	}

	return entityHeader{}, false
}

// findOrCreateEntity looks up an entity by name/alias, respecting type for disambiguation.
// If no match is found, a new entity is appended to the world.
func findOrCreateEntity(world *World, name, entityType string) *Entity {
	for i := range world.Entities {
		ent := &world.Entities[i]
		nameMatch := strings.EqualFold(ent.Name, name) || ent.NameMatchesAlias(name)
		if !nameMatch {
			continue
		}
		if entityType == "" {
			return ent
		}
		if ent.Type == "" {
			return ent
		}
		if strings.EqualFold(ent.Type, entityType) {
			return ent
		}
	}

	world.Entities = append(world.Entities, Entity{Name: name})
	return &world.Entities[len(world.Entities)-1]
}

// findReferences scans file content for mentions of known entities.
func findReferences(world *World, content, file string) {
	lines := strings.Split(content, "\n")

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		disambigMatched := make(map[int]bool)

		// First pass: disambiguated references like "Barovia (town)".
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

		// Second pass: plain name/alias matching.
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

// containsDisambiguatedRef checks if text contains "name (type)" as a pattern.
func containsDisambiguatedRef(haystack, name, entityType string) bool {
	lower := strings.ToLower(haystack)
	lowerName := strings.ToLower(name)
	lowerType := strings.ToLower(entityType)
	target := lowerName + " (" + lowerType + ")"
	return strings.Contains(lower, target)
}

// isOnlyDisambiguated checks whether every occurrence of name in text is followed
// by " (type)" for some known entity type — meaning there are no bare references.
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
			closePos := strings.Index(lower[after+2:], ")")
			if closePos >= 0 {
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

// findEntityAtLine returns the name of the entity defined at the given file:line, if any.
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
