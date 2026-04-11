# Spec: Entity State Tracking

## Problem

Lore entities currently carry only a description — free-form prose accumulated
across files in file order. That works for most narrative facts, but misses
state that changes over time and should be visible at a glance:

- Conditions on NPCs (`injured`, `bleeding`, `on-fire`, `dead`).
- Numeric counts (town populations, gold, quest progress).
- Inventories and lists (what an NPC is carrying, who is in a party).

Today you either bake these into prose (and lose the current value at a glance)
or omit them. This spec adds first-class state tracking while keeping the
format close to plain notes.

## Goals

- Show the **current state** of an entity in hover, `lore query`, and other
  renders.
- Support **tags** (bare labels) and **fields** (named numeric or text
  values).
- **Validate** state operations and surface inconsistencies as diagnostics.
- Keep syntax natural enough to write during a live session.

## Non-Goals (v1)

- Point-in-time queries (`lore query Sildar --at session-5`). History is
  stored so this can be added later without a format change.
- State-based CLI filters (`lore query --tag injured`). Trivial to add later.
- Computed or derived fields.
- Cross-entity state references (`+owned-by Sildar`).
- Arithmetic on numeric fields beyond `+=` / `-=`.
- List-reset syntax (`inventory = []` or similar). Enumerated removals are
  the intended reset path; `[value]` would also collide with markdown link
  syntax.

## Model

Two kinds of state, both attached to entities:

1. **Tags** — bare labels, present or absent. `injured`, `bleeding`,
   `on-fire`. A tag is either set or not; there is no value.
2. **Fields** — named values. A field is one of two **kinds**, fixed by its
   first assignment:
   - **Numeric** — holds a single number. Supports arithmetic: `= 5`,
     `+= 5`, `-= 5`. Example: `population = 100`.
   - **Text** — holds an ordered list of strings. A length-1 list and a
     "scalar" are the same thing; the display logic renders short lists
     as scalars. Supports `= x` (reset the list to `[x]`), `+= x`
     (append), `-= x` (remove). Examples: `status = alive`,
     `inventory = "sword", "shield"`.

The kind is determined by the **syntactic form** of the first value, not by
what it parses to:

- **Numeric** if and only if the first value is an unquoted number literal
  (`100`, `-5`, `3.14`).
- **Text** in every other case: a bareword identifier (`alive`), a single
  quoted string (`"sword"`), or a comma-separated list of those
  (`"sword", "shield"`).

A quoted digit string is **always** text, even though it looks like a
number:

```
room-code = "123"             # text field, single item ["123"]
room-code += "456"            # text append → ["123", "456"]
room-code -= "123"            # text remove → ["456"]

population = 100              # numeric field
population += 50              # numeric → 150
population += "50"            # error: cannot append text to numeric field
```

This rule is deliberately mechanical. A reader looking at a single
directive can always tell the field's kind from the surface form without
needing to know what the author meant. The cost is that
`something = "123"` is a text field even if you meant a number — a small
price for predictability. If you want a number, drop the quotes.

Mixing kinds is the only type error: `population += "sword"` after
`population = 100` is a diagnostic, as is `status = 42` after
`status = alive`. The spec does not convert between kinds — the author
must be explicit.

State is resolved in **file order** — the same ordering used for description
accumulation (alphabetical by path relative to the project root). Directives
within a single file are processed top to bottom.

## Syntax

State directives live **inside entity description bodies** only, never in
headers. Headers remain clean: name, type, aliases. Directives are mixed
freely with prose.

### Tags

```
Sildar: Took an arrow to the knee. +injured
Sildar: Patched up by the cleric. -injured
Sildar: Got hit again, and it's bad this time. +critically-injured
```

- `+tag` sets the tag.
- `-tag` clears the tag.
- Tag names are identifiers: the first character is a Unicode letter
  (`\p{L}`), and subsequent characters are Unicode letters, Unicode
  digits (`\p{N}`), `_`, or `-`. This accepts non-ASCII word characters
  (`+blessèd`, `+mörkö`, `+呪われた`). Multi-word tags use hyphens
  (`critically-injured`) or the quoted escape hatch
  (`+"critically injured"`); the quoted form is normalised by replacing
  spaces with hyphens, so `+"critically injured"` and `+critically-injured`
  refer to the same tag.

### Fields

```
Phandalin (location): Sleepy frontier town. population = 100
Phandalin: Redbrands raid the square. population -= 50
Phandalin: Refugees from Thundertree arrive. population += 100

Sildar: Hands us his longsword. inventory += "longsword"
Sildar: Gives the sword to Gundren. inventory -= "longsword"
```

- `field = value` — for numeric fields, set the number. For text fields,
  reset the list to `[value]`.
- `field = a, b, c` — only valid for text fields; resets the list to the
  given items.
- `field += value` — for numeric fields, increment. For text fields,
  append.
- `field -= value` — for numeric fields, decrement. For text fields,
  remove the matching item.
- Field names follow the same identifier rule as tags.
- Values are one of:
  - A number literal (`100`, `-5`, `3.14`) — makes the field numeric.
  - A bareword identifier (`alive`, `active`) — makes the field text.
  - A quoted string (`"two handed sword"`) — makes the field text.
    Multi-word values **must** be quoted.
- A text list may be initialised from multiple comma-separated values:
  `inventory = "longsword", "shield", "torch"`. Quoted strings are atomic
  and may contain commas.

### Resetting a List

There is no dedicated reset syntax. To empty a list, remove each item with
`-=`. Autocomplete on `-=` suggests only items currently in the list for
this entity, so the cost is low and the audit trail is preserved.

### Recognising Directives in Prose

A token sequence is a directive only if it matches the directive grammar
exactly. Otherwise it is prose. Directives are recognised by shape, not
position — a `+tag` sitting in the middle of a sentence is still a
directive. Authors who want a literal `+` in prose should phrase around it.

### Directive Termination

A directive ends at the first **terminator** encountered outside a quoted
string or number literal. Terminators are:

- Sentence punctuation: `.`, `!`, `?`, `;`
- End of line
- The blank line that ends the entity definition

Everything between the directive's start and its terminator is parsed as
the directive body. Prose after the terminator is unaffected.

```
Sildar: Took an arrow. +injured He's in bad shape.
```

Here `+injured` ends at the space before `He's` because tags are
single-token directives. Field and list directives extend further:

```
We looted the body. inventory += "sword", "rations x2". We kept walking.
```

- `inventory += "sword", "rations x2"` is the directive; it terminates
  at the `.` after `"rations x2"`.
- `We kept walking.` is prose.

The `.` inside `population = 3.14` is part of the number literal and does
not terminate. The `,` inside `"rations x2"` (if there were one) would be
part of the quoted string.

If a directive contains invalid grammar before its terminator — a trailing
comma, a comma followed by a non-value, a malformed number — it is
reported as a diagnostic and the offending span is highlighted. This means
an author writing `inventory += sword, shield, and we kept walking.` will
see a squiggle: `and` parses as a bareword value, but `we` has no
preceding comma, so the directive is malformed. The fix is to end the
directive sooner (`inventory += sword, shield. And we kept walking.`) or
quote properly.

The practical guidance is simple: **end list directives with sentence
punctuation before continuing prose**. This is natural to write and keeps
the parser predictable.

## Display

Current state is rendered in a compact block above the description text,
everywhere entities are shown (LSP hover, `lore query`, CLI output).

```
Sildar Hallwinter (character)
  +bleeding +injured
  inventory: chainmail, longsword
  ---
  Fighter, member of the Lords Alliance. Was captured at Cragmaw Hideout;
  we rescued him. Took an arrow to the knee. +injured Got hit again, and
  it's bad this time. +critically-injured
```

Rules:

- **Tags line** — all active tags, each prefixed with `+`, space-separated,
  alphabetically sorted. Omitted if no tags are active.
- **Field lines** — one per field, alphabetically sorted. Numeric fields
  render as `name: N`. Text fields render as `name: value` if the list
  has one item, or `name: a, b, c` (items alphabetical) if multiple.
  Omitted if no fields exist.
- **Separator** — `---` between the state block and the description, only
  when a state block is present.
- **Empty state** — no block rendered at all.
- **Directives in description** — left **visible** in the rendered
  description text so the reader can see where state came from. LSP renders
  them in a faded/italic style; CLI uses dim ANSI where supported.

## LSP

### Completions

- `+` in a description → every tag name seen anywhere in the project,
  alphabetically sorted.
- `-` in a description → only tags currently active on this entity at this
  point in the file (i.e., applied by directives earlier in file order).
- `fieldname` at the start of a directive → existing fields on this entity
  first, then other fields seen project-wide.
- `fieldname +=` where the field is text → values seen in that field
  anywhere in the project.
- `fieldname -=` where the field is text → only items currently in the
  list for this entity at this point.

All completions are context-sensitive: they consider state resolved up to
the cursor position, not end-of-file state.

### Diagnostics

Surfaced as squiggly underlines in the editor and warnings in `lore check`:

- `-tag` where the tag is not currently active on this entity.
- `field -= value` against a text field where `value` is not in the list.
- `field -= N` or `field += N` where `field` has never been initialised.
- Kind conflict: mixing numeric and text operations on the same field
  (e.g. `population += "sword"` after `population = 100`, or
  `status = 42` after `status = alive`). The diagnostic points at the
  offending value and cites the initial assignment's location.
- Casing drift: `+Injured` here but `+injured` elsewhere → info-level hint.

Every diagnostic points at the offending token (tag, value, or field name),
not the whole directive.

## Parser and Data Model

### Parsing

Directives are lexed inside description bodies as part of the existing
description pass. The lexer recognises the shapes `+TAG`, `-TAG`, and
`FIELD OP VALUE` where `OP` is one of `=`, `+=`, `-=`. Tokens that do not
match a directive shape are prose.

### Data Model

Added to `Entity`:

```go
type Entity struct {
    // ...existing fields...
    Tags         map[string]bool
    Fields       map[string]FieldValue
    StateHistory []StateEvent
}

type FieldValue struct {
    Kind   FieldKind // numeric, text
    Number float64  // valid when Kind == numeric
    Text   []string // valid when Kind == text; length >= 1
}

type StateEvent struct {
    Op     StateOp // set, add, remove
    Target string  // tag or field name
    Value  *FieldValue
    Source SourceLocation
}
```

`Tags` and `Fields` hold the fully-resolved current state. `StateHistory`
preserves every directive in file order so diagnostics can reference
earlier events and a future point-in-time query feature has everything it
needs without a format change.

### Resolution

After all descriptions are collected, a single state-resolution pass walks
each entity's `StateHistory` in order and folds it into `Tags` and
`Fields`, emitting diagnostics as it goes. This mirrors the existing
description accumulation pass and runs in O(total directives).

## Migration

State tracking is purely additive. Existing files with no directives
continue to parse and render exactly as before — entities with no state
render no state block.
