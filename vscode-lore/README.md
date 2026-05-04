# Lore — TTRPG Knowledge Base

Entity definitions, cross-references, and semantic highlighting for TTRPG campaign notes written in plain markdown.

Lore reads your session notes and prose, recognises entity names, tracks their state across the timeline (`+captured`, `hp = 12`), and surfaces it as hovers, autocomplete, semantic colouring, and a wiki view. No database, no frontmatter — just markdown that reads as prose and parses as structure.

See the [main project README](https://github.com/franklin-ross/lore#readme) for the file format, CLI, and a worked example.

## Activation

The extension activates automatically when the workspace contains a `lore.toml`. Run `lore config init` from the campaign root once, and reopen the folder in VSCode.

The extension uses the bundled `lore` binary. Override via `lore.serverPath` if you want a specific build.

## Features

- **Semantic highlighting** with stable per-entity colours.
- **Hover** showing resolved state at the cursor and the latest state.
- **Go to Definition / Find References** via the language server.
- **Autocomplete** for entity names with type-aware disambiguation.
- **Diagnostics** for undefined references.
- **Definition styling** — tinted background or underline on the canonical occurrence of each name.
- **Wiki view** — full picture of an entity (state, history, descriptions, inbound and outbound references). Open via `F12` on a name, the entity tree, or the `Lore: Open Entity Wiki` command.
- **Entity tree** in the Explorer sidebar with filter and refresh.
- **Knowledge graph** — interactive force-directed view of every entity and the references between them. See [Knowledge Graph](#knowledge-graph) below.

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

`Lore: Open Knowledge Graph` opens an interactive force-directed view of every entity in the project, with edges drawn between entities that reference each other. Single-click focuses a node (mirrors to the wiki); double-click opens the entity wiki; drag pins a node; click empty space to clear focus.

### Toolbar

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
