package lsp

import (
	"io/fs"

	"lore/internal/lore"
)

// Index is the LSP's incremental view of the project. It keeps a per-file
// parse cache keyed by project-relative path, and rebuilds the merged World
// lazily on demand. Editor buffers and on-disk files both flow through the
// same SetFile path — the LSP server decides which content wins.
//
// All mutations come from LSP handlers, which glsp dispatches serially on a
// single goroutine, so no locking is needed. If we ever parallelise parsing,
// workers should fan out and merge on a single owner rather than mutating
// this structure concurrently.
type Index struct {
	files   map[string]*lore.FileParse
	matcher lore.Matcher // captured at LoadProject; drives merge order
	config  lore.Config  // captured at LoadProject; supplies [relations.*]
	world   *lore.World  // cached merged world; nil when stale
}

// NewIndex returns an empty Index.
func NewIndex() *Index {
	return &Index{files: make(map[string]*lore.FileParse)}
}

// LoadProject populates the index from every file in the project. Existing
// entries are discarded. Use this on initialize before any editor buffers
// have been registered.
func (idx *Index) LoadProject(project *lore.Project) error {
	idx.files = make(map[string]*lore.FileParse, len(project.FilePaths))
	idx.matcher = project.Matcher
	idx.config = project.Config
	for _, rel := range project.FilePaths {
		data, err := fs.ReadFile(project.FS, rel)
		if err != nil {
			continue
		}
		idx.files[rel] = lore.ParseFile(rel, string(data))
	}
	idx.world = nil
	return nil
}

// SetFile replaces the per-file parse for path with a fresh one built from
// content, and invalidates the cached world.
func (idx *Index) SetFile(path, content string) {
	idx.files[path] = lore.ParseFile(path, content)
	idx.world = nil
}

// RemoveFile drops a file from the index.
func (idx *Index) RemoveFile(path string) {
	if _, ok := idx.files[path]; !ok {
		return
	}
	delete(idx.files, path)
	idx.world = nil
}

// World returns the merged world for the current set of files, rebuilding
// it if any mutation has occurred since the last call.
func (idx *Index) World() *lore.World {
	if idx.world != nil {
		return idx.world
	}
	files := make([]*lore.FileParse, 0, len(idx.files))
	for _, fp := range idx.files {
		files = append(files, fp)
	}
	idx.matcher.SortFileParses(files)
	idx.world = lore.Merge(files)
	// Overlay the project's [relations.*] config onto the built-in vocabulary
	// and validate it, so hover/relations resolution and the relation-config
	// diagnostics match what `lore query` produces.
	defs := lore.EffectiveRelationDefs(idx.config)
	idx.world.Vocab = lore.NewRelationVocab(defs)
	idx.world.RelationIssues = lore.ValidateRelations(defs)
	return idx.world
}

// Content returns the raw content currently tracked for path, or "" false
// if the index has no entry for it.
func (idx *Index) Content(path string) (string, bool) {
	fp, ok := idx.files[path]
	if !ok {
		return "", false
	}
	return fp.Content, true
}

// FileCount returns the number of files currently tracked, for logging.
func (idx *Index) FileCount() int {
	return len(idx.files)
}
