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
- Support **tags** (bare labels) and **fields** (named scalars or lists).
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
2. **Fields** — named values. Each field holds one of:
   - A scalar string (`status = alive`).
   - A scalar number (`population = 100`).
   - A list (`inventory = "sword", "shield"`).

A field's type is inferred from its first assignment and fixed thereafter.
Mixing types (e.g. `+=` against a scalar) is a diagnostic, not a runtime
error.

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
- Tag names are identifiers: `[a-zA-Z][a-zA-Z0-9_-]*`. Multi-word tags use
  hyphens (`critically-injured`) or the quoted escape hatch
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

- `field = value` — set or overwrite. For lists, replaces the entire list.
- `field += value` — increment (numeric) or append (list).
- `field -= value` — decrement (numeric) or remove (list).
- Field names follow the same identifier rule as tags.
- Values are one of:
  - A bareword identifier (`alive`, `active`).
  - A number literal (`100`, `-5`, `3.14`).
  - A quoted string (`"two handed sword"`). Multi-word values **must** be
    quoted.
- A list may be initialised from multiple comma-separated values:
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
- **Field lines** — one per field, alphabetically sorted, `name: value`
  format. Lists render as `name: a, b, c` (list items also alphabetical).
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
- `fieldname +=` where the field is a list → values seen in that field
  anywhere in the project.
- `fieldname -=` where the field is a list → only items currently in the
  list for this entity at this point.

All completions are context-sensitive: they consider state resolved up to
the cursor position, not end-of-file state.

### Diagnostics

Surfaced as squiggly underlines in the editor and warnings in `lore check`:

- `-tag` where the tag is not currently active on this entity.
- `field -= value` where `value` is not in the list.
- `field -= N` or `field += N` where `field` has never been initialised.
- Type conflict: `+=` or `-=` against a scalar, or `=` assigning a list to
  a previously-scalar field, or vice versa.
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
    Kind   FieldKind // scalarString, scalarNumber, list
    String string
    Number float64
    List   []string
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
