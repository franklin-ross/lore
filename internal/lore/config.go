package lore

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const configFilename = "lore.toml"

// Config holds the parsed lore.toml settings.
type Config struct {
	Files  []string `toml:"files"`
	Ignore []string `toml:"ignore"`
}

// Project is a loaded lore project: its filesystem, config, matcher, and
// collected file paths.
type Project struct {
	FS        fs.FS
	Config    Config
	Matcher   Matcher
	FilePaths []string // relative to FS root, ordered by glob pattern then alpha
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
	matcher := Matcher{Patterns: cfg.Files, Ignore: cfg.Ignore}

	paths, err := matcher.Find(fsys)
	if err != nil {
		return nil, fmt.Errorf("collecting files: %w", err)
	}

	return &Project{FS: fsys, Config: cfg, Matcher: matcher, FilePaths: paths}, nil
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

// DefaultConfigContent is the content written by InitConfig.
const DefaultConfigContent = `# Glob files to parse. Pattern order defines parse order, then alphabetic.
files = ["**/*.md"]

# ignore = ["archive"]
`

// InitConfig creates a lore.toml in the given directory with default settings.
// Returns an error if the file already exists.
func InitConfig(dir string) error {
	path := filepath.Join(dir, configFilename)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	return os.WriteFile(path, []byte(DefaultConfigContent), 0644)
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
