package lsp

import (
	"strings"
	"testing"

	"lore/internal/lore"
)

// findNodes returns every node in the tree whose Kind matches `kind`,
// walking children recursively. Test-only helper.
func findNodes(nodes []MarkdownNode, kind string) []MarkdownNode {
	var out []MarkdownNode
	for _, n := range nodes {
		if n.Kind == kind {
			out = append(out, n)
		}
		out = append(out, findNodes(n.Children, kind)...)
	}
	return out
}

// joinedText concatenates every text-leaf segment in the tree, in
// source order. Useful for assertions about content survival.
func joinedText(nodes []MarkdownNode) string {
	var b strings.Builder
	walkText(nodes, &b)
	return b.String()
}

func walkText(nodes []MarkdownNode, b *strings.Builder) {
	for _, n := range nodes {
		switch n.Kind {
		case "text":
			for _, s := range n.Segments {
				b.WriteString(s.Text)
			}
		case "code-block", "code-inline":
			b.WriteString(n.Text)
		}
		if len(n.Children) > 0 {
			walkText(n.Children, b)
		}
	}
}

func TestBuildDescriptionContentParagraph(t *testing.T) {
	got := buildDescriptionContent(lore.NewWorld(), "Ranger of the North.")
	if len(got) != 1 || got[0].Kind != "paragraph" {
		t.Fatalf("got %+v", got)
	}
	if joinedText(got) != "Ranger of the North." {
		t.Fatalf("text = %q", joinedText(got))
	}
}

func TestBuildDescriptionContentBlockquoteSpansParagraphs(t *testing.T) {
	src := "> Lots of prose.\n>\n> Spread over many lines."
	got := buildDescriptionContent(lore.NewWorld(), src)
	bqs := findNodes(got, "blockquote")
	if len(bqs) != 1 {
		t.Fatalf("expected one blockquote, got %d (tree=%+v)", len(bqs), got)
	}
	// The blockquote body keeps both halves of the quote separated by
	// the blank `>` line. CommonMark parses these as two paragraphs.
	paras := findNodes(bqs, "paragraph")
	if len(paras) != 2 {
		t.Fatalf("expected two paragraphs inside blockquote, got %d", len(paras))
	}
	text := joinedText(got)
	if !strings.Contains(text, "Lots of prose.") || !strings.Contains(text, "Spread over many lines.") {
		t.Fatalf("blockquote content lost: %q", text)
	}
	// The leading `>` markers must not survive into rendered text.
	if strings.Contains(text, ">") {
		t.Fatalf("blockquote `>` marker leaked: %q", text)
	}
}

func TestBuildDescriptionContentHorizontalRule(t *testing.T) {
	got := buildDescriptionContent(lore.NewWorld(), "before\n\n---\n\nafter")
	hrs := findNodes(got, "hr")
	if len(hrs) != 1 {
		t.Fatalf("expected one hr, got %d", len(hrs))
	}
}

func TestBuildDescriptionContentList(t *testing.T) {
	src := "- one\n- two\n- three"
	got := buildDescriptionContent(lore.NewWorld(), src)
	lists := findNodes(got, "list")
	if len(lists) != 1 || lists[0].Ordered {
		t.Fatalf("expected one unordered list, got %+v", lists)
	}
	items := findNodes(lists, "list-item")
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
}

func TestBuildDescriptionContentOrderedList(t *testing.T) {
	got := buildDescriptionContent(lore.NewWorld(), "1. first\n2. second")
	lists := findNodes(got, "list")
	if len(lists) != 1 || !lists[0].Ordered {
		t.Fatalf("expected one ordered list, got %+v", lists)
	}
}

func TestBuildDescriptionContentHeading(t *testing.T) {
	got := buildDescriptionContent(lore.NewWorld(), "## Subsection\n\nbody")
	headings := findNodes(got, "heading")
	if len(headings) != 1 || headings[0].Level != 2 {
		t.Fatalf("expected one h2, got %+v", headings)
	}
}

func TestBuildDescriptionContentFencedCode(t *testing.T) {
	src := "```go\nfunc f() {}\n```"
	got := buildDescriptionContent(lore.NewWorld(), src)
	codes := findNodes(got, "code-block")
	if len(codes) != 1 {
		t.Fatalf("expected one code block, got %d", len(codes))
	}
	if codes[0].Lang != "go" {
		t.Fatalf("lang = %q", codes[0].Lang)
	}
	if !strings.Contains(codes[0].Text, "func f() {}") {
		t.Fatalf("code text = %q", codes[0].Text)
	}
}

func TestBuildDescriptionContentEmphasisStrongCodeLink(t *testing.T) {
	src := "see *italic*, **bold**, `code`, and [text](https://example.com)"
	got := buildDescriptionContent(lore.NewWorld(), src)
	if len(findNodes(got, "emphasis")) == 0 {
		t.Errorf("missing emphasis")
	}
	if len(findNodes(got, "strong")) == 0 {
		t.Errorf("missing strong")
	}
	if len(findNodes(got, "code-inline")) == 0 {
		t.Errorf("missing code-inline")
	}
	links := findNodes(got, "link")
	if len(links) != 1 || links[0].Href != "https://example.com" {
		t.Fatalf("link = %+v", links)
	}
}

func TestBuildDescriptionContentEntityColouringInsideMarkdown(t *testing.T) {
	// An entity mention inside a blockquote body must end up in a text
	// leaf whose segment carries the entity's colourIndex, not as plain
	// text. Same expectation as the legacy flat-segments path.
	world := lore.NewWorld()
	world.Entities = []lore.Entity{{Name: "Aragorn", Type: "character"}}
	world.Match = lore.BuildMatchIndex(world)
	got := buildDescriptionContent(world, "> Met Aragorn at dusk.")
	var sawColoured bool
	var visit func(nodes []MarkdownNode)
	visit = func(nodes []MarkdownNode) {
		for _, n := range nodes {
			for _, s := range n.Segments {
				if strings.Contains(s.Text, "Aragorn") && s.ColourIndex >= 0 {
					sawColoured = true
				}
			}
			visit(n.Children)
		}
	}
	visit(got)
	if !sawColoured {
		t.Fatalf("expected coloured Aragorn segment, got tree: %+v", got)
	}
}

func TestBuildDescriptionContentJournalExample(t *testing.T) {
	// End-to-end shape check for the block-aside body the user
	// originally asked for: a journal item with a multi-paragraph
	// blockquote.
	src := "> Lots of prose.\n>\n> Spread over many lines."
	got := buildDescriptionContent(lore.NewWorld(), src)
	if len(findNodes(got, "blockquote")) != 1 {
		t.Fatalf("blockquote missing: %+v", got)
	}
	if joinedText(got) == "" {
		t.Fatalf("no text survived")
	}
}
