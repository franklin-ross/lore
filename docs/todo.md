# Outstanding Work

## Bugs

- ~~**Self-references are noisy** — fixed: `GetReferences` now filters self-references.~~
- ~~**`(type)` must sit adjacent to the name** — fixed: header parser now requires `(type)` to be at the leading or trailing edge of a `|`-segment; mid-segment parens disqualify the line from typed parsing.~~

## Incomplete Features

- **`check` command — undefined references** — `check` now surfaces state-directive diagnostics (via [`docs/specs/2026-04-11-entity-state-tracking.md`](specs/2026-04-11-entity-state-tracking.md)), but still doesn't detect undefined entity references. Should scan all text for potential entity names that don't have definitions and report them as warnings.
- **Error messages** — `lore` outside a project directory shows a raw error. Should say something like `No lore.toml found. Run from within a lore project, or create one with lore init.`

## Tests

- **Multi-file e2e test** — e2e tests only use a single fixture file. Add tests with glossary + session files to verify cross-file entity accumulation and reference detection.
- **Test against real notes** — run the parser against the Strahd campaign notes to verify nothing breaks on real-world markdown.

## Future (Phase 2+)

- ~~**VSCode extension** — done: LSP server (`lore lsp`) with hover, go-to-def, references, autocomplete, semantic tokens. VSCode extension in `vscode-lore/`.~~
- **Distribute the `lore` binary via the extension** — the extension currently spawns bare `lore`, which fails with `ENOENT` whenever VS Code's PATH doesn't include wherever the user installed it (e.g., launched from Finder). Ship per-arch binaries via GitHub Releases and have the extension download the right one on first activation. Spec: [docs/specs/extension-binary-distribution.md](specs/extension-binary-distribution.md). Blocked on creating a public GitHub repo for `lore`.
- **`lore init`** — create a `lore.toml` in the current directory.
- ~~**Watch workspace for out-of-editor changes** — done: the LSP dynamically registers `workspace/didChangeWatchedFiles` at `initialized` time for every project glob plus `lore.toml`. Disk events refresh tracked files straight from disk, skipping anything currently held by an editor buffer (those reconcile on `didClose`). A `lore.toml` change triggers a full project reload while preserving open-buffer content.~~
- **State tracking follow-ups** — deferred from [`docs/specs/2026-04-11-entity-state-tracking.md`](specs/2026-04-11-entity-state-tracking.md): point-in-time queries (`lore query --at session-5`), state-based CLI filters (`lore query --tag injured`), casing-drift diagnostic (`+Injured` vs `+injured`), LSP completion of field names and list item values beyond tags, and faded/italic rendering of directives in hover + dim ANSI in CLI.
- **LLM tidy** — `lore tidy` reads session notes, suggests entity definitions.
- **Export** — generate summary documents from the entity graph.
