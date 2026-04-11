# Outstanding Work

## Bugs

- ~~**Self-references are noisy** — fixed: `GetReferences` now filters self-references.~~
- ~~**`(type)` must sit adjacent to the name** — fixed: header parser now requires `(type)` to be at the leading or trailing edge of a `|`-segment; mid-segment parens disqualify the line from typed parsing.~~

## Incomplete Features

- **`check` command** — placeholder, doesn't detect undefined references yet. Should scan all text for potential entity names that don't have definitions and report them as warnings.
- **Error messages** — `lore` outside a project directory shows a raw error. Should say something like `No lore.toml found. Run from within a lore project, or create one with lore init.`

## Tests

- **Multi-file e2e test** — e2e tests only use a single fixture file. Add tests with glossary + session files to verify cross-file entity accumulation and reference detection.
- **Test against real notes** — run the parser against the Strahd campaign notes to verify nothing breaks on real-world markdown.

## Future (Phase 2+)

- ~~**VSCode extension** — done: LSP server (`lore lsp`) with hover, go-to-def, references, autocomplete, semantic tokens. VSCode extension in `vscode-lore/`.~~
- **Distribute the `lore` binary via the extension** — the extension currently spawns bare `lore`, which fails with `ENOENT` whenever VS Code's PATH doesn't include wherever the user installed it (e.g., launched from Finder). Ship per-arch binaries via GitHub Releases and have the extension download the right one on first activation. Spec: [docs/specs/extension-binary-distribution.md](specs/extension-binary-distribution.md). Blocked on creating a public GitHub repo for `lore`.
- **`lore init`** — create a `lore.toml` in the current directory.
- **Watch workspace for out-of-editor changes** — the LSP already re-parses on `didOpen`/`didChange`/`didSave`, but git pulls, branch switches, and files written by other tools leave the index stale until the user reopens a file. Handle `workspace/didChangeWatchedFiles` (or run an fsnotify watcher) to refresh affected files.
- **LLM tidy** — `lore tidy` reads session notes, suggests entity definitions.
- **Export** — generate summary documents from the entity graph.
