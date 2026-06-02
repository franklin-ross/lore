package lsp

import (
	"testing"

	"lore/internal/lore"
)

func TestStringValueTokensExcludeCommas(t *testing.T) {
	lines := []string{"inventory += sword, shield"}
	// ValueSpan covers "sword, shield" at bytes [13,26).
	sp := lore.StateSpan{File: "x.md", Line: 1, StartByte: 13, EndByte: 26}
	var toks []rawToken
	appendStringValueTokens(&toks, lines, sp, "x.md")

	// Two string items plus a punctuation token for the comma: order is
	// sword (string), comma (punctuation), shield (string).
	if len(toks) != 3 {
		t.Fatalf("want 3 tokens (2 items + comma), got %d: %+v", len(toks), toks)
	}
	if toks[0].startChar != 13 || toks[0].length != 5 || toks[0].tokenType != tokenTypeString {
		t.Errorf("sword token = %+v; want string {13,5}", toks[0])
	}
	if toks[1].startChar != 18 || toks[1].length != 1 || toks[1].tokenType != tokenTypePunctuation {
		t.Errorf("comma token = %+v; want punctuation {18,1}", toks[1])
	}
	if toks[2].startChar != 20 || toks[2].length != 6 || toks[2].tokenType != tokenTypeString {
		t.Errorf("shield token = %+v; want string {20,6}", toks[2])
	}
}

func TestStringValueTokensQuotedCommaNotSplit(t *testing.T) {
	lines := []string{`note = "a, b"`}
	// ValueSpan covers `"a, b"` at bytes [7,13).
	sp := lore.StateSpan{File: "x.md", Line: 1, StartByte: 7, EndByte: 13}
	var toks []rawToken
	appendStringValueTokens(&toks, lines, sp, "x.md")
	if len(toks) != 1 {
		t.Fatalf("quoted comma should not split; got %d tokens: %+v", len(toks), toks)
	}
}
