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
