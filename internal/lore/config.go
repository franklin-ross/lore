package lore

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/bmatcuk/doublestar/v4"
)

const configFilename = "lore.toml"

// Config holds the parsed lore.toml settings.
type Config struct {
	Files  []string `toml:"files"`
	Ignore []string `toml:"ignore"`
}

// Project is a loaded lore project: its filesystem, config, and collected file paths.
type Project struct {
	FS        fs.FS
	Config    Config
	FilePaths []string // relative to FS root, sorted alphabetically
}

// FindAndLoad walks up from the current directory to find lore.toml, parses it,
// and collects all matching files.
func FindAndLoad() (*Project, error) {
	root, err := FindRoot()
	if err != nil {
		return nil, err
	}
	return FindAndLoadFrom(root)
}

// FindAndLoadFrom loads a lore project from the given root directory.
func FindAndLoadFrom(root string) (*Project, error) {
	cfg, err := loadConfig(filepath.Join(root, configFilename))
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	fsys := os.DirFS(root)

	paths, err := CollectFiles(fsys, cfg)
	if err != nil {
		return nil, fmt.Errorf("collecting files: %w", err)
	}

	return &Project{FS: fsys, Config: cfg, FilePaths: paths}, nil
}

// FindRoot walks up from the working directory looking for lore.toml.
func FindRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	for {
		candidate := filepath.Join(dir, configFilename)
		if _, err := os.Stat(candidate); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s found in any parent directory", configFilename)
		}
		dir = parent
	}
}

func loadConfig(path string) (Config, error) {
	cfg := Config{
		Files: []string{"**/*.md"},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing %s: %w", path, err)
	}

	if len(cfg.Files) == 0 {
		cfg.Files = []string{"**/*.md"}
	}

	return cfg, nil
}

// CollectFiles finds all files matching the config globs within fsys,
// respecting ignore patterns. Returns relative paths, sorted.
func CollectFiles(fsys fs.FS, cfg Config) ([]string, error) {
	// Combine multiple patterns into a single brace expression so doublestar
	// walks the filesystem once: {"**/*.md","**/*.lore"} → {**/*.md,**/*.lore}
	pattern := cfg.Files[0]
	if len(cfg.Files) > 1 {
		pattern = "{" + strings.Join(cfg.Files, ",") + "}"
	}

	seen := make(map[string]bool)
	var paths []string

	err := doublestar.GlobWalk(fsys, pattern, func(rel string, d fs.DirEntry) error {
		if seen[rel] || isIgnored(rel, cfg.Ignore) {
			return nil
		}
		seen[rel] = true
		paths = append(paths, rel)
		return nil
	}, doublestar.WithFilesOnly())
	if err != nil {
		return nil, err
	}

	sort.Strings(paths)
	return paths, nil
}

func isIgnored(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := doublestar.Match(pattern, path); matched {
			return true
		}
		// Also try matching against just the first path component for directory patterns.
		if strings.Contains(pattern, "/") {
			continue
		}
		dir := strings.SplitN(path, string(filepath.Separator), 2)[0]
		if matched, _ := doublestar.Match(pattern, dir); matched {
			return true
		}
	}
	return false
}
