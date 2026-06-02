# Lore File Format

## Overview

Lore files (`.md` by default, configurable in `lore.toml`) contain entity definitions and free text. The format is designed to be fast to write during a TTRPG session — closer to notes than code.

## Entity Definitions

```
Name (type) | alias1 | alias2: Description. More description.
  Continuation until the next blank line.
```

### Header

The first line of an entity definition, terminated by `:`. Contains:

- **Name** — the canonical name
- **Type** — optional, in parentheses: `(character)`, `(location)`, etc. Can appear anywhere in the header
- **Aliases** — optional, separated by `|`. Any alias can be used as the heading in later definitions
- **Colon** — terminates the header, starts the description

The type, name, and aliases can appear in any order:

```
Count Strahd von Zarovich (character) | Strahd:
Count Strahd von Zarovich | Strahd (character):
(character) Count Strahd von Zarovich | Strahd:
Strahd | Count Strahd von Zarovich (character):
```

These are all equivalent. The first segment (before any `|`) that isn't a `(type)` is the canonical name. The rest are aliases.

### Type

Optional parenthesised word. Only required on the first definition of an entity — subsequent definitions can omit it. Common types: `character`, `location`, `quest`, `item`, `faction`, `event`, `concept`. Any word works.

The type is what distinguishes an entity definition from free text. A line starting with unknown text followed by `:` is only treated as an entity definition if it includes a `(type)` annotation. Once the entity is known, its name or aliases followed by `:` are recognised without the type.

### Disambiguation

When two entities share a name, use the type:

```
Barovia (town): The main town. Dark, gothic, choked with vines.

Barovia (nation): Perpetually cloudy and grey. Nobody can leave.
```

References use the same syntax: `in Barovia (town)` vs `in Barovia (nation)`.

### Description

Everything after the `:` until the next blank line. Free-form prose. Sentences separated by `.` or `;`. Can span multiple lines.

The parser doesn't extract key-value pairs — it carries the text and finds entity references within it.

### Multiple Definitions

An entity can be defined across multiple files or locations. Descriptions are concatenated. Use any name or alias as the heading:

**glossary/characters.md:**
```
Sildar Hallwinter (character) | Sildar: Fighter. Member of the
  Lords Alliance.
```

**sessions/03-cragmaw.md:**
```
Sildar: Was captured at Cragmaw Hideout; we rescued him. Gave
  us quest: Find Iarno Albrek.
```

Both descriptions are combined when you query Sildar.

## State Directives

Entity descriptions can include **state directives** — compact tags and
fields that track the current state of an entity across sessions. The
resolved state is shown at the top of the entity in `lore query` output
and in VSCode hover tooltips.

### Tags

Bare labels you add or remove:

```
Sildar: Took an arrow to the knee. +injured
Sildar: Patched up by the cleric. -injured
Sildar: Got hit again, and it's bad this time. +critically-injured
```

Multi-word tags use hyphens. Quoted escape works too: `+"critically
injured"` is the same tag as `+critically-injured`.

### Fields

Named values. Numeric fields support arithmetic; text fields hold ordered
lists (a single-value list renders as a scalar):

```
Phandalin (location): Sleepy frontier town. population = 100
Phandalin: Redbrands raid the square. population -= 50

Sildar: Hands us his longsword. inventory += longsword
Sildar: Gives the sword to Gundren. inventory -= longsword
```

Multi-word bareword values are fine: `inventory += two-handed sword`.
Quotes are only needed when the value contains punctuation like a comma
or period.

### Terminators

Directives end at the first of `.`, `!`, `?`, `;`, or end-of-line.
Separate multiple directives in one sentence with `;`:

```
Sildar: He's hurt. inventory += helm; health -= 3. We head home.
```

### Diagnostics

The parser warns about likely mistakes: removing a tag that isn't set,
decrementing a field that was never initialised, kind conflicts
(e.g. `population += "sword"` after `population = 100`), missing list
separators between quoted and bareword items, and run-on directives
(`inventory += helm health -= 3.`) where a `;` was probably intended.

See [`docs/specs/2026-04-11-entity-state-tracking.md`](specs/2026-04-11-entity-state-tracking.md)
for the full specification.

## Relations

A **relation** is a typed, directional link between two entities, written
with `->` alongside tags and fields. Declare it once and it renders on
both endpoints.

```
Sarah (person): father -> Doug
Party (group): members -> Aragorn, Bilbo
```

Multiple targets are comma-separated, and relations accumulate across
sessions like list fields. Remove one with `-/>` as the world changes:

```
Guild (group): members -> Borin
Guild: Borin storms out. members -/> Borin
```

Removal is reciprocity-aware: either endpoint can retract the link with
either label. Hover and the wiki show an entity's relations **as of the
cursor position** — the net set after removals, on the same timeline as
state. An undefined label still works (a *generic* edge); the vocabulary
below is pure upgrade.

### The One Rule

The label names **what the target is to the subject**:

```
A: rel -> B          reads as          "B is A's rel."
```

- `Sarah: father -> Doug` → Doug is Sarah's father.
- `Party: members -> Aragorn` → Aragorn is Party's member.

Read it that way and direction never surprises you. The natural-language
idiom "Sarah, *daughter of* Doug" is the other way around, so write it
from the parent (`Doug: daughter -> Sarah`) or possessively from the
child (`Sarah: father -> Doug`) — both record the same link.

### Configuring Relations

The vocabulary is optional config in `lore.toml`. It enriches relations
you reuse enough to care about — without it, `->` still works.

```toml
[relations.parent]
reciprocal = "child"                       # the reverse label, shown on the far side
aliases = ["father", "mother", "dad", "mum"]

[relations.child]
plural = "children"                        # for merged headers; defaults to name + "s"
aliases = ["son", "daughter"]

[relations.spouse]
reciprocal = "spouse"                      # self-reciprocal = symmetric
aliases = ["husband", "wife", "partner"]
```

- **`reciprocal`** — the canonical reverse. Bidirectional: defining one
  way implies the other. A relation whose reciprocal is itself is
  symmetric (`spouse`, `sibling`, `ally`).
- **`aliases`** — surface labels that mean the same relation. Display is
  preserving: Lore keeps the word you typed and uses the canonical only
  for matching, reciprocity, and merging.
- **`plural`** — used in merged headers (`child` → `children`). A relation's
  plural also resolves as an **input label**, so `Cuthbert: children -> Milly,
  Bobby` works as naturally as `child -> Milly`. If the auto-pluralised form
  reads awkwardly, set `plural` explicitly or add a pluralised alias.

Four rules keep a custom relation from misbehaving — the parser warns on
the first two, but the last two are on you:

1. **Reciprocals are one-to-one.** Two relations must not share a
   reciprocal. To cover gendered variants, make them **aliases of one
   canonical**, never separate canonicals — `aunt` and `uncle` are
   aliases of a single relation, not two relations both reversing
   `nibling`. (Lore's built-ins use the neutral `pibling`/`nibling` for
   exactly this reason.)
2. **Reciprocals are mutual.** If `A` reverses `B`, then `B` must reverse
   `A`, not some third relation.
3. **Canonicals are singular nouns.** The canonical is the edge's key,
   the label shown on an undeclared reverse side, and the stem that gets
   pluralised for headers — so a verb breaks it (`contains` would
   pluralise to "containses"). Make the canonical a noun (`contents`) and
   put the verb in `aliases` (`contains`, `holds`). If a noun canonical
   already ends in "s", pin its `plural` to itself.
4. **A verb alias goes on the side whose *subject* performs it** — which
   is the opposite side from the matching noun, because the noun reads
   possessively. `A: leader -> B` means "B is A's leader", so A is the
   follower; the verb `serves` (the subject serves) therefore belongs to
   `leader`, and `leads` to `follower`. Likewise, `owns` sits with
   `possession` (not `owner`), and `made`/`forged` with `creation`.

Built-in vocabulary for common familial, social, membership, containment,
residence, ownership, hierarchy, mentorship, and authorship relations
ships by default; define a relation of the same canonical name to extend
or override it. A bad definition (rules 1–2) is flagged in `lore check`
and as a warning toast in VSCode when the config loads.

See [`docs/specs/2026-06-02-entity-relations.md`](specs/2026-06-02-entity-relations.md)
for the full specification.

## Free Text

Any text that isn't an entity definition. Separated from entity definitions by blank lines. Treated as narrative — searchable, and entity references within it are detected, but it's not attached to any particular entity.

```
# Session 3

We followed the goblin trail and found Sildar captured inside
Cragmaw Hideout. Fought through goblins and killed Klarg.

Sildar: Told us Gundren was taken to Cragmaw Castle.
```

The first paragraph is free text. The second is an addition to Sildar's entry.

## File Organisation

Up to you. The tool reads all Lore files (`.md` by default, configurable in `lore.toml`) in the project recursively. Some options:

- One file per session with inline entity definitions
- Separate glossary files by type (characters.md, locations.md)
- One big file
- Any combination

## Full Example

```
# Session 1 — Barovia

We enter Barovia (town) from the west at night. Misty. Full moon.

Mad Mary (character): Mother of Gertrude. Tells us it's not safe
  outside. Has a doll with "Is no fun, is no Blinsky."

Gertrude (character): Mad Mary's daughter. Missing; smitten with
  and probably taken by Strahd.

Find Gertrude (quest): Given by Mad Mary in Barovia (town).

Blood on the Vine Tavern (location): In Barovia (town). Seen better
  days. Owned by Vistani; run by Aryk. No beer; only wine.

We go to the tavern. 3 Vistani are drunk and rowdy. Another man
looks at us on entry.

Ismark (character): Mayor of Barovia (town). Long blond hair, black
  tunic. Worried Strahd wants to abduct his sister Ireena.

Ireena Kolyana (character) | Ireena: Ismark's sister; ~27; pretty,
  blue eyes, red scarf covering bite marks. Strahd fed on her but
  didn't turn her. Orphan, not blood related to Ismark.

Escort Ireena (quest): Given by Ismark. Take Ireena to Valacki or
  Krezk for safety.

Count Strahd von Zarovich (character) | Strahd: Vampire lord. Lives
  at Castle Ravenloft. Tall, black hair, white skin, princely.
  Doesn't like sunlight or running water; can't enter without
  invitation.

Castle Ravenloft (location): Strahd's castle. Ghostly procession
  marches towards it nightly.

Barovia (town): Gothic, dark, misty, rundown.

Barovia (nation): Perpetually cloudy. Nobody can leave; mist
  around the border.
```
