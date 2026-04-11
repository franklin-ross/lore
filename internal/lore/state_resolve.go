package lore

import "fmt"

// ResolveState folds a sequence of state events (in file order) into a
// resolved tag set, field map, and any issues produced by the resolution
// phase. Lexer-time issues are not returned here — callers combine them
// with the returned resolution issues.
func ResolveState(events []StateEvent) (map[string]bool, map[string]FieldValue, []StateIssue) {
	tags := make(map[string]bool)
	fields := make(map[string]FieldValue)
	var issues []StateIssue

	for _, ev := range events {
		if ev.Value != nil {
			issues = append(issues, applyFieldEvent(fields, ev)...)
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

// applyFieldEvent applies a single field-valued event to the fields map
// and returns any issues produced.
func applyFieldEvent(fields map[string]FieldValue, ev StateEvent) []StateIssue {
	var issues []StateIssue
	existing, hasExisting := fields[ev.Target]

	switch ev.Op {
	case StateOpSet:
		if hasExisting && existing.Kind != ev.Value.Kind {
			issues = append(issues, StateIssue{
				Severity: SeverityError,
				Message:  fmt.Sprintf("cannot change kind of field %q from %s to %s", ev.Target, kindName(existing.Kind), kindName(ev.Value.Kind)),
				Span:     ev.Span,
			})
			return issues
		}
		// Copy-by-value; for text, copy the slice so we don't alias.
		fields[ev.Target] = copyFieldValue(*ev.Value)

	case StateOpIncrement:
		if !hasExisting {
			// Initialise from the increment value.
			fields[ev.Target] = copyFieldValue(*ev.Value)
			return issues
		}
		if existing.Kind != ev.Value.Kind {
			issues = append(issues, StateIssue{
				Severity: SeverityError,
				Message:  fmt.Sprintf("cannot %s %s value to %s field %q", opName(ev.Op), kindName(ev.Value.Kind), kindName(existing.Kind), ev.Target),
				Span:     ev.Span,
			})
			return issues
		}
		if ev.Value.Kind == FieldNumeric {
			existing.Number += ev.Value.Number
			fields[ev.Target] = existing
		}
		if ev.Value.Kind == FieldText {
			combined := make([]string, 0, len(existing.Text)+len(ev.Value.Text))
			combined = append(combined, existing.Text...)
			combined = append(combined, ev.Value.Text...)
			fields[ev.Target] = FieldValue{Kind: FieldText, Text: combined}
		}

	case StateOpRemove:
		if !hasExisting {
			issues = append(issues, StateIssue{
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("field %q is not initialised", ev.Target),
				Span:     ev.Span,
			})
			return issues
		}
		if existing.Kind != ev.Value.Kind {
			issues = append(issues, StateIssue{
				Severity: SeverityError,
				Message:  fmt.Sprintf("cannot %s %s value from %s field %q", opName(ev.Op), kindName(ev.Value.Kind), kindName(existing.Kind), ev.Target),
				Span:     ev.Span,
			})
			return issues
		}
		if ev.Value.Kind == FieldNumeric {
			existing.Number -= ev.Value.Number
			fields[ev.Target] = existing
		}
		if ev.Value.Kind == FieldText {
			remaining := append([]string(nil), existing.Text...)
			for _, itemToRemove := range ev.Value.Text {
				idx := -1
				for i, it := range remaining {
					if it == itemToRemove {
						idx = i
						break
					}
				}
				if idx < 0 {
					issues = append(issues, StateIssue{
						Severity: SeverityWarning,
						Message:  fmt.Sprintf("list %q has no item %q to remove", ev.Target, itemToRemove),
						Span:     ev.Span,
					})
					continue
				}
				remaining = append(remaining[:idx], remaining[idx+1:]...)
			}
			if len(remaining) == 0 {
				delete(fields, ev.Target)
			} else {
				fields[ev.Target] = FieldValue{Kind: FieldText, Text: remaining}
			}
		}
	}
	return issues
}

// copyFieldValue returns a copy of v that doesn't alias its Text slice.
func copyFieldValue(v FieldValue) FieldValue {
	if v.Kind == FieldText && v.Text != nil {
		text := make([]string, len(v.Text))
		copy(text, v.Text)
		v.Text = text
	}
	return v
}

func opName(op StateOp) string {
	switch op {
	case StateOpIncrement:
		return "append"
	case StateOpRemove:
		return "remove"
	case StateOpSet:
		return "assign"
	}
	return "apply"
}

func kindName(k FieldKind) string {
	switch k {
	case FieldNumeric:
		return "numeric"
	case FieldText:
		return "text"
	}
	return "unknown"
}
