# Outstanding Work

## Bugs

- **Self-references are noisy** — an entity's own definition line shows up as a reference to itself in `refs` and `query` output. Should filter these out.

## Incomplete Features

- **`check` command** — placeholder, doesn't detect undefined references yet. Should scan all text for potential entity names that don't have definitions and report them as warnings.
- **Error messages** — `lore` outside a project directory shows a raw error. Should say something like `No lore.toml found. Run from within a lore project, or create one with lore init.`

## Tests

- **Multi-file e2e test** — e2e tests only use a single fixture file. Add tests with glossary + session files to verify cross-file entity accumulation and reference detection.
- **Test against real notes** — run the parser against the Strahd campaign notes to verify nothing breaks on real-world markdown.

## Future (Phase 2+)

- **VSCode extension** — TextMate grammar, LSP server, syntax highlighting, autocomplete, hover, go-to-def, find all references.
- **`lore init`** — create a `lore.toml` in the current directory.
- **Watch mode** — re-parse on file change for LSP.
- **LLM tidy** — `lore tidy` reads session notes, suggests entity definitions.
- **Export** — generate summary documents from the entity graph.
