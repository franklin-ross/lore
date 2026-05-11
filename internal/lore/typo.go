package lore

import "strings"

// typoSuspect inspects an untyped definition that didn't resolve to any
// entity and returns a warning if its name is close enough to an existing
// entity name or alias to look like a typo. Edit distance must be no more
// than max(1, min(4, len(name)/3)) — the floor of 1 keeps short entity
// names (e.g. `X`, `AI`) in scope; the cap of 4 lets multi-word names
// (`Sildar Hallwinter`, `Captain Casimir`) tolerate a few keystrokes
// without reaching distances that would just confuse unrelated names.
// Both header lines and inline asides are checked: an aside whose name
// is one keystroke off a known entity is just as likely to be a typo as
// a header line.
func typoSuspect(world *World, fp *FileParse, def Definition) (StateIssue, bool) {
	name := def.Header.Name
	if name == "" {
		return StateIssue{}, false
	}
	threshold := len(name) / 3
	if threshold > 4 {
		threshold = 4
	}
	if threshold < 1 {
		threshold = 1
	}
	best, _ := closestEntityName(world, name, threshold)
	if best == "" {
		return StateIssue{}, false
	}
	var start, end int
	var ok bool
	if def.IsAside {
		start, end, ok = asideNameRange(fp.Content, def.Line, def.StartColumn)
	} else {
		start, end, ok = headerNameRange(fp.Content, def.Line)
	}
	if !ok {
		return StateIssue{}, false
	}
	return StateIssue{
		Severity: SeverityWarning,
		Message:  `Untyped header "` + name + `" matches no entity. Did you mean "` + best + `"?`,
		Span: StateSpan{
			File:      fp.Path,
			Line:      def.Line,
			StartByte: start,
			EndByte:   end,
		},
	}, true
}

// closestEntityName returns the entity name or alias within maxDist edits
// of query (case-insensitive), plus its distance. Candidates whose length
// differs from query by more than maxDist are skipped without running the
// DP, since |len(a)-len(b)| is a lower bound on edit distance — this is
// what keeps the scan cheap when query is a prose sentence that happens
// to end in a colon. Ties break alphabetically on the candidate. Returns
// ("", 0) when nothing is within maxDist.
func closestEntityName(world *World, query string, maxDist int) (string, int) {
	q := strings.ToLower(query)
	bestName := ""
	bestDist := -1
	consider := func(candidate string) {
		cl := strings.ToLower(candidate)
		diff := len(q) - len(cl)
		if diff < 0 {
			diff = -diff
		}
		if diff > maxDist {
			return
		}
		d := levenshteinBounded(q, cl, maxDist)
		if d > maxDist || d == 0 {
			return
		}
		if bestDist == -1 || d < bestDist || (d == bestDist && candidate < bestName) {
			bestDist = d
			bestName = candidate
		}
	}
	for _, ent := range world.Entities {
		consider(ent.Name)
		for _, a := range ent.Aliases {
			consider(a)
		}
	}
	if bestDist == -1 {
		return "", 0
	}
	return bestName, bestDist
}

// levenshteinBounded is levenshtein with an early-exit when the smallest
// value on the current row exceeds maxDist — at that point no continuation
// can produce a result ≤ maxDist, so the function returns maxDist+1.
// Pass maxDist < 0 to disable the bound.
func levenshteinBounded(a, b string, maxDist int) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		rowMin := curr[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
			if curr[j] < rowMin {
				rowMin = curr[j]
			}
		}
		if rowMin > maxDist {
			return maxDist + 1
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

// headerNameRange locates the byte column range of the name portion of a
// header line — leading whitespace excluded, ending at the header colon —
// on the 1-based line within content. Returns ok=false when the line is
// missing or has no top-level colon.
func headerNameRange(content string, line int) (start, end int, ok bool) {
	raw, ok := lineSlice(content, line)
	if !ok {
		return 0, 0, false
	}
	ws := 0
	for ws < len(raw) && (raw[ws] == ' ' || raw[ws] == '\t') {
		ws++
	}
	colon := IndexHeaderColon(raw)
	if colon < 0 || colon <= ws {
		return 0, 0, false
	}
	return ws, colon, true
}

// asideNameRange locates the byte column range of an aside's name on its
// opener line. startColumn is the column of the leading '(' on that line.
// The range covers everything between the '(' (after whitespace) and the
// aside-level ':' that introduces the body, with surrounding whitespace
// trimmed. Nested parens — e.g. a `(type)` annotation — are skipped so the
// scan only stops at a top-level colon. Returns ok=false when the line
// lacks the expected aside shape.
func asideNameRange(content string, line, startColumn int) (start, end int, ok bool) {
	raw, ok := lineSlice(content, line)
	if !ok || startColumn >= len(raw) || raw[startColumn] != '(' {
		return 0, 0, false
	}
	rest := raw[startColumn:]
	colon := -1
	depth := 0
	for i := 1; i < len(rest); i++ {
		switch rest[i] {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return 0, 0, false
			}
			depth--
		case ':':
			if depth == 0 {
				colon = i
			}
		}
		if colon >= 0 {
			break
		}
	}
	if colon < 0 {
		return 0, 0, false
	}
	nameStart := 1
	for nameStart < colon && (rest[nameStart] == ' ' || rest[nameStart] == '\t') {
		nameStart++
	}
	nameEnd := colon
	for nameEnd > nameStart && (rest[nameEnd-1] == ' ' || rest[nameEnd-1] == '\t') {
		nameEnd--
	}
	if nameEnd <= nameStart {
		return 0, 0, false
	}
	return startColumn + nameStart, startColumn + nameEnd, true
}

// lineSlice returns the substring of content covering the 1-based line,
// excluding any trailing newline. Returns ok=false when the line index is
// past the end of content.
func lineSlice(content string, line int) (string, bool) {
	lo := 0
	for n := 1; n < line; n++ {
		nl := strings.IndexByte(content[lo:], '\n')
		if nl < 0 {
			return "", false
		}
		lo += nl + 1
	}
	hi := len(content)
	if nl := strings.IndexByte(content[lo:], '\n'); nl >= 0 {
		hi = lo + nl
	}
	return content[lo:hi], true
}
