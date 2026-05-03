package lsp

import (
	"regexp"
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
//
// URL-bearing constructs (`<autolink>`, `[label](url)`, bare `https://…`
// and `www.…`) are passed through verbatim: entity-name matches inside
// them are dropped and `<>&` are not escaped, so VSCode's markdown
// renderer can still parse them as links.
func (c *colouriser) Wrap(text string) string {
	if c == nil || len(c.palette) == 0 || c.world == nil {
		return escapeForHTML(text)
	}

	protected := protectedURLRanges(text)

	matches := lore.ScanEntities(c.world, text, false)
	if len(protected) > 0 {
		filtered := matches[:0]
		for _, m := range matches {
			if !overlapsRange(m.Start, m.End, protected) {
				filtered = append(filtered, m)
			}
		}
		matches = filtered
	}

	if len(matches) == 0 && len(protected) == 0 {
		return escapeForHTML(text)
	}

	var b strings.Builder
	emitGap := func(start, end int) {
		// Emit `text[start:end)`, splitting around any protected URL
		// ranges so they pass through unescaped while the rest is
		// HTML-escaped.
		cursor := start
		for _, p := range protected {
			if p[1] <= cursor {
				continue
			}
			if p[0] >= end {
				break
			}
			ps, pe := p[0], p[1]
			if ps < cursor {
				ps = cursor
			}
			if pe > end {
				pe = end
			}
			if ps > cursor {
				b.WriteString(escapeForHTML(text[cursor:ps]))
			}
			b.WriteString(text[ps:pe])
			cursor = pe
		}
		if cursor < end {
			b.WriteString(escapeForHTML(text[cursor:end]))
		}
	}

	prev := 0
	for i := 0; i < len(matches); {
		m := matches[i]
		// Group equal-span matches: a bare name shared by multiple
		// entities with no `(type)` suffix in the source. Hover has no
		// place to surface the choice, so we render the text once with
		// the first candidate's colour. The editor diagnostic and wiki
		// view are where the user actually resolves the ambiguity.
		j := i + 1
		for j < len(matches) && matches[j].Start == m.Start && matches[j].End == m.End {
			j++
		}
		emitGap(prev, m.Start)
		idx := int(entityColourIndex(&c.world.Entities[m.EntityIdx]))
		if idx < 0 || idx >= len(c.palette) {
			b.WriteString(escapeForHTML(text[m.Start:m.End]))
		} else {
			b.WriteString(`<span style="color:`)
			b.WriteString(c.palette[idx])
			b.WriteString(`;">`)
			b.WriteString(escapeForHTML(text[m.Start:m.End]))
			b.WriteString(`</span>`)
		}
		prev = m.End
		i = j
	}
	emitGap(prev, len(text))
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

var (
	// `<…>` autolink: no whitespace or angle brackets inside, and at
	// least one `.` or `:` so plain `<foo>` HTML-ish snippets still get
	// escaped.
	autolinkRe = regexp.MustCompile(`<[^<>\s]+>`)
	// URL portion of `[label](url)`. Captures group 1 = URL bytes.
	mdLinkURLRe = regexp.MustCompile(`\]\(([^)\n]+)\)`)
	// Bare URL: `http://…`, `https://…`, or `www.…`.
	bareURLRe = regexp.MustCompile(`(?:https?://|www\.)[^\s<>)\]]+`)
)

// protectedURLRanges returns sorted, merged byte ranges in `text` that
// must pass through Wrap untouched: markdown autolinks, the URL part of
// `[label](url)`, and bare URLs. Without this, entity-name matches inside
// URLs (e.g. "link" inside `www.link.com`) get wrapped in `<span>` tags
// and break markdown link parsing.
func protectedURLRanges(text string) [][2]int {
	if !strings.ContainsAny(text, "<(:.") {
		return nil
	}
	var ranges [][2]int
	for _, m := range autolinkRe.FindAllStringIndex(text, -1) {
		inner := text[m[0]+1 : m[1]-1]
		if strings.ContainsAny(inner, ".:") {
			ranges = append(ranges, [2]int{m[0], m[1]})
		}
	}
	for _, m := range mdLinkURLRe.FindAllStringSubmatchIndex(text, -1) {
		if len(m) >= 4 && m[2] >= 0 {
			ranges = append(ranges, [2]int{m[2], m[3]})
		}
	}
	for _, m := range bareURLRe.FindAllStringIndex(text, -1) {
		ranges = append(ranges, [2]int{m[0], m[1]})
	}
	if len(ranges) == 0 {
		return nil
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i][0] < ranges[j][0] })
	merged := ranges[:0]
	cur := ranges[0]
	for _, r := range ranges[1:] {
		if r[0] <= cur[1] {
			if r[1] > cur[1] {
				cur[1] = r[1]
			}
			continue
		}
		merged = append(merged, cur)
		cur = r
	}
	merged = append(merged, cur)
	return merged
}

// overlapsRange reports whether `[start, end)` overlaps any of the
// sorted, non-overlapping ranges.
func overlapsRange(start, end int, ranges [][2]int) bool {
	lo, hi := 0, len(ranges)
	for lo < hi {
		mid := (lo + hi) / 2
		r := ranges[mid]
		if r[1] <= start {
			lo = mid + 1
		} else if r[0] >= end {
			hi = mid
		} else {
			return true
		}
	}
	return false
}
