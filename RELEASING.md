# Releasing

Releases are built and published by GitHub Actions. Pushing a semver tag triggers cross-platform CLI builds, packages the VSCode extension, and creates a GitHub Release with all artifacts attached.

## Prerequisites

- All changes merged to `main`.
- CI green on `main` (`go vet`, `go test`, `golangci-lint`, VSCode bundle).
- Working tree clean.

## Steps

### 1. Verify Locally

```bash
task              # format, lint, build, test
task test-e2e     # end-to-end
```

### 2. Cut the Release

Auto-bump from the current `vscode-lore/package.json`:

```bash
task release -- patch         # 1.2.3 → 1.2.4
task release -- minor         # 1.2.3 → 1.3.0
task release -- major         # 1.2.3 → 2.0.0
task release -- prerelease    # 1.2.3 → 1.2.4-rc.0
```

Or pin an exact version:

```bash
task release -- v1.0.0
```

Either form bumps `vscode-lore/package.json` (and `package-lock.json`), commits `release: vX.Y.Z`, creates an annotated tag, and pushes the branch and tag. The tag push triggers the release workflow.

Tags must be semver and start with `v` (e.g. `v1.2.3`, `v1.2.3-rc.1`). `vsce package` reads the bumped `package.json` so the VSIX manifest matches the tag. The CLI binary is built with `-ldflags -X main.Version=$TAG` so `lore version` reports the released version.

### 3. Wait for the Release Workflow

The tag push triggers `.github/workflows/release.yml`, which:

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

| Task                     | Description                                                                |
| ------------------------ | -------------------------------------------------------------------------- |
| `task`                   | Format, lint, build, test (default)                                        |
| `task build`             | Build CLI to `bin/lore`                                                    |
| `task build-vscode`      | Bundle the VSCode extension                                                |
| `task package-vscode`    | Package the extension as `bin/lore.vsix`                                   |
| `task lint`              | Run `golangci-lint`                                                        |
| `task test`              | Run unit + e2e tests                                                       |
| `task test-unit`         | Unit tests only (forwards `-- -run X`)                                     |
| `task test-e2e`          | End-to-end CLI tests                                                       |
| `task install`           | Build + install CLI to `~/bin` and VSCode ext                              |
| `task version`           | Print the current version (from `vscode-lore/package.json`)                |
| `task release -- <bump>` | Bump (`patch`/`minor`/`major`/`prerelease` or `vX.Y.Z`), commit, tag, push |
| `task clean`             | Remove build artifacts                                                     |
