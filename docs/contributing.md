# Contributing

## Commit Messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/).

Format: `<type>: <description>`

Types:

- **feat** — new feature or functionality
- **fix** — bug fix
- **test** — adding or updating tests
- **docs** — documentation changes
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

- **`entity.zig`** — data types and shared helpers
- **`world.zig`** — the `World` container and query methods
- **`parser.zig`** — the parsing pipeline (entity definitions from text)
- **`refs.zig`** — reference detection pass (analysis over parsed data)
- **`config.zig`** — project config, file discovery, glob matching
- **`main.zig`** — CLI entry point and command handlers

Unit tests stay inline in their module unless the file is already very long, then break them into module_test.zig. Integration tests that exercise the full pipeline go in `lore_test.zig`, following Zig's `_test.zig` convention.
