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
