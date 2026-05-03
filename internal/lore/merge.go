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

	// Span covers the full source extent of the definition — for header
	// lines, from column 0 of Line through the end of the last continuation
	// line; for inline asides, from the '(' to one past the matching ')'.
	StartColumn int
	EndLine     int // 1-based, inclusive
	EndColumn   int // 0-based byte column on EndLine, exclusive

	// IsAside flags `(Name: body)` constructs so reference attribution
	// can treat the aside header as outer-prose territory. Header-line
	// definitions stay false.
	IsAside bool

	// BodyColumn is the byte column on Line where the description body
	// begins (after `: ` on the header line). Sourced from segments[0]
	// when present; otherwise falls back to StartColumn.
	BodyColumn int
}

// ParseFile runs the per-file parse: walks lines, pulls out header candidates,
// and joins each header with its continuation lines until the next blank line.
// It also extracts inline asides — `(Name: body)` constructs in prose — and
// adds each one as a synthetic Definition so they participate in merge the
// same way a normal header line would. It does not consult any other file or
// world state.
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
		endLine := headerLine
		endCol := len(originalHeaderLine)

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
			// Compute the joiner offset before writing it. Joining with `\n`
			// (not a space) lets the directive scanner treat line breaks as
			// terminators, so a missing comma at end-of-line doesn't swallow
			// the next line's content into the list value.
			joinedStart := desc.Len()
			if desc.Len() > 0 {
				desc.WriteByte('\n')
				joinedStart++ // segment starts after the joiner newline
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
			endLine = i + 1
			endCol = len(rawLine)
		}

		fp.Definitions = append(fp.Definitions, Definition{
			Line:        headerLine,
			Header:      header,
			Description: desc.String(),
			Segments:    segments,
			StartColumn: 0,
			EndLine:     endLine,
			EndColumn:   endCol,
			BodyColumn:  headerCol,
		})
	}

	// Inline asides — `(Name: body)` constructs found anywhere in prose —
	// are appended as synthetic Definitions and re-sorted into source-line
	// order. A stable sort keeps the line-header definition ahead of any
	// aside that happens to sit on the same line.
	for _, hit := range extractInlineAsides(content) {
		fp.Definitions = append(fp.Definitions, Definition{
			Line:        hit.Line,
			Header:      hit.Header,
			Description: hit.Body,
			Segments: []descSegment{{
				joinedStart: 0,
				line:        hit.BodyLine,
				column:      hit.BodyColumn,
			}},
			StartColumn: hit.OpenColumn,
			EndLine:     hit.Line,
			EndColumn:   hit.CloseColumn,
			IsAside:     true,
			BodyColumn:  hit.BodyColumn,
		})
	}
	sort.SliceStable(fp.Definitions, func(i, j int) bool {
		return fp.Definitions[i].Line < fp.Definitions[j].Line
	})

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
			// Inline asides — `(Name: ...)` — are extracted as synthetic
			// definitions by ParseFile and routed to their named subject.
			// To avoid double-counting, blank the aside ranges out of the
			// body before scanning for directives so the owner's event
			// list reflects only directives written outside any aside.
			parseBody := blankInlineAsides(def.Description)
			events, lexIssues := ParseDirectives(parseBody, fp.Path, def.Line)
			cleanText := stripDirectivesFromText(def.Description, events)
			cleanText = replaceInlineAsidesWithName(cleanText, world)
			translateSpans(events, lexIssues, def.Segments)
			ent.Descriptions = append(ent.Descriptions, Description{
				Text:        def.Description,
				CleanText:   cleanText,
				File:        fp.Path,
				Line:        def.Line,
				Events:      events,
				LexIssues:   lexIssues,
				StartColumn: def.StartColumn,
				EndLine:     def.EndLine,
				EndColumn:   def.EndColumn,
				IsAside:     def.IsAside,
				BodyColumn:  def.BodyColumn,
			})
		}
	}

	// Phase 2.5: resolve state for each entity by folding events from its
	// descriptions in attachment order, which matches sorted file/line
	// order across both directly-authored and inline-aside-derived
	// descriptions.
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
		lowerLine := strings.ToLower(line)

		leadingWS := 0
		for leadingWS < len(line) && (line[leadingWS] == ' ' || line[leadingWS] == '\t') {
			leadingWS++
		}

		// Collect every candidate match on this line across every
		// entity, then run one global containment-dedup pass. Without a
		// global pass, a longer canonical name like "Vistani Camp near
		// Vallaki" wouldn't suppress sub-spans from the "Vistani" and
		// "Vallaki" entities — each would still emit its own ref and
		// the wiki would show duplicate-looking rows for the same word.
		type cand struct {
			start, end int
			entityIdx  int
		}
		var all []cand
		for i := range mi.Entities {
			em := &mi.Entities[i]
			for _, s := range candidateSpans(lowerLine, em, mi.Types) {
				all = append(all, cand{s.start, s.end, i})
			}
		}
		if len(all) == 0 {
			continue
		}
		sort.Slice(all, func(a, b int) bool {
			if all[a].start != all[b].start {
				return all[a].start < all[b].start
			}
			if (all[a].end - all[a].start) != (all[b].end - all[b].start) {
				return (all[a].end - all[a].start) > (all[b].end - all[b].start)
			}
			return all[a].entityIdx < all[b].entityIdx
		})

		kept := all[:0]
		for _, m := range all {
			skip := false
			for _, k := range kept {
				if k.start > m.start || m.end > k.end {
					continue
				}
				equal := k.start == m.start && k.end == m.end
				if equal {
					// Two entities with the same name claim the same
					// span — ambiguous reference, both kept. Within
					// the same entity (alias literally equal to its
					// own name) drop the duplicate.
					if k.entityIdx == m.entityIdx {
						skip = true
						break
					}
					continue
				}
				// m is strictly contained in k — drop. Covers both
				// within-entity overlap (alias inside name, plain
				// name inside disambig form) and cross-entity overlap
				// (Vistani inside Vistani Camp near Vallaki).
				skip = true
				break
			}
			if !skip {
				kept = append(kept, m)
			}
		}

		for _, m := range kept {
			matchInTrimmed := max(m.start-leadingWS, 0)
			world.AddReference(world.Entities[m.entityIdx].Name, Reference{
				File:         file,
				Line:         lineNum,
				SourceEntity: findEntityAtMention(world, file, lineNum, m.start),
				Context:      trimContextBeforeMatch(trimmed, matchInTrimmed, 4),
			})
		}
	}
}

// trimContextBeforeMatch returns `text` starting at most `wordsBefore`
// whitespace-separated tokens before `matchPos`, prefixed with "… " when
// any leading bytes were dropped. Used to keep reference previews short
// enough that several refs in the same sentence don't render as visually
// identical rows in the wiki. Punctuation stays attached to the word it
// sits beside (only space and tab count as separators).
func trimContextBeforeMatch(text string, matchPos, wordsBefore int) string {
	if matchPos <= 0 || wordsBefore <= 0 {
		return text
	}
	var wordStarts []int
	inWord := false
	for i := 0; i < len(text); i++ {
		isWord := text[i] != ' ' && text[i] != '\t'
		if isWord && !inWord {
			wordStarts = append(wordStarts, i)
		}
		inWord = isWord
	}
	matchWord := -1
	for i, s := range wordStarts {
		if s > matchPos {
			break
		}
		matchWord = i
	}
	if matchWord < 0 {
		return text
	}
	cutWord := matchWord - wordsBefore
	if cutWord <= 0 {
		return text
	}
	cutAt := wordStarts[cutWord]
	if cutAt == 0 {
		return text
	}
	return "… " + text[cutAt:]
}

// matchSpan is a [start, end) byte range for one candidate match within
// a line, used internally by candidateSpans.
type matchSpan struct{ start, end int }

// candidateSpans returns every potential mention of `em` on the line:
// disambiguated form (Name (type)), plain name, and each alias. The
// caller does the global containment dedup so this function intentionally
// does not collapse overlapping spans on its own.
func candidateSpans(lowerLine string, em *EntityMatch, knownTypes map[string]struct{}) []matchSpan {
	var out []matchSpan

	if em.LowerType != "" {
		for _, pos := range FindWordMatches(lowerLine, em.LowerName) {
			end := pos + len(em.LowerName)
			after := MatchesTypeSuffix(lowerLine, end, em.LowerType)
			if after < 0 {
				continue
			}
			out = append(out, matchSpan{pos, after})
		}
	}

	for _, pos := range FindWordMatches(lowerLine, em.LowerName) {
		end := pos + len(em.LowerName)
		if MatchesAnyTypeSuffix(lowerLine, end, knownTypes) >= 0 {
			continue
		}
		out = append(out, matchSpan{pos, end})
	}

	for _, la := range em.LowerAliases {
		for _, pos := range FindWordMatches(lowerLine, la) {
			out = append(out, matchSpan{pos, pos + len(la)})
		}
	}

	return out
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

// findEntityAtMention returns the entity whose description span (line +
// byte-column range) contains the given mention position. When multiple
// descriptions overlap — typically an inline aside nested inside the line
// of a header description — the tightest containing span wins, so refs
// inside an aside attribute to the aside-defined entity rather than the
// surrounding owner. Returns "" when the position falls outside every
// description (free text).
//
// For asides only the body span [BodyColumn, EndColumn) counts as the
// entity's territory: the aside header reads naturally as part of the
// surrounding prose, so a name appearing in the header (e.g. "Captain
// Casimir" inside `(Captain Casimir (npc) | Casimir: …)`) attributes to
// free text rather than to the aside's own entity.
func findEntityAtMention(world *World, file string, line, byteCol int) string {
	var bestName string
	bestSize := -1
	for _, ent := range world.Entities {
		for _, desc := range ent.Descriptions {
			if desc.File != file {
				continue
			}
			if line < desc.Line || line > desc.EndLine {
				continue
			}
			startCol := desc.StartColumn
			if desc.IsAside {
				startCol = desc.BodyColumn
			}
			startOK := line > desc.Line || byteCol >= startCol
			endOK := line < desc.EndLine || byteCol < desc.EndColumn
			if !startOK || !endOK {
				continue
			}
			size := descSpanSize(desc)
			if bestSize == -1 || size < bestSize {
				bestSize = size
				bestName = ent.Name
			}
		}
	}
	return bestName
}

// descSpanSize ranks descriptions for tightest-match selection. Single-line
// spans (asides) get a size equal to their column extent; multi-line spans
// get a value that's always larger than any single-line span, so an aside
// nested inside a header definition wins.
func descSpanSize(d Description) int {
	if d.EndLine > d.Line {
		// Use a base offset that exceeds any plausible single-line column
		// extent, then add (EndLine - Line) so a tighter multi-line span
		// still beats a looser one if both somehow contain the position.
		return 1<<20 + (d.EndLine-d.Line)*1024
	}
	return d.EndColumn - d.StartColumn
}
