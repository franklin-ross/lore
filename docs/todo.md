# Outstanding Work

## Bugs

- **Disambiguation broken** — `Barovia (town)` and `Barovia (nation)` should be two separate entities. Currently `findOrCreateEntity` matches on name alone, ignoring the type qualifier. Needs a composite key of name + type, with disambiguation syntax in references.
- **Known-entity re-definition without type** — `Strahd: Showed up at the funeral.` gets skipped because the parser requires `(type)` on every definition. Should recognise known names and aliases followed by `:` and attach the description to the existing entity.
- **Self-references are noisy** — an entity's own definition line shows up as a reference to itself in `refs` and `query` output. Should filter these out.

## Incomplete Features

- **`check` command** — placeholder, doesn't detect undefined references yet. Should scan all text for potential entity names that don't have definitions and report them as warnings.
- **Error messages** — `lore` outside a project directory shows `Error: error.NoLoreToml`. Should say something like `No lore.toml found. Run from within a lore project, or create one with lore init.`

## Tests

- **Multi-file e2e test** — e2e tests only use a single fixture file. Add tests with glossary + session files to verify cross-file entity accumulation and reference detection.
- **Test against real notes** — run the parser against the Strahd campaign notes (`/Users/franklinross/code/ttrpgs/campaigns/strahd/`) to verify nothing breaks on real-world markdown.
- **Disambiguation tests** — once the bug is fixed, test that `Barovia (town)` and `Barovia (nation)` are separate entities and references resolve correctly.

## Future (Phase 2+)

- **VSCode extension** — TextMate grammar, LSP server, syntax highlighting, autocomplete, hover, go-to-def, reverse references panel.
- **`lore init`** — create a `lore.toml` in the current directory.
- **Watch mode** — re-parse on file change for LSP.
- **LLM tidy** — `lore tidy` reads session notes, suggests entity definitions.
- **Export** — generate summary documents from the entity graph.
