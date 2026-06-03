package lore

// FieldKind is the kind of a field value. A field's kind is fixed at its
// first assignment and cannot change.
type FieldKind int

const (
	FieldUnknown FieldKind = iota
	FieldNumeric
	FieldText
)

// FieldValue holds the current value of a field. Only the member matching
// Kind is meaningful.
type FieldValue struct {
	Kind   FieldKind
	Number float64  // valid when Kind == FieldNumeric
	Text   []string // valid when Kind == FieldText; len >= 1
}

// StateOp is the kind of state-changing operation a StateEvent represents.
type StateOp int

const (
	StateOpUnknown    StateOp = iota
	StateOpAdd                // +tag
	StateOpRemove             // -tag, field -= value
	StateOpSet                // field = value
	StateOpIncrement          // field += value
	StateOpEdgeAdd            // label -> target[, target]
	StateOpEdgeRemove         // label -/> target[, target]
)

// StateSpan is the byte span of a directive within its source file.
type StateSpan struct {
	File      string
	Line      int // 1-based line of the description's header
	StartByte int // byte offset within the description text
	EndByte   int // exclusive
}

// StateEvent is a single parsed state directive. For tag operations
// (StateOpAdd/StateOpRemove with no Value) Target holds the tag name and
// Value is nil. For field operations Target holds the field name and Value
// is non-nil.
type StateEvent struct {
	Op     StateOp
	Target string
	Value  *FieldValue
	Span   StateSpan
	// OpSpan and NameSpan locate the sub-tokens within Span for highlighting:
	// OpSpan is the operator/sigil/arrow (`+`, `=`, `+=`, `-=`, `->`, `-/>`),
	// NameSpan is the tag name, field name, or relation label. Both are in the
	// same coordinate space as Span and translate with it.
	OpSpan   StateSpan
	NameSpan StateSpan
	// ValueSpan locates a field directive's value (numeric or text). Empty for
	// tags and edges (edge targets are entities, highlighted on their own).
	// Pair it with Value.Kind to colour the value as a number or string.
	ValueSpan StateSpan
}

// StateIssue is a diagnostic produced by directive parsing or state
// resolution. Severity controls how it is displayed.
type StateIssue struct {
	Severity StateIssueSeverity
	Message  string
	Span     StateSpan
}

// StateIssueSeverity classifies a state issue for display purposes.
type StateIssueSeverity int

const (
	SeverityInfo StateIssueSeverity = iota
	SeverityWarning
	SeverityError
)
