package lore

import "fmt"

// ResolveState folds a sequence of state events (in file order) into a
// resolved tag set, field map, and any issues produced by the resolution
// phase. Lexer-time issues are not returned here — callers combine them
// with the returned resolution issues.
//
// For this task only tag events (Value == nil) are handled. Field events
// (Value != nil) are currently ignored and will be implemented in Tasks 7
// and 8.
func ResolveState(events []StateEvent) (map[string]bool, map[string]FieldValue, []StateIssue) {
	tags := make(map[string]bool)
	fields := make(map[string]FieldValue)
	var issues []StateIssue

	for _, ev := range events {
		if ev.Value != nil {
			// Field events are handled in later tasks. Skip for now.
			continue
		}
		switch ev.Op {
		case StateOpAdd:
			tags[ev.Target] = true
		case StateOpRemove:
			if !tags[ev.Target] {
				issues = append(issues, StateIssue{
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("tag %q is not currently active", ev.Target),
					Span:     ev.Span,
				})
			}
			delete(tags, ev.Target)
		}
	}

	return tags, fields, issues
}
