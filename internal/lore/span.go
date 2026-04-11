package lore

// descSegment records the mapping from a contiguous run of bytes in a joined
// description string back to its origin in the source file.
type descSegment struct {
	joinedStart int // byte offset in the joined string where this segment begins
	line        int // 1-based file line the segment came from
	column      int // 0-based byte column on that line where the segment's content starts
}

// translateSpans remaps directive spans from joined-description coordinates
// to original-file coordinates using the description's segment table.
func translateSpans(events []StateEvent, issues []StateIssue, segments []descSegment) {
	for i := range events {
		events[i].Span = translateSpan(events[i].Span, segments)
	}
	for i := range issues {
		issues[i].Span = translateSpan(issues[i].Span, segments)
	}
}

// translateSpan remaps a single span from joined-description coordinates to
// original-file coordinates. It finds the segment whose range contains
// span.StartByte and translates Line, StartByte, and EndByte accordingly.
//
// Note: a directive that crosses a segment boundary (e.g. a value that wraps
// across a continuation) will be attributed to the segment containing
// StartByte. This is an acceptable approximation for the current format, where
// directives are typically contained within a single source line.
func translateSpan(span StateSpan, segments []descSegment) StateSpan {
	if len(segments) == 0 {
		return span
	}
	// Find the last segment whose joinedStart is <= span.StartByte.
	seg := segments[0]
	for _, s := range segments {
		if span.StartByte >= s.joinedStart {
			seg = s
		} else {
			break
		}
	}
	offset := span.StartByte - seg.joinedStart
	span.Line = seg.line
	span.StartByte = seg.column + offset
	span.EndByte = seg.column + (span.EndByte - seg.joinedStart)
	return span
}
