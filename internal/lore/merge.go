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
	Segments    []descSegment // maps joined-description byte ranges back to file lines/columns
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
		originalHeaderLine := lines[i]

		var desc strings.Builder
		var segments []descSegment

		// Segment 0: the DescStart portion of the header line.
		// Compute the column where DescStart begins in the original line.
		// The original line may be trimmed for header parsing, but we need the
		// column in the untrimmed line. Find the ':' and skip leading whitespace
		// after it to locate DescStart's first byte.
		headerCol := 0
		if colonIdx := strings.Index(originalHeaderLine, ":"); colonIdx >= 0 {
			afterColon := colonIdx + 1
			for afterColon < len(originalHeaderLine) && (originalHeaderLine[afterColon] == ' ' || originalHeaderLine[afterColon] == '\t') {
				afterColon++
			}
			headerCol = afterColon
		}
		if header.DescStart != "" {
			segments = append(segments, descSegment{
				joinedStart: 0,
				line:        headerLine,
				column:      headerCol,
			})
		}
		desc.WriteString(header.DescStart)

		for i+1 < len(lines) {
			i++
			rawLine := lines[i]
			next := strings.TrimSpace(rawLine)
			if next == "" {
				break
			}
			// Compute the joiner space offset before writing it.
			joinedStart := desc.Len()
			if desc.Len() > 0 {
				desc.WriteByte(' ')
				joinedStart++ // segment starts after the joiner space
			}
			// Column of the first non-whitespace byte in the original line.
			col := 0
			for col < len(rawLine) && (rawLine[col] == ' ' || rawLine[col] == '\t') {
				col++
			}
			segments = append(segments, descSegment{
				joinedStart: joinedStart,
				line:        i + 1, // 1-based
				column:      col,
			})
			desc.WriteString(next)
		}

		fp.Definitions = append(fp.Definitions, Definition{
			Line:        headerLine,
			Header:      header,
			Description: desc.String(),
			Segments:    segments,
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
			events, lexIssues := ParseDirectives(def.Description, fp.Path, def.Line)
			cleanText := stripDirectivesFromText(def.Description, events)
			translateSpans(events, lexIssues, def.Segments)
			ent.Descriptions = append(ent.Descriptions, Description{
				Text:      def.Description,
				CleanText: cleanText,
				File:      fp.Path,
				Line:      def.Line,
				Events:    events,
				LexIssues: lexIssues,
			})
		}
	}

	// Phase 2.5: resolve state for each entity from the accumulated descriptions.
	// Events and lexer issues are already attached per-description by Phase 2;
	// here we fold them in file order into the entity's resolved state.
	for i := range world.Entities {
		ent := &world.Entities[i]
		var events []StateEvent
		var lexIssues []StateIssue
		for _, desc := range ent.Descriptions {
			events = append(events, desc.Events...)
			lexIssues = append(lexIssues, desc.LexIssues...)
		}
		tags, fields, resolveIssues := ResolveState(events)
		ent.Tags = tags
		ent.Fields = fields
		ent.StateHistory = events
		ent.StateIssues = append(lexIssues, resolveIssues...)
	}

	// Build the lookup cache now that all entities (and their descriptions)
	// are established. Phase 3 relies on it, and external callers — semantic
	// token rendering, cursor lookup — read it directly off the world.
	world.Match = buildMatchIndex(world)

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
// them to the world's reference index. It reads the pre-lowered lookup data
// off world.Match, so the hot loop never re-lowercases an entity name.
func findReferences(world *World, content, file string) {
	mi := world.Match
	if mi == nil || len(mi.Entities) == 0 {
		return
	}

	lines := strings.Split(content, "\n")

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lowerTrimmed := strings.ToLower(trimmed)

		disambigMatched := make(map[int]bool)

		// Pass 1: disambiguated references like "Barovia (town)" — any number
		// of spaces between the name and the `(type)` are accepted.
		for i := range mi.Entities {
			em := &mi.Entities[i]
			if em.LowerType == "" {
				continue
			}
			for _, pos := range FindWordMatches(lowerTrimmed, em.LowerName) {
				end := pos + len(em.LowerName)
				if MatchesTypeSuffix(lowerTrimmed, end, em.LowerType) < 0 {
					continue
				}
				disambigMatched[i] = true
				world.AddReference(world.Entities[i].Name, Reference{
					File:         file,
					Line:         lineNum,
					SourceEntity: findEntityAtLine(world, file, lineNum),
					Context:      trimmed,
				})
				break
			}
		}

		// Pass 2: plain name or alias matching, excluding anything already
		// counted as a disambiguated reference on this line.
		for i := range mi.Entities {
			if disambigMatched[i] {
				continue
			}
			em := &mi.Entities[i]

			matched := false
			if HasWordMatch(lowerTrimmed, em.LowerName) {
				if !isOnlyDisambiguated(lowerTrimmed, em.LowerName, mi.Types) {
					matched = true
				}
			}
			if !matched {
				for _, la := range em.LowerAliases {
					if HasWordMatch(lowerTrimmed, la) {
						matched = true
						break
					}
				}
			}
			if matched {
				world.AddReference(world.Entities[i].Name, Reference{
					File:         file,
					Line:         lineNum,
					SourceEntity: findEntityAtLine(world, file, lineNum),
					Context:      trimmed,
				})
			}
		}
	}
}

// isOnlyDisambiguated reports whether every standalone-word occurrence of
// name in text is already followed by optional spaces and a "(type)"
// suffix for some known entity type. All inputs must already be lowercased.
func isOnlyDisambiguated(lowerText, lowerName string, lowerTypes map[string]struct{}) bool {
	for _, pos := range FindWordMatches(lowerText, lowerName) {
		end := pos + len(lowerName)
		if MatchesAnyTypeSuffix(lowerText, end, lowerTypes) < 0 {
			return false
		}
	}
	return true
}

// SkipSpaces returns the first index at or after pos that isn't a plain
// space character. Tabs and other whitespace are intentionally not skipped —
// a disambiguator is a user-facing inline construct, not a layout element.
func SkipSpaces(s string, pos int) int {
	for pos < len(s) && s[pos] == ' ' {
		pos++
	}
	return pos
}

// MatchesTypeSuffix checks whether lowerText at position pos is followed by
// any number of spaces and "(lowerType)", where spaces are also allowed
// directly after the opening paren and before the closing paren. Returns
// the index after the closing paren, or -1 if the pattern doesn't match.
func MatchesTypeSuffix(lowerText string, pos int, lowerType string) int {
	i := SkipSpaces(lowerText, pos)
	if i >= len(lowerText) || lowerText[i] != '(' {
		return -1
	}
	i = SkipSpaces(lowerText, i+1)
	if i+len(lowerType) > len(lowerText) || lowerText[i:i+len(lowerType)] != lowerType {
		return -1
	}
	i = SkipSpaces(lowerText, i+len(lowerType))
	if i >= len(lowerText) || lowerText[i] != ')' {
		return -1
	}
	return i + 1
}

// MatchesAnyTypeSuffix is like MatchesTypeSuffix but accepts any known
// entity type inside the parentheses. Spaces are allowed between the name
// and the paren, just inside the parens, and just before the close.
func MatchesAnyTypeSuffix(lowerText string, pos int, lowerTypes map[string]struct{}) int {
	i := SkipSpaces(lowerText, pos)
	if i >= len(lowerText) || lowerText[i] != '(' {
		return -1
	}
	typeStart := SkipSpaces(lowerText, i+1)
	closeOffset := strings.Index(lowerText[typeStart:], ")")
	if closeOffset < 0 {
		return -1
	}
	typeEnd := typeStart + closeOffset
	for typeEnd > typeStart && lowerText[typeEnd-1] == ' ' {
		typeEnd--
	}
	if _, ok := lowerTypes[lowerText[typeStart:typeEnd]]; !ok {
		return -1
	}
	return typeStart + closeOffset + 1
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
