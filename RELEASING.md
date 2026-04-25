# Releasing

Releases are built and published by GitHub Actions. Pushing a semver tag triggers cross-platform CLI builds, packages the VSCode extension, and creates a GitHub Release with all artifacts attached.

## Prerequisites

- All changes merged to `main`.
- CI green on `main` (`go vet`, `go test`, `golangci-lint`, VSCode bundle).
- `vscode-lore/package.json` `version` bumped if the extension changed.

## Steps

### 1. Verify Locally

```bash
task              # format, lint, build, test
task test-e2e     # end-to-end
```

### 2. Tag

Use [semantic versioning](https://semver.org). Tags must start with `v`.

```bash
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

### 3. Wait for the Release Workflow

Pushing the tag triggers `.github/workflows/release.yml`, which:

1. Checks out the tagged commit.
2. Cross-compiles the CLI for:
   - `lore-darwin-arm64` (macOS Apple Silicon)
   - `lore-darwin-amd64` (macOS Intel)
   - `lore-linux-amd64`
   - `lore-linux-arm64`
   - `lore-windows-amd64.exe`
3. Packages the VSCode extension as `lore.vsix`.
4. Creates a GitHub Release with auto-generated notes.
5. Attaches all binaries and the `.vsix` as release assets.

### 4. Verify

Check the [Releases](../../releases) page — all six assets should be present and the notes should reflect commits since the previous tag.

## CI Pipeline

Every push to `main` and every PR runs `.github/workflows/ci.yml`:

- **Go job**: `go vet ./...`, `go test ./...`, `golangci-lint run ./...`.
- **VSCode job**: `npm install` + `npm run bundle` in `vscode-lore/`.

## Local Task Reference

| Task                  | Description                                       |
| --------------------- | ------------------------------------------------- |
| `task`                | Format, lint, build, test (default)               |
| `task build`          | Build CLI to `bin/lore`                           |
| `task build-vscode`   | Bundle the VSCode extension                       |
| `task package-vscode` | Package the extension as `bin/lore.vsix`          |
| `task lint`           | Run `golangci-lint`                               |
| `task test`           | Run unit + e2e tests                              |
| `task test-unit`      | Unit tests only (forwards `-- -run X`)            |
| `task test-e2e`       | End-to-end CLI tests                              |
| `task install`        | Build + install CLI to `~/bin` and VSCode ext     |
| `task clean`          | Remove build artifacts                            |
