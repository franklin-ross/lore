package lsp

import (
	"sort"
	"strings"

	"lore/internal/lore"
)

// colouriser wraps the world plus the client-supplied palette. When palette
// is empty (no init option), Wrap returns the input unchanged, so hovers
// degrade to plain markdown rather than emitting unstyled HTML.
type colouriser struct {
	world   *lore.World
	palette []string
}

// Wrap returns markdown text with every entity name occurrence in `text`
// wrapped in a `<span style="color:#hex">` tag, where the hex comes from
// `palette[entityColourIndex(ent)]`. `<` and `&` outside spans are escaped
// so user prose containing those characters renders as written. Other
// markdown markers (`*`, `_`, etc.) are left intact so bold/italic/etc.
// continue to work in the hover.
func (c *colouriser) Wrap(text string) string {
	if c == nil || len(c.palette) == 0 || c.world == nil || c.world.Match == nil {
		return escapeForHTML(text)
	}

	type span struct {
		start  int
		end    int
		colour int
	}

	lower := strings.ToLower(text)
	var spans []span
	for i := range c.world.Match.Entities {
		em := &c.world.Match.Entities[i]
		colour := int(entityColourIndex(&c.world.Entities[i]))
		if em.LowerName != "" {
			for _, p := range lore.FindWordMatches(lower, em.LowerName) {
				spans = append(spans, span{p, p + len(em.LowerName), colour})
			}
		}
		for _, la := range em.LowerAliases {
			for _, p := range lore.FindWordMatches(lower, la) {
				spans = append(spans, span{p, p + len(la), colour})
			}
		}
	}
	if len(spans) == 0 {
		return escapeForHTML(text)
	}

	// Greedy resolve: longer matches win at the same start, and any span
	// overlapping a previously kept span is dropped.
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return (spans[i].end - spans[i].start) > (spans[j].end - spans[j].start)
	})
	kept := spans[:0]
	lastEnd := -1
	for _, s := range spans {
		if s.start < lastEnd {
			continue
		}
		kept = append(kept, s)
		lastEnd = s.end
	}

	var b strings.Builder
	prev := 0
	for _, s := range kept {
		b.WriteString(escapeForHTML(text[prev:s.start]))
		idx := s.colour
		if idx < 0 || idx >= len(c.palette) {
			b.WriteString(escapeForHTML(text[s.start:s.end]))
		} else {
			b.WriteString(`<span style="color:`)
			b.WriteString(c.palette[idx])
			b.WriteString(`;">`)
			b.WriteString(escapeForHTML(text[s.start:s.end]))
			b.WriteString(`</span>`)
		}
		prev = s.end
	}
	b.WriteString(escapeForHTML(text[prev:]))
	return b.String()
}

// escapeForHTML escapes only the characters that would break HTML rendering
// when supportHtml is on. Markdown markers (`*`, `_`, `` ` ``) pass through
// so the markdown processor still picks them up before the HTML sanitiser
// runs.
func escapeForHTML(s string) string {
	if !strings.ContainsAny(s, "<>&") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
