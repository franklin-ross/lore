package lore

import (
	"io/fs"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Matcher selects project files using ordered glob patterns and ignore rules.
// Files are grouped by the first pattern they match, with patterns earlier in
// the list sorting before later ones, and alphabetical order applied within
// each group.
type Matcher struct {
	Patterns []string
	Ignore   []string
}

// Find walks fsys and returns matching paths in pattern-then-alpha order.
func (m Matcher) Find(fsys fs.FS) ([]string, error) {
	if len(m.Patterns) == 0 {
		return nil, nil
	}

	// Combine into a single brace expression so doublestar walks the FS once.
	pattern := m.Patterns[0]
	if len(m.Patterns) > 1 {
		pattern = "{" + strings.Join(m.Patterns, ",") + "}"
	}

	seen := make(map[string]bool)
	var paths []string
	err := doublestar.GlobWalk(fsys, pattern, func(rel string, _ fs.DirEntry) error {
		if seen[rel] || m.isIgnored(rel) {
			return nil
		}
		seen[rel] = true
		paths = append(paths, rel)
		return nil
	}, doublestar.WithFilesOnly())
	if err != nil {
		return nil, err
	}

	m.sortByPatternThenAlpha(paths)
	return paths, nil
}

// Matches reports whether path is selected by the matcher's patterns and not
// excluded by its ignore rules.
func (m Matcher) Matches(path string) bool {
	return m.patternIndex(path) >= 0
}

// SortAndFilter returns the subset of paths that match a pattern and aren't
// ignored, sorted in pattern-then-alpha order. The input slice is not modified.
func (m Matcher) SortAndFilter(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if m.patternIndex(p) >= 0 {
			out = append(out, p)
		}
	}
	m.sortByPatternThenAlpha(out)
	return out
}

// patternIndex returns the index of the first pattern that matches path, or
// -1 if path matches no pattern or is ignored.
func (m Matcher) patternIndex(path string) int {
	if m.isIgnored(path) {
		return -1
	}
	for i, pattern := range m.Patterns {
		if matched, _ := doublestar.Match(pattern, path); matched {
			return i
		}
	}
	return -1
}

func (m Matcher) sortByPatternThenAlpha(paths []string) {
	sort.SliceStable(paths, func(i, j int) bool {
		ai := m.patternIndex(paths[i])
		bi := m.patternIndex(paths[j])
		if ai != bi {
			return ai < bi
		}
		return paths[i] < paths[j]
	})
}

func (m Matcher) isIgnored(path string) bool {
	for _, pattern := range m.Ignore {
		if matched, _ := doublestar.Match(pattern, path); matched {
			return true
		}
		// Bare ignore patterns (no slash) also match the top-level dir of path,
		// so `ignore = ["archive"]` excludes everything under `archive/`.
		if strings.Contains(pattern, "/") {
			continue
		}
		dir, _, ok := strings.Cut(path, "/")
		if !ok {
			continue
		}
		if matched, _ := doublestar.Match(pattern, dir); matched {
			return true
		}
	}
	return false
}
