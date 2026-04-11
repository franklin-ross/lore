# Lore — Design

## What It Is

A plain-text knowledge base for tabletop RPG campaigns, with a VSCode extension for editing and a CLI for querying. You write entity definitions and session logs in markdown files. The tooling gives you syntax highlighting, autocomplete, hover descriptions, go-to-definition, reverse references, and search.

No database. Files are parsed into an in-memory graph on demand.

## Project Root

A `lore.toml` file marks the project root (like `.git` marks a repo). The CLI and LSP walk up from the current directory to find it.

```toml
# Glob patterns for files to parse (default: **/*.md)
files = ["**/*.md"]

# Paths to ignore
ignore = ["archive"]
```

## The Format

Markdown files contain **entity definitions** and **free text** (session notes, prose, whatever). Entity definitions are the structured part. Free text is carried along and searchable but not parsed for structure.

See [format.md](format.md) for the full specification.

### Entity Definitions

An entity definition has a **header** terminated by `:` and a **description** that continues until the next blank line. The header contains the entity name, optional type in parentheses, and optional aliases separated by `|`. These can appear in any order.

```
Sildar Hallwinter (character) | Sildar: Fighter. Member of the
  Lords Alliance. Was captured at Cragmaw Hideout; we rescued him.

Cragmaw Hideout (location): North of Triboar Trail. Infested
  with goblins. Klarg is the boss.

Find Iarno Albrek (quest): Given by Sildar. Status: active.
  Iarno went missing in Phandalin.
```

Rules:
- **Header** is terminated by `:` — this distinguishes entity definitions from free text
- **Name** is the first non-type segment before any `|`
- **Type** is optional, in parentheses, can appear anywhere in the header. Required on first definition only
- **Aliases** separated by `|`. Any alias works as a heading in later definitions
- **Description** is everything after `:` until the next blank line
- References to other entities are recognised automatically by name matching
- Parenthetical disambiguation: `Barovia (town)` vs `Barovia (nation)`

### Session Logs

Free text that isn't an entity definition. Can reference entities by name. Useful for narrative session notes.

```
# Session 3 — The Cragmaw Hideout

We followed the goblin trail from the ambush site. Found Sildar
captured inside. Fought through goblins and killed Klarg. Sildar
told us Gundren was taken to Cragmaw Castle.
```

### File Organisation

Up to you. The tool reads all files matching the `files` glob in `lore.toml` (default `**/*.md`) and builds the graph from all of them. Some options:

- One file per session + glossary files for entities
- One file per entity type (characters.md, locations.md)
- Everything in one big file
- Any combination

## The Parser

Reads all matching files and produces:

1. **Entity list** — name, type, aliases, source file + line, description text
2. **Reference index** — for each entity, every place it's mentioned (in its own definition, in other entities, in free text)
3. **Reverse reference index** — for each entity, every other entity that mentions it

This is enough to power all the features. No triples, no predicates, no fact extraction. Just entities and cross-references.

### File Ordering

Files are sorted alphabetically by their full path relative to the project root before parsing. This means numeric prefixes control order naturally (`00-intro.md` before `01-beginnings.md`), and folder structure is respected (`glossary/characters.md` before `sessions/01.md`).

When an entity is defined in multiple places, the definition order follows file order. This matters for descriptions — they're concatenated in the order they appear.

### Parsing Strategy

1. Collect and sort all matching files alphabetically by path
2. First pass: find all entity definitions (headers with `(type)` and `:`) and build the entity name list
3. Second pass: find references to known entity names in all text
4. Ambiguity resolved by longest match and parenthetical disambiguation

Performance: even with hundreds of files, parsing is milliseconds. No caching needed initially.

## VSCode Extension

### Language Features (via LSP)

- **Syntax highlighting** — entity definition headers, type annotations, references to known entities
- **Autocomplete** — entity names as you type, with type annotation
- **Hover** — show entity description and type on hover over a reference
- **Go to definition** — jump to the entity's first definition from any reference
- **Find all references** — every mention of an entity across all files
- **Reverse references** — "Sildar is referenced by: Find Iarno Albrek, Session 3"
- **Diagnostics** (soft warnings) — "Iarno Albrek is mentioned but has no definition"

### Implementation

- **Language server** in Go (reuses the parser)
- **TextMate grammar** for basic syntax highlighting (fast, no LSP needed)
- **LSP** for the smart features (autocomplete, hover, go-to-def, references)
- **VSCode extension** in TypeScript, spawns the Go LSP server binary

### Concurrency Model

The LSP runs single-threaded. glsp dispatches every notification and request serially on one goroutine, so handler state (the index, open-buffer set, project) is only ever touched by one handler at a time. Don't add mutexes to these structures — they're dead weight. If parsing ever becomes a bottleneck, fan work out to workers and merge results on a single owner, rather than introducing concurrent mutation of shared state.

## CLI

Secondary to the extension, but useful for terminal and scripting:

```
lore query <name>         # show entity description + reverse references
lore query --type quest   # list all quests
lore list                 # list all entities
lore refs <name>          # show all references to an entity
lore search <text>        # full-text search across all files
lore check                # report undefined references
```

## Tech Stack

- **Parser + CLI + LSP server:** Go
- **VSCode extension:** TypeScript wrapper that spawns the Go binary

## Build Commands

Uses [Task](https://taskfile.dev/). Run `task` for the default pipeline (format, lint, build, test), or see `task --list` for all targets.

## Build Order

### Phase 1 — Parser + CLI (done)

1. `lore.toml` loading (find root, parse config, resolve file globs)
2. Parser: read files → entity list + reference index
3. CLI: `lore list`, `lore query`, `lore refs`, `lore search`, `lore check`
4. Tests against the example files and real campaign notes

### Phase 2 — VSCode Extension

5. TextMate grammar for syntax highlighting
6. LSP server in Go (hover, go-to-def, references, autocomplete)
7. VSCode extension wrapper (TypeScript, spawns LSP binary)
8. Diagnostics for undefined references

### Phase 3 — Polish

9. Watch mode (re-parse on file change)
10. LLM integration: `lore tidy` reads session notes, suggests entity definitions
11. Export: generate summary documents from the entity graph
