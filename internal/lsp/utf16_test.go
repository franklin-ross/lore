package lsp

import (
	"strings"
	"testing"
)

func TestIsASCII(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", true},
		{"ascii short", "hello", true},
		{"ascii 7 bytes", "abcdefg", true},
		{"ascii 8 bytes (chunk-aligned)", "abcdefgh", true},
		{"ascii 9 bytes (chunk+tail)", "abcdefghi", true},
		{"ascii long", strings.Repeat("x", 1024), true},
		{"high bit in head", "\xc2hello", false},
		{"high bit at chunk end", "abcdefg\xff", false},
		{"high bit just past chunk", "abcdefgh\xff", false},
		{"high bit deep", strings.Repeat("x", 100) + "\xff" + strings.Repeat("y", 100), false},
		{"smart quote (3-byte UTF-8)", "she\xe2\x80\x99s", false},
		{"all high", "\xff\xff\xff\xff\xff\xff\xff\xff", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isASCII(tc.in); got != tc.want {
				t.Fatalf("isASCII(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
