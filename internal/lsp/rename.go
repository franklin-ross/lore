package lsp

import (
	"errors"
	"fmt"
	"strings"

	"lore/internal/lore"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// prepareRename validates that the cursor is on an entity name or alias and
// returns the byte range of the matched slot. VSCode uses the placeholder
// to seed the input box and the range to highlight which spelling will be
// rewritten — so positioning on "Sildar" inside a longer header like
// `Sildar Hallwinter (character) | Sildar:` selects only the alias, leaving
// the canonical name untouched.
func (s *Server) prepareRename(_ *glsp.Context, params *protocol.PrepareRenameParams) (any, error) {
	ps, _ := s.projectForURI(params.TextDocument.URI)
	if ps == nil {
		return nil, nil
	}
	match := s.findEntityAtPosition(ps, params.TextDocument.URI, params.Position)
	if match == nil || match.MatchedName == "" {
		return nil, nil
	}
	line := s.getLine(params.TextDocument.URI, params.Position.Line)
	startByte := match.Start
	endByte := match.Start + len(match.MatchedName)
	if endByte > len(line) {
		endByte = match.End
	}
	rng := protocol.Range{
		Start: protocol.Position{Line: params.Position.Line, Character: utf16UnitsForBytes(line, startByte)},
		End:   protocol.Position{Line: params.Position.Line, Character: utf16UnitsForBytes(line, endByte)},
	}
	return protocol.RangeWithPlaceholder{Range: rng, Placeholder: match.MatchedName}, nil
}

// rename returns a WorkspaceEdit that rewrites every occurrence of the
// specific name or alias under the cursor. Renaming an alias updates the
// alias slot in the entity's header(s) and every prose mention spelled the
// same way; the canonical name and other aliases are untouched. Renaming
// the canonical name follows the same rule for its own spelling.
func (s *Server) rename(_ *glsp.Context, params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	newName := strings.TrimSpace(params.NewName)
	if newName == "" {
		return nil, errors.New("new name must not be empty")
	}
	if strings.ContainsAny(newName, ":|()") {
		return nil, fmt.Errorf("new name must not contain `:`, `|`, or parentheses")
	}

	ps, _ := s.projectForURI(params.TextDocument.URI)
	if ps == nil {
		return nil, nil
	}
	match := s.findEntityAtPosition(ps, params.TextDocument.URI, params.Position)
	if match == nil || match.MatchedName == "" {
		return nil, nil
	}

	changes := s.collectRenameEdits(ps, match.Entity, match.MatchedName, newName)
	if len(changes) == 0 {
		return nil, nil
	}
	return &protocol.WorkspaceEdit{Changes: changes}, nil
}

// collectRenameEdits walks the entity's header descriptions and reference
// index, emitting a TextEdit for every span that spells the same alias or
// canonical name as the cursor. References for entities sharing a canonical
// name are filtered by TargetType so a rename on `Barovia (town)` doesn't
// touch mentions of `Barovia (country)`.
func (s *Server) collectRenameEdits(ps *projectState, ent *lore.Entity, matchedName, newName string) map[protocol.DocumentUri][]protocol.TextEdit {
	changes := make(map[protocol.DocumentUri][]protocol.TextEdit)
	seen := make(map[string]bool) // file:line:start dedup across header + refs

	addEdit := func(file string, line, byteStart, byteEnd int) {
		key := fmt.Sprintf("%s:%d:%d", file, line, byteStart)
		if seen[key] {
			return
		}
		seen[key] = true
		text := ps.lineText(file, line)
		if byteStart < 0 || byteEnd > len(text) || byteStart >= byteEnd {
			return
		}
		uri := protocol.DocumentUri(ps.fileToURI(file))
		l := uint32(line - 1)
		changes[uri] = append(changes[uri], protocol.TextEdit{
			Range: protocol.Range{
				Start: protocol.Position{Line: l, Character: utf16UnitsForBytes(text, byteStart)},
				End:   protocol.Position{Line: l, Character: utf16UnitsForBytes(text, byteEnd)},
			},
			NewText: newName,
		})
	}

	for _, desc := range ent.Descriptions {
		line := ps.lineText(desc.File, desc.Line)
		if line == "" {
			continue
		}
		headerStart, headerEnd, ok := headerColumnRange(line, desc)
		if !ok {
			continue
		}
		relStart, relEnd, found := lore.LocateNameInHeader(line[headerStart:headerEnd], matchedName)
		if !found {
			continue
		}
		addEdit(desc.File, desc.Line, headerStart+relStart, headerStart+relEnd)
	}

	for _, ref := range ps.world().References[ent.Name] {
		if ref.TargetType != "" && ent.Type != "" && !strings.EqualFold(ref.TargetType, ent.Type) {
			continue
		}
		line := ps.lineText(ref.File, ref.Line)
		if ref.MatchStart < 0 || ref.MatchEnd > len(line) || ref.MatchEnd <= ref.MatchStart {
			continue
		}
		if !strings.EqualFold(line[ref.MatchStart:ref.MatchEnd], matchedName) {
			continue
		}
		addEdit(ref.File, ref.Line, ref.MatchStart, ref.MatchEnd)
	}

	return changes
}

// headerColumnRange returns the byte span on desc.Line that holds the
// header part — `Name [(type)] [| Alias …]` — between the start of the
// definition and the colon separating it from the body. For inline asides
// the range sits inside the surrounding `(...)`; for header lines it
// starts at column 0 and ends at the line's first paren-depth-zero colon.
func headerColumnRange(line string, desc lore.Description) (int, int, bool) {
	if desc.IsAside {
		innerStart := desc.StartColumn + 1
		innerEnd := desc.EndColumn - 1
		if innerStart < 0 || innerEnd > len(line) || innerStart >= innerEnd {
			return 0, 0, false
		}
		colon := lore.IndexHeaderColon(line[innerStart:innerEnd])
		if colon < 0 {
			return 0, 0, false
		}
		return innerStart, innerStart + colon, true
	}
	colon := lore.IndexHeaderColon(line)
	if colon < 0 {
		return 0, 0, false
	}
	return 0, colon, true
}
