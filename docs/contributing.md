# Contributing

## Commit Messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/).

Format: `<type>: <description>`

Types:

- **feat** — new feature or functionality
- **fix** — bug fix
- **test** — adding or updating tests
- **docs** — documentation changes
- **style** — purely stylistic or whitespace changes
- **refactor** — code changes that don't add features or fix bugs
- **chore** — build, tooling, or dependency changes

Optionally add a scope in parentheses to clarify context:

`<type>(<scope>): <description>`

Examples:

```
feat: implement entity disambiguation by type
feat(parser): support known-entity re-definition without type
fix(refs): self-references appearing in query output
test(e2e): add multi-file tests
docs(format): update spec with disambiguation syntax
refactor(parser): extract entity matching into separate module
chore: add zlob dependency for glob matching
```

Keep the description short (under 72 characters), lowercase, imperative mood.

## Workflow

Work test-first: write a failing test that demonstrates the expected behaviour, then write the code to make it pass. This applies to bug fixes (reproduce the bug as a test first) and new features alike.

## File Organisation

Keep source files focused and short — a few hundred lines is a good upper bound. Split by responsibility, not by size alone:

- **`internal/lore/entity.go`** — data types and shared helpers
- **`internal/lore/world.go`** — the `World` container and query methods
- **`internal/lore/parser.go`** — the parsing pipeline (entity definitions, reference detection)
- **`internal/lore/config.go`** — project config, file discovery, glob matching
- **`cmd/main.go`** — CLI entry point and command handlers

Unit tests are colocated with their source (`entity_test.go`, `parser_test.go`, etc.) and use `fstest.MapFS` for in-memory filesystems. End-to-end tests in `e2e_test.go` shell out to the compiled binary.

## Build

This project uses [Task](https://taskfile.dev/). Run `task` for the default pipeline (format, lint, build, test), or individual tasks:

```
task build       # build the binary
task format      # gofmt
task lint        # golangci-lint
task test        # all tests (unit + e2e)
task test-unit   # unit tests only
task test-e2e    # e2e tests only
```
