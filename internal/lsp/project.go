package lsp

import (
	"io/fs"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"lore/internal/lore"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// projectState is one lore.toml-rooted unit: its config-driven Project, its
// per-file index, and the absolute filesystem root it lives at. The server
// holds one of these per discovered lore.toml.
type projectState struct {
	root    string // absolute path to the directory containing lore.toml
	project *lore.Project
	index   *Index
}

// fileToURI returns a file:// URI for a project-root-relative path.
// Path bytes are percent-encoded so filenames containing URI reserved
// characters (`?`, `#`, ` `, etc.) survive the round trip through
// VSCode's URI parser.
func (p *projectState) fileToURI(rel string) string {
	abs := filepath.Join(p.root, filepath.FromSlash(rel))
	// On Windows the leading drive letter needs the URL path to start
	// with `/`; on POSIX the path is already absolute. filepath.ToSlash
	// converts separators so url.URL.String() emits forward slashes.
	p2 := filepath.ToSlash(abs)
	if !strings.HasPrefix(p2, "/") {
		p2 = "/" + p2
	}
	return (&url.URL{Scheme: "file", Path: p2}).String()
}

// world returns the merged world for this project's current files.
func (p *projectState) world() *lore.World {
	return p.index.World()
}

// locAtLine builds an LSP Location pointing at the start of `line` (1-based)
// in the project-rel file. Used by wiki-style responses that just need the
// line, not a precise column range.
func (p *projectState) locAtLine(rel string, line int) protocol.Location {
	return protocol.Location{URI: p.fileToURI(rel), Range: lineRange(line)}
}

// lineText returns the text of the 1-based line in the project-rel file,
// or "" when the file isn't tracked or the line is out of range.
func (p *projectState) lineText(rel string, line int) string {
	content, _ := p.index.Content(rel)
	lines := strings.Split(content, "\n")
	idx := line - 1
	if idx < 0 || idx >= len(lines) {
		return ""
	}
	return lines[idx]
}

// locAtMatch builds an LSP Location pointing at a substring on `line` (both
// 1-based) in the project-rel file. byteStart and byteEnd are byte offsets
// within the original line; they're converted to UTF-16 code units so the
// resulting Range matches what VSCode expects. Falls back to a zero-width
// range at the line start when the file content can't be retrieved.
func (p *projectState) locAtMatch(rel string, line, byteStart, byteEnd int) protocol.Location {
	uri := p.fileToURI(rel)
	content, _ := p.index.Content(rel)
	lines := strings.Split(content, "\n")
	idx := line - 1
	if idx < 0 || idx >= len(lines) {
		return protocol.Location{URI: uri, Range: lineRange(line)}
	}
	lineText := lines[idx]
	if byteStart < 0 || byteEnd < byteStart || byteEnd > len(lineText) {
		return protocol.Location{URI: uri, Range: lineRange(line)}
	}
	l := uint32(idx)
	return protocol.Location{
		URI: uri,
		Range: protocol.Range{
			Start: protocol.Position{Line: l, Character: utf16UnitsForBytes(lineText, byteStart)},
			End:   protocol.Position{Line: l, Character: utf16UnitsForBytes(lineText, byteEnd)},
		},
	}
}

// discoverProjects walks workspaceRoot collecting one projectState per
// lore.toml encountered. Files that fall under a *descendant* project's root
// are excluded from the ancestor's index so each file belongs to its nearest
// ancestor lore.toml only.
//
// If no lore.toml exists anywhere in the tree, the workspace root is treated
// as a single virtual project so users without a config still get default
// indexing.
func discoverProjects(workspaceRoot string) (map[string]*projectState, error) {
	if workspaceRoot == "" {
		return nil, nil
	}

	roots, err := findConfigDirs(workspaceRoot)
	if err != nil {
		return nil, err
	}
	if len(roots) == 0 {
		roots = []string{workspaceRoot}
	}

	// Sort shallow-first so we can compute descendant relationships in one pass.
	sort.Slice(roots, func(i, j int) bool { return len(roots[i]) < len(roots[j]) })

	out := make(map[string]*projectState, len(roots))
	for _, root := range roots {
		ps, err := loadProjectAt(root, roots)
		if err != nil {
			continue
		}
		out[root] = ps
	}
	return out, nil
}

// loadProjectAt builds a projectState rooted at root, with FilePaths trimmed
// of anything that lives under a deeper project root in allRoots.
func loadProjectAt(root string, allRoots []string) (*projectState, error) {
	proj, err := lore.FindAndLoadFrom(root)
	if err != nil {
		return nil, err
	}

	filtered := proj.FilePaths[:0]
	for _, rel := range proj.FilePaths {
		if pathUnderDescendantProject(root, rel, allRoots) {
			continue
		}
		filtered = append(filtered, rel)
	}
	proj.FilePaths = filtered

	ps := &projectState{root: root, project: proj, index: NewIndex()}
	if err := ps.index.LoadProject(proj); err != nil {
		return nil, err
	}
	return ps, nil
}

// findConfigDirs walks root and returns the directory of every lore.toml it
// finds. Skips noisy system directories that should never house lore content.
func findConfigDirs(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p == root {
				return nil
			}
			switch d.Name() {
			case ".git", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == "lore.toml" {
			out = append(out, filepath.Dir(p))
		}
		return nil
	})
	return out, err
}

// pathUnderDescendantProject reports whether projRel (relative to root) sits
// inside another project root that is itself a descendant of root.
func pathUnderDescendantProject(root, projRel string, allRoots []string) bool {
	abs := filepath.Join(root, filepath.FromSlash(projRel))
	rootPrefix := root + string(filepath.Separator)
	for _, other := range allRoots {
		if other == root {
			continue
		}
		if !strings.HasPrefix(other, rootPrefix) {
			continue
		}
		if abs == other {
			return true
		}
		if strings.HasPrefix(abs, other+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// findOwner picks the project whose root is the longest matching ancestor of
// absPath. Returns nil if absPath is outside every project. The returned
// projRel uses forward slashes to match fs.FS conventions.
func (s *Server) findOwner(absPath string) (*projectState, string) {
	var best *projectState
	var bestRel string
	for _, ps := range s.projects {
		rel, ok := relUnder(ps.root, absPath)
		if !ok {
			continue
		}
		if best == nil || len(ps.root) > len(best.root) {
			best = ps
			bestRel = rel
		}
	}
	return best, bestRel
}

// projectForURI resolves a file:// URI to (owning project, project-rel path).
func (s *Server) projectForURI(uri string) (*projectState, string) {
	return s.findOwner(uriToPath(&uri))
}

// relUnder returns abs as a path relative to root, or ("", false) if abs is
// outside root. Output uses forward slashes.
func relUnder(root, abs string) (string, bool) {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	if rel == "." {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// configRoots returns absolute project roots in deterministic (sorted) order.
// Used by the watcher registration.
func (s *Server) configRoots() []string {
	out := make([]string, 0, len(s.projects))
	for r := range s.projects {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}
