# Spec: Distribute the `lore` Binary via the VSCode Extension

## Problem

The VSCode extension spawns a bare `lore` command, which fails with `ENOENT`
whenever VSCode's `PATH` does not include the binary's location. This is the
default state when launching VSCode from Finder on macOS. Bundling a single
host-arch binary inside the VSIX would fix the immediate launch failure, but
locks the extension to one architecture and bloats every install with a binary
the user may not be able to run.

## Approach

Publish per-architecture binaries on GitHub Releases, and have the extension
download the matching binary for the user's host on first activation.

### Prerequisites

- Create a public GitHub repository for `lore`.
- Add a release workflow (`goreleaser` via GitHub Actions is the obvious
  choice) that builds and uploads the following targets on every tag:
  - `darwin-amd64`, `darwin-arm64`
  - `linux-amd64`, `linux-arm64`
  - `windows-amd64`
- Tag releases with semantic versions matching the extension's `package.json`
  version (e.g. extension `v0.1.0` → release `v0.1.0`).

### Extension Behaviour

On activation, resolve the server binary in this order:

1. `lore.serverPath` setting, if set — this remains the escape hatch for local
   development and custom builds.
2. Cached binary at `context.globalStorageUri/bin/lore-<extension-version>`,
   if it exists and is executable.
3. Download from
   `https://github.com/<owner>/lore/releases/download/v<extension-version>/lore-<os>-<arch>`,
   write it to the cache path above with the executable bit set, then use it.

Pinning by extension version (not `latest`) ensures an older VSIX never pulls
a newer, possibly incompatible server.

### First-Run UX

- Show a VSCode progress notification while downloading
  ("Downloading Lore language server…").
- On success, start the language client normally.
- On failure (offline, network error, missing release asset), show an error
  message with two actions:
  - **Retry** — re-run the download.
  - **Set path manually** — open the `lore.serverPath` setting.
- Do not crash the extension host; the rest of VSCode should remain usable.

### Cache Layout

```
<globalStorageUri>/
  bin/
    lore-0.1.0          # binary, chmod +x on unix
    lore-0.1.0.sha256   # downloaded alongside, verified before use
```

Old versions can be left in place; they are small and the user may downgrade.
A `lore.clearCache` command can be added later if disk usage becomes an issue.

### Integrity

Releases should publish a `SHA256SUMS` file. The extension downloads the
matching `.sha256` for the target binary and verifies before marking it as
usable. A mismatch deletes the file and surfaces the same error UI as a
network failure.

## Out of Scope

- Auto-update of the binary independent of extension updates. If a user wants
  a newer server, they update the extension.
- Bundling the binary inside the VSIX for offline installs. Users in offline
  environments should use `lore.serverPath` to point at a manually placed
  binary.
- Code signing / notarization for macOS. The downloaded binary will trip
  Gatekeeper on first run; document the `xattr -d com.apple.quarantine`
  workaround until signing is set up.

## Migration

Until this is implemented, `lore` must be installed manually and made
available on `PATH` (or `lore.serverPath` set explicitly). The current
`Taskfile.yml` `install` task continues to cover the local-dev case.
