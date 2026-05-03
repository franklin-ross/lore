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

## Reporting Issues

[github.com/franklin-ross/lore/issues](https://github.com/franklin-ross/lore/issues)
