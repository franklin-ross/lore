package lsp

import (
	"io/fs"
	"sync"

	"lore/internal/lore"
)

// Index is the LSP's incremental view of the project. It keeps a per-file
// parse cache keyed by project-relative path, and rebuilds the merged World
// lazily on demand. Editor buffers and on-disk files both flow through the
// same SetFile path — the LSP server decides which content wins.
type Index struct {
	mu    sync.Mutex
	files map[string]*lore.FileParse
	world *lore.World // cached merged world; nil when stale
}

// NewIndex returns an empty Index.
func NewIndex() *Index {
	return &Index{files: make(map[string]*lore.FileParse)}
}

// LoadProject populates the index from every file in the project. Existing
// entries are discarded. Use this on initialize before any editor buffers
// have been registered.
func (idx *Index) LoadProject(project *lore.Project) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.files = make(map[string]*lore.FileParse, len(project.FilePaths))
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
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.files[path] = lore.ParseFile(path, content)
	idx.world = nil
}

// RemoveFile drops a file from the index.
func (idx *Index) RemoveFile(path string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if _, ok := idx.files[path]; !ok {
		return
	}
	delete(idx.files, path)
	idx.world = nil
}

// World returns the merged world for the current set of files, rebuilding
// it if any mutation has occurred since the last call.
func (idx *Index) World() *lore.World {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.world != nil {
		return idx.world
	}
	files := make([]*lore.FileParse, 0, len(idx.files))
	for _, fp := range idx.files {
		files = append(files, fp)
	}
	idx.world = lore.Merge(files)
	return idx.world
}

// Content returns the raw content currently tracked for path, or "" false
// if the index has no entry for it.
func (idx *Index) Content(path string) (string, bool) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	fp, ok := idx.files[path]
	if !ok {
		return "", false
	}
	return fp.Content, true
}

// FileCount returns the number of files currently tracked, for logging.
func (idx *Index) FileCount() int {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return len(idx.files)
}
