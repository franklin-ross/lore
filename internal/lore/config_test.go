package lore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitConfigCreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := InitConfig(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "lore.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `files = ["**/*.md"]`) {
		t.Fatalf("unexpected content: %s", data)
	}
}

func TestInitConfigRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := InitConfig(dir); err != nil {
		t.Fatal(err)
	}
	if err := InitConfig(dir); err == nil {
		t.Fatal("expected error when file already exists")
	}
}
