package lsp

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"lore/internal/lore"
)

// MarkdownNode is one rendered piece of a description body, structured
// so the wiki webview can build DOM directly from it without parsing
// markdown itself. Kind names mirror the corresponding HTML tags the
// client emits, with two flavours of leaf:
//
//   - "text": carries entity-aware Segments built from the source slice
//   - "code-block" / "code-inline": carry raw Text (no entity scanning,
//     because identifiers inside code shouldn't trigger references)
//
// Container kinds ("paragraph", "blockquote", "heading", "list",
// "list-item", "emphasis", "strong", "link") nest via Children. Level
// is used by headings; Ordered by lists; Lang by code blocks; Href by
// links.
type MarkdownNode struct {
	Kind     string           `json:"kind"`
	Children []MarkdownNode   `json:"children,omitempty"`
	Segments []ContextSegment `json:"segments,omitempty"`
	Text     string           `json:"text,omitempty"`
	Level    int              `json:"level,omitempty"`
	Ordered  bool             `json:"ordered,omitempty"`
	Lang     string           `json:"lang,omitempty"`
	Href     string           `json:"href,omitempty"`
}

// buildDescriptionContent renders a description body into a structured
// tree of MarkdownNodes. The tree is the wire format the wiki webview
// consumes; the same parser could later feed any other markdown output
// (CLI export, HTML preview) without duplicating block awareness.
//
// Entity colouring is woven into "text" leaves only — inline content
// inside emphasis/strong/link is itself a tree of inline nodes whose
// leaves carry segments. Code spans and code blocks intentionally skip
// entity scanning so an identifier inside `like_this` or a fenced
// block doesn't get a colour span.
func buildDescriptionContent(world *lore.World, source string) []MarkdownNode {
	if source == "" {
		return nil
	}
	src := []byte(source)
	doc := goldmark.New().Parser().Parse(text.NewReader(src))
	var out []MarkdownNode
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		out = append(out, convertBlock(c, src, world)...)
	}
	return out
}

func convertBlock(n ast.Node, src []byte, world *lore.World) []MarkdownNode {
	switch v := n.(type) {
	case *ast.Paragraph:
		return []MarkdownNode{{Kind: "paragraph", Children: convertInlines(v, src, world)}}
	case *ast.Blockquote:
		return []MarkdownNode{{Kind: "blockquote", Children: convertContainerBlocks(v, src, world)}}
	case *ast.ThematicBreak:
		return []MarkdownNode{{Kind: "hr"}}
	case *ast.Heading:
		return []MarkdownNode{{Kind: "heading", Level: v.Level, Children: convertInlines(v, src, world)}}
	case *ast.List:
		return []MarkdownNode{{Kind: "list", Ordered: v.IsOrdered(), Children: convertContainerBlocks(v, src, world)}}
	case *ast.ListItem:
		return []MarkdownNode{{Kind: "list-item", Children: convertContainerBlocks(v, src, world)}}
	case *ast.FencedCodeBlock:
		return []MarkdownNode{{
			Kind: "code-block",
			Lang: string(v.Language(src)),
			Text: codeBlockText(v, src),
		}}
	case *ast.CodeBlock:
		return []MarkdownNode{{Kind: "code-block", Text: codeBlockText(v, src)}}
	}
	// Unknown block (raw HTML, link reference definition, …) — fall
	// through as a paragraph carrying the raw source slice so content
	// isn't silently lost.
	return []MarkdownNode{{Kind: "paragraph", Children: convertInlines(n, src, world)}}
}

func convertContainerBlocks(n ast.Node, src []byte, world *lore.World) []MarkdownNode {
	var out []MarkdownNode
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		out = append(out, convertBlock(c, src, world)...)
	}
	return out
}

func convertInlines(n ast.Node, src []byte, world *lore.World) []MarkdownNode {
	var out []MarkdownNode
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		out = append(out, convertInline(c, src, world)...)
	}
	return out
}

func convertInline(n ast.Node, src []byte, world *lore.World) []MarkdownNode {
	switch v := n.(type) {
	case *ast.Text:
		txt := string(v.Value(src))
		// Soft and hard line breaks both render as a newline in the
		// emitted text; the client decides whether to render them as
		// space or <br>. Joining adjacent Text siblings into one
		// segment-bearing leaf preserves byte boundaries for entity
		// scanning over the merged run.
		if v.SoftLineBreak() || v.HardLineBreak() {
			txt += "\n"
		}
		if txt == "" {
			return nil
		}
		return []MarkdownNode{{Kind: "text", Segments: buildContextSegments(world, txt)}}
	case *ast.String:
		txt := string(v.Value)
		if txt == "" {
			return nil
		}
		return []MarkdownNode{{Kind: "text", Segments: buildContextSegments(world, txt)}}
	case *ast.Emphasis:
		kind := "emphasis"
		if v.Level >= 2 {
			kind = "strong"
		}
		return []MarkdownNode{{Kind: kind, Children: convertInlines(v, src, world)}}
	case *ast.CodeSpan:
		return []MarkdownNode{{Kind: "code-inline", Text: inlineText(v, src)}}
	case *ast.Link:
		return []MarkdownNode{{Kind: "link", Href: string(v.Destination), Children: convertInlines(v, src, world)}}
	case *ast.AutoLink:
		url := string(v.URL(src))
		return []MarkdownNode{{
			Kind: "link",
			Href: url,
			Children: []MarkdownNode{{
				Kind:     "text",
				Segments: []ContextSegment{{Text: url, ColourIndex: -1}},
			}},
		}}
	}
	return nil
}

// codeBlockText concatenates the source slices of a code block's
// content lines back into a single string. Goldmark stores fenced and
// indented code blocks as a list of source segments rather than a
// pre-joined string, so we walk Lines() to rebuild it.
func codeBlockText(n ast.Node, src []byte) string {
	lines := n.Lines()
	if lines == nil || lines.Len() == 0 {
		return ""
	}
	var buf []byte
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf = append(buf, seg.Value(src)...)
	}
	return string(buf)
}

// inlineText pulls the raw text out of an inline container (e.g. a
// `<code>` span) by concatenating its Text leaves. Used for content
// that should bypass entity scanning.
func inlineText(n ast.Node, src []byte) string {
	var buf []byte
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			buf = append(buf, t.Value(src)...)
		}
	}
	return string(buf)
}
