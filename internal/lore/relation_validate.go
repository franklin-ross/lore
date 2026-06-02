package lore

import (
	"fmt"
	"sort"
	"strings"
)

// RelationIssue is a problem found in the relation vocabulary — typically a
// reciprocity conflict in a user's lore.toml. It is not tied to a source line
// because relation definitions are project config, not prose.
type RelationIssue struct {
	Message string
}

// ValidateRelations checks a set of relation definitions for reciprocity
// integrity and returns one issue per problem found. The definitions are the
// effective set (built-ins overlaid with config), so a config relation that
// collides with a built-in is caught too.
//
// Two failures are detected, both of which silently corrupt edge identity:
//
//   - Many-to-one reciprocal: two or more relations reciprocate the same
//     relation. A reciprocal is a one-to-one back-pointer, so it can only
//     point back to one — modelling aunt and uncle as separate canonicals that
//     both reciprocate nibling is the canonical example. The fix is to make
//     them aliases of a single canonical.
//   - Non-mutual reciprocal: A reciprocates B but B reciprocates some C ≠ A.
func ValidateRelations(defs []RelationDef) []RelationIssue {
	// Effective declared reciprocal per canonical, last definition winning —
	// matching NewRelationVocab's override order.
	recip := make(map[string]string)
	var order []string
	for _, d := range defs {
		c := canonKey(d.Canonical)
		if c == "" {
			continue
		}
		if _, seen := recip[c]; !seen {
			order = append(order, c)
		}
		recip[c] = canonKey(d.Reciprocal)
	}

	var issues []RelationIssue
	issues = append(issues, manyToOneIssues(recip, order)...)
	issues = append(issues, nonMutualIssues(recip, order)...)
	return issues
}

// manyToOneIssues flags reciprocals claimed by more than one relation.
func manyToOneIssues(recip map[string]string, order []string) []RelationIssue {
	claimers := make(map[string][]string)
	for _, c := range order {
		r := recip[c]
		if r == "" || r == c {
			continue
		}
		claimers[r] = append(claimers[r], c)
	}

	targets := make([]string, 0, len(claimers))
	for r := range claimers {
		targets = append(targets, r)
	}
	sort.Strings(targets)

	var issues []RelationIssue
	for _, r := range targets {
		cs := claimers[r]
		if len(cs) < 2 {
			continue
		}
		sort.Strings(cs)
		issues = append(issues, RelationIssue{Message: fmt.Sprintf(
			"relations %s all reciprocate %q, but a reciprocal can only point back to one relation — make them aliases of a single canonical instead",
			quoteJoin(cs), r)})
	}
	return issues
}

// nonMutualIssues flags reciprocals that don't agree both ways, reporting each
// conflicting pair once.
func nonMutualIssues(recip map[string]string, order []string) []RelationIssue {
	reported := make(map[string]bool)
	var issues []RelationIssue
	for _, c := range order {
		r := recip[c]
		if r == "" || r == c {
			continue
		}
		r2, ok := recip[r]
		if !ok || r2 == "" || r2 == c {
			continue
		}
		pair := c + "\x00" + r
		if c > r {
			pair = r + "\x00" + c
		}
		if reported[pair] {
			continue
		}
		reported[pair] = true
		issues = append(issues, RelationIssue{Message: fmt.Sprintf(
			"relation %q reciprocates %q, but %q reciprocates %q — reciprocals must be mutual",
			c, r, r, r2)})
	}
	return issues
}

// quoteJoin renders a list of names as `"a", "b" and "c"` for messages.
func quoteJoin(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = fmt.Sprintf("%q", n)
	}
	switch len(quoted) {
	case 0:
		return ""
	case 1:
		return quoted[0]
	default:
		return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
	}
}
