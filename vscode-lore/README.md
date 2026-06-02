# Lore — TTRPG Knowledge Base

Entity definitions, cross-references, and semantic highlighting for TTRPG campaign notes written in plain markdown.

Lore reads your session notes and prose, recognises entity names, tracks their state across the timeline (`+captured`, `hp = 12`), records typed relations between them (`father -> Doug`), and surfaces it all as hovers, autocomplete, semantic colouring, a wiki view, and a knowledge graph. No database, no frontmatter — just markdown that reads as prose and parses as structure.

See the [main project README](https://github.com/franklin-ross/lore#readme) for the file format, CLI, and a worked example.

## Activation

The extension activates automatically when the workspace contains a `lore.toml`. Run `lore config init` from the campaign root once, and reopen the folder in VSCode.

The extension uses the bundled `lore` binary. Override via `lore.serverPath` if you want a specific build.

## Features

- **Semantic highlighting** with stable per-entity colours. Directive sub-tokens colour too — operators (`= += -= -> -/>`) and tag sigils, state/relation names, list separators, and field values by kind (numbers vs text). Driven by real parsed directives, so prose like `x = y` stays plain.
- **Relations** — typed, directional edges between entities (`father -> Doug`, `members -/> Borin`), declared once and rendered on both endpoints. See [Relations](#relations) below.
- **Hover** showing resolved state at the cursor and the latest state, plus the entity's relations as of that point in the timeline. State resolves to the end of the current definition, not the end of the line.
- **Go to Definition / Find References** via the language server.
- **Autocomplete** for entity names with type-aware disambiguation, relation labels in the directive label slot, and entity targets after a relation arrow (`-> ` / `-/> `).
- **Diagnostics** for undefined references and for relation removals (`-/>`) of relations that were never set.
- **Definition styling** — tinted background or underline on the canonical occurrence of each name.
- **Wiki view** — full picture of an entity (relations, state, history, descriptions, inbound and outbound references). Open via `F12` on a name, the entity tree, or the `Lore: Open Entity Wiki` command. Navigate with the toolbar `←` `→` arrows or the mouse back/forward buttons.
- **Entity tree** in the Explorer sidebar with filter and refresh.
- **Knowledge graph** — interactive force-directed view of every entity and the references between them. See [Knowledge Graph](#knowledge-graph) below.

## Relations

A relation is a typed, directional edge between two entities, written with the `->` operator alongside tags and fields:

```
Sarah (person): father -> Doug
Party (group): members -> Aragorn, Bilbo
```

One declaration renders on both endpoints. With a known vocabulary, the reverse side shows the canonical reciprocal — Sarah's card reads `father → Doug`, Doug's reads `child → Sarah`. An undefined label still works as a generic relation: the reverse falls back to the named-incoming form (`Sarah → bestie`).

Relations are world-state on the timeline, so they accumulate and can be retracted as the world changes, with `-/>`:

```
Guild (group): members -/> Borin
```

Removal is reciprocity-aware — either endpoint retracts the relation with either label. Removing a relation that was never set raises a diagnostic. Hover and the wiki show an entity's relations as of the cursor position, the net set after removals.

Relation vocabulary is optional config in `lore.toml` — it enriches relations you reuse enough to care about (reciprocals, aliases, plurals):

```toml
[relations.parent]
reciprocal = "child"        # bidirectional; child gets reciprocal = parent for free
aliases = ["father", "mother", "dad", "mum"]

[relations.spouse]
reciprocal = "spouse"       # self-reciprocal = symmetric
```

Built-in vocabulary for common familial, social, and membership relations ships by default; extend or override by defining the same name. A bad definition (two relations sharing one reciprocal, or a non-mutual reciprocal) raises a warning toast at project load. See the [main project README](https://github.com/franklin-ross/lore#readme) and the [relations spec](https://github.com/franklin-ross/lore/blob/main/docs/specs/2026-06-02-entity-relations.md) for the full model.

## Commands

All commands appear in the palette under the `Lore:` prefix.

| Command                               | What it does                                                                  |
| ------------------------------------- | ----------------------------------------------------------------------------- |
| `Lore: Open Entity Wiki`              | Prompt for an entity name and open the wiki.                                  |
| `Lore: Open Entity Wiki at Cursor`    | Open the wiki for the entity under the cursor (or the current selection).     |
| `Lore: Refresh Entities`              | Reload the entity tree.                                                       |
| `Lore: Filter Entities`               | Filter the entity tree by name, type, alias, or tag.                          |
| `Lore: Clear Entities Filter`         | Clear the entity-tree filter.                                                 |
| `Lore: Toggle Hover State Directives` | Show/hide inline state directives inside description prose in the hover view. |
| `Lore: Open Knowledge Graph`          | Open the interactive force-directed graph of every entity.                    |

## Keybindings

| Key   | Command                 | When                                       |
| ----- | ----------------------- | ------------------------------------------ |
| `F12` | `lore.openWikiAtCursor` | Markdown file inside a `lore.toml` project |

`F12` only fires inside a lore project, so it doesn't shadow Go to Definition in unrelated markdown files. The extension exposes a context key `lore.inProject` you can use in your own `when` clauses.

To unbind or rebind, use `Preferences: Open Keyboard Shortcuts (JSON)`:

```json
{ "key": "f12", "command": "-lore.openWikiAtCursor" },
{ "key": "alt+f12", "command": "lore.openWikiAtCursor",
  "when": "editorTextFocus && editorLangId == markdown && lore.inProject" }
```

## Settings

| Setting                          | Default          | Description                                                                                 |
| -------------------------------- | ---------------- | ------------------------------------------------------------------------------------------- |
| `lore.serverPath`                | (bundled binary) | Path to the `lore` binary.                                                                  |
| `lore.hover.stateMode`           | `both`           | Which state blocks the hover shows — `both`, `atCursor`, or `latest`.                       |
| `lore.hover.showStateDirectives` | `false`          | Show inline `+tag` / `field = …` directives inside description prose in the hover view.     |
| `lore.definitionStyle`           | `background`     | How to highlight the canonical occurrence of a name — `background`, `underline`, or `none`. |
| `lore.trace.server`              | `off`            | LSP message tracing — `off`, `messages`, or `verbose`.                                      |

## Knowledge Graph

`Lore: Open Knowledge Graph` opens an interactive force-directed view of every entity in the project. Two edge layers are drawn: **relations** (explicit typed edges, bold, labelled at the edge midpoint, arrowheads suppressed on symmetric relations) and **mentions** (faint, reference-derived). Single-click focuses a node (mirrors to the wiki); double-click opens the entity wiki; drag pins a node; click empty space to clear focus.

### Toolbar

- **Edges** — `Relations` / `Mentions` toggles, each an independent on/off layer. Relations default on, mentions off — explicit edges are the less-overwhelming default.
- **Hops** — `1` / `2` / `all`, limits visible nodes to those within N edges of the focused node
- **Types** — opens a quick-pick to filter visible entity types
- **Layout** — picks the simulation algorithm (see below)
- **⚙︎** — opens the settings panel for the active layout

### Layouts

Three force-directed simulations are available; pick whichever reads best for the question you're asking. The settings panel exposes layout-specific sliders, plus a **Restore defaults** button.

#### Force (hierarchical) — default

Two coupled simulations: a small **cluster graph** (one node per entity type, edges weighted by cross-type reference count) settles first, then **members** anchor to their type's cluster centroid while charge spreads them within the cluster. Highly cross-referenced types pull toward each other; isolated types drift to the rim under charge. Result: hub-and-spoke at the cluster level, local fan-out within each cluster.

Splitting the layers means cluster topology converges in tens of ticks regardless of how many entities exist — the member graph then resolves quickly against stable centroid targets.

- **Strengths**: colocates similar types of nodes; minimises distance between busy types; settles quickly even on large graphs
- **Weaknesses**: well-connected nodes may have long edges; small groups are out of sight

#### Force (flat)

Pure link-graph, nodes attracts connected nodes.

- **Strengths**: shows the actual reference structure without type bias; fewer long edges
- **Weaknesses**: weakly-connected entities drift to the rim; no visual grouping if you care about types

#### ForceAtlas2

Gephi's ForceAtlas2 algorithm via [graphology](https://graphology.github.io). Barnes-Hut optimised, scales well to 1k+ nodes. No type-aware clustering — communities emerge from edge density alone.

- **Strengths**: separates densely-linked communities cleanly, especially with **lin-log mode** on; handles uneven graphs better than the d3 forces
- **Weaknesses**: difficult to tune, can produce tight blobs

If you see a cramped overlapping ball with FA2: try `scaling` 50–100, enable `lin-log`, and drop **Hops** to 1 or 2.

If hubs feel "crushed" toward the centre while leaves spread to the rim: keep `dissuade hubs` on (default). It divides each edge's attraction by the source's degree, so hubs aren't yanked toward every connected leaf.

`edge influence` only bites when `lin-log` is off. Lin-log replaces linear attraction `d` with `log(1+d)`, which compresses distance so much that the weight exponent has little visible effect. Turn lin-log off to see edge weight respond to the slider.

### Settings persistence

Each layout persists slider and toggle values independently. Switching from `Force (hierarchical)` to `ForceAtlas2` and back restores both layouts' last-tuned state.

## Reporting Issues

[github.com/franklin-ross/lore/issues](https://github.com/franklin-ross/lore/issues)
