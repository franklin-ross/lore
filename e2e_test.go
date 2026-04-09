package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureSession = `# Session 1

We entered the town and met some locals.

Gundren (character) | Gundren Rockseeker: A dwarf merchant.
  Hired us to deliver supplies to Phandalin.

Phandalin (location): A small frontier town. Gundren sent us here.

Deliver Supplies (quest): Given by Gundren. Take supplies to Phandalin.

Count Strahd von Zarovich (character) | Strahd: Vampire lord.
  Lives at Castle Ravenloft. Doesn't like sunlight.

Castle Ravenloft (location): Strahd's castle. Very spooky.
`

var binaryPath string

func TestMain(m *testing.M) {
	// Build the binary once before all tests.
	tmp, err := os.MkdirTemp("", "lore-e2e-bin-*")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	binaryPath = filepath.Join(tmp, "lore")
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("failed to build lore binary: " + err.Error())
	}

	os.Exit(m.Run())
}

func setupFixtures(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lore.toml"), []byte("files = [\"**/*.md\"]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session-01.md"), []byte(fixtureSession), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runLore(t *testing.T, dir string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = dir
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("failed to run lore: %v", err)
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func TestE2EListShowsAllEntities(t *testing.T) {
	dir := setupFixtures(t)
	stdout, _, _ := runLore(t, dir, "list")

	for _, name := range []string{"Gundren", "Phandalin", "Deliver Supplies", "Count Strahd von Zarovich", "Castle Ravenloft"} {
		if !strings.Contains(stdout, name) {
			t.Errorf("list output missing %q", name)
		}
	}
}

func TestE2EQueryShowsEntityDetails(t *testing.T) {
	dir := setupFixtures(t)
	stdout, _, _ := runLore(t, dir, "query", "Gundren")

	for _, want := range []string{"Gundren", "character", "dwarf merchant", "Gundren Rockseeker"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("query output missing %q", want)
		}
	}
}

func TestE2EQueryByAlias(t *testing.T) {
	dir := setupFixtures(t)
	stdout, _, _ := runLore(t, dir, "query", "Strahd")

	if !strings.Contains(stdout, "Count Strahd von Zarovich") {
		t.Error("expected canonical name in output")
	}
	if !strings.Contains(stdout, "Vampire lord") {
		t.Error("expected description in output")
	}
}

func TestE2ESearchFindsText(t *testing.T) {
	dir := setupFixtures(t)
	stdout, _, _ := runLore(t, dir, "search", "sunlight")

	if !strings.Contains(stdout, "sunlight") {
		t.Error("expected search result containing 'sunlight'")
	}
}

func TestE2EQueryUnknownEntity(t *testing.T) {
	dir := setupFixtures(t)
	_, stderr, code := runLore(t, dir, "query", "Nonexistent")

	if code == 0 {
		t.Error("expected non-zero exit code")
	}
	if !strings.Contains(stderr, "not found") {
		t.Errorf("expected 'not found' in stderr, got %q", stderr)
	}
}

func TestE2ERefsExcludesSelfReferences(t *testing.T) {
	dir := setupFixtures(t)
	stdout, _, _ := runLore(t, dir, "refs", "Gundren")

	// "Gundren" should not appear as a reference from its own definition.
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "A dwarf merchant") {
			t.Errorf("self-reference not filtered: %s", line)
		}
	}
	// But cross-refs from other entities should remain.
	if !strings.Contains(stdout, "Phandalin") || !strings.Contains(stdout, "Deliver Supplies") {
		t.Error("expected cross-references from Phandalin and Deliver Supplies")
	}
}

func TestE2ELSPStartsAndExits(t *testing.T) {
	dir := setupFixtures(t)
	cmd := exec.Command(binaryPath, "lsp")
	cmd.Dir = dir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Close stdin immediately — the LSP should exit cleanly.
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		// LSP may exit with a non-zero code when stdin closes; that's acceptable.
		// We're testing it doesn't hang or crash.
		t.Logf("lsp exited with: %v (acceptable)", err)
	}
}

func TestE2EConfigInitCreatesToml(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := runLore(t, dir, "config", "init")

	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "Created lore.toml") {
		t.Errorf("expected confirmation message, got %q", stdout)
	}
	data, err := os.ReadFile(filepath.Join(dir, "lore.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "files") {
		t.Fatalf("lore.toml missing files key: %s", data)
	}
}

func TestE2EConfigInitRefusesOverwrite(t *testing.T) {
	dir := setupFixtures(t) // already has lore.toml
	_, stderr, code := runLore(t, dir, "config", "init")

	if code == 0 {
		t.Fatal("expected non-zero exit code when lore.toml exists")
	}
	if !strings.Contains(stderr, "already exists") {
		t.Errorf("expected 'already exists' error, got %q", stderr)
	}
}

func TestE2ENoArgsShowsUsage(t *testing.T) {
	dir := setupFixtures(t)
	stdout, _, code := runLore(t, dir)

	if code == 0 {
		t.Error("expected non-zero exit code")
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Error("expected usage text in output")
	}
}
