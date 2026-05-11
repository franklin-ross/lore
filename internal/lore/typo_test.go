package lore

import (
	"strings"
	"testing"
)

func TestMergeWarnsOnUntypedHeaderTypo(t *testing.T) {
	content := "Sildar Hallwinter (character): Fighter.\n" +
		"\n" +
		"Sildar Hallwinder: Patched up.\n"
	world := setupTestWorld(t, content)
	if len(world.FileIssues) != 1 {
		t.Fatalf("FileIssues: %+v", world.FileIssues)
	}
	fi := world.FileIssues[0]
	if fi.Span.Line != 3 {
		t.Fatalf("line: %d", fi.Span.Line)
	}
	if fi.Span.StartByte != 0 || fi.Span.EndByte != len("Sildar Hallwinder") {
		t.Fatalf("span: %+v", fi.Span)
	}
	if !strings.Contains(fi.Message, "Sildar Hallwinter") {
		t.Fatalf("message: %q", fi.Message)
	}
	if fi.Severity != SeverityWarning {
		t.Fatalf("severity: %v", fi.Severity)
	}
}

func TestMergeNoTypoWarningForExactCaseFoldMatch(t *testing.T) {
	content := "Sildar (character): Fighter.\n" +
		"\n" +
		"sildar: Patched up.\n"
	world := setupTestWorld(t, content)
	if len(world.FileIssues) != 0 {
		t.Fatalf("FileIssues: %+v", world.FileIssues)
	}
}

func TestMergeNoTypoWarningForFarName(t *testing.T) {
	content := "Sildar (character): Fighter.\n" +
		"\n" +
		"Note to self: nothing more.\n"
	world := setupTestWorld(t, content)
	if len(world.FileIssues) != 0 {
		t.Fatalf("FileIssues: %+v", world.FileIssues)
	}
}

func TestMergeTypoWarningForShortName(t *testing.T) {
	content := "Notes (location): The journal.\n" +
		"\n" +
		"Note: a brief jot.\n"
	world := setupTestWorld(t, content)
	if len(world.FileIssues) != 1 {
		t.Fatalf("FileIssues: %+v", world.FileIssues)
	}
	if !strings.Contains(world.FileIssues[0].Message, "Notes") {
		t.Fatalf("message: %q", world.FileIssues[0].Message)
	}
}

func TestMergeTypoWarningForTwoCharEntityName(t *testing.T) {
	content := "AI (faction): Hostile.\n" +
		"\n" +
		"Al: a guard.\n"
	world := setupTestWorld(t, content)
	if len(world.FileIssues) != 1 {
		t.Fatalf("FileIssues: %+v", world.FileIssues)
	}
	if !strings.Contains(world.FileIssues[0].Message, "AI") {
		t.Fatalf("message: %q", world.FileIssues[0].Message)
	}
}

func TestMergeTypoWarningMatchesAlias(t *testing.T) {
	content := "Sildar Hallwinter (character) | Sildar: Fighter.\n" +
		"\n" +
		"Slidar: Patched up.\n"
	world := setupTestWorld(t, content)
	if len(world.FileIssues) != 1 {
		t.Fatalf("FileIssues: %+v", world.FileIssues)
	}
	if !strings.Contains(world.FileIssues[0].Message, "Sildar") {
		t.Fatalf("message: %q", world.FileIssues[0].Message)
	}
}

func TestMergeTypoWarningRespectsIndentation(t *testing.T) {
	content := "Sildar Hallwinter (character): Fighter.\n" +
		"\n" +
		"    Sildar Hallwinder: Patched up.\n"
	world := setupTestWorld(t, content)
	if len(world.FileIssues) != 1 {
		t.Fatalf("FileIssues: %+v", world.FileIssues)
	}
	fi := world.FileIssues[0]
	if fi.Span.StartByte != 4 {
		t.Fatalf("start: %d", fi.Span.StartByte)
	}
	if fi.Span.EndByte != 4+len("Sildar Hallwinder") {
		t.Fatalf("end: %d", fi.Span.EndByte)
	}
}

func TestMergeTypoWarningForUntypedAside(t *testing.T) {
	content := "Sildar Hallwinter (character): Fighter.\n" +
		"\n" +
		"We met him (Sildar Hallwinder: looked rough) yesterday.\n"
	world := setupTestWorld(t, content)
	if len(world.FileIssues) != 1 {
		t.Fatalf("FileIssues: %+v", world.FileIssues)
	}
	fi := world.FileIssues[0]
	if fi.Span.Line != 3 {
		t.Fatalf("line: %d", fi.Span.Line)
	}
	openIdx := strings.Index("We met him (Sildar Hallwinder: looked rough) yesterday.", "(")
	wantStart := openIdx + 1
	wantEnd := wantStart + len("Sildar Hallwinder")
	if fi.Span.StartByte != wantStart || fi.Span.EndByte != wantEnd {
		t.Fatalf("span %+v want [%d,%d)", fi.Span, wantStart, wantEnd)
	}
	if !strings.Contains(fi.Message, "Sildar Hallwinter") {
		t.Fatalf("message: %q", fi.Message)
	}
}

func TestMergeTypoWarningForUntypedAsideShortName(t *testing.T) {
	content := "Notes (location): The journal.\n" +
		"\n" +
		"He left a (Note: see ledger) for us.\n"
	world := setupTestWorld(t, content)
	if len(world.FileIssues) != 1 {
		t.Fatalf("FileIssues: %+v", world.FileIssues)
	}
}

func TestMergeNoTypoWarningForLongSentenceColon(t *testing.T) {
	// A prose paragraph that happens to end in a colon parses as an
	// untyped header whose Name is the whole pre-colon span. It must
	// not warn — every entity name is far shorter than the sentence, so
	// the length-diff bail keeps the scan O(N) per entity rather than
	// running a full DP table against a 100-byte query.
	long := strings.Repeat("we waited and waited and ", 5) + "and then she said"
	content := "Sildar Hallwinter (character): Fighter.\n" +
		"\n" +
		long + ":\n"
	world := setupTestWorld(t, content)
	if len(world.FileIssues) != 0 {
		t.Fatalf("FileIssues: %+v", world.FileIssues)
	}
}

func TestLevenshteinBoundedEarlyExit(t *testing.T) {
	// Strings that differ wildly in length must short-circuit rather
	// than fill a |a|×|b| DP table — the bounded variant should return
	// maxDist+1 as soon as the row minimum exceeds the bound.
	long := strings.Repeat("x", 500)
	if got := levenshteinBounded(long, "abc", 2); got != 3 {
		t.Fatalf("got %d, want 3 (maxDist+1)", got)
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"kitten", "sitting", 3},
		{"hallwinter", "hallwinder", 1},
		{"hallwynder", "hallwinter", 2},
	}
	for _, c := range cases {
		// Use a slack max so the bound never trips — we want the exact
		// distance here, not the bounded short-circuit.
		max := len(c.a) + len(c.b)
		if got := levenshteinBounded(c.a, c.b, max); got != c.want {
			t.Errorf("levenshteinBounded(%q,%q,%d) = %d, want %d", c.a, c.b, max, got, c.want)
		}
	}
}
