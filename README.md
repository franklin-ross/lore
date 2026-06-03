# Lore

A plan-text knowledge base for TTRPGs, including VSCode plugin. Take structured notes fluently while writing session notes and prose.

Write entity definitions and session notes in markdown. Lore parses them into a graph and gives you a CLI for queries plus a VSCode extension with semantic highlighting, hover, go-to-definition, autocomplete, and reverse references.

No database. No frontmatter. Just markdown that reads as prose and parses as structure.

## Example

```markdown
party (group):
inventory = large egg
gold = 53
silver = 11
copper = 5

# Session 3

Sildar Hallwinter (npc) | Sildar: +warrior +missing. Member of the Lords Alliance. Taken by goblins.

Cragmaw Hideout (location): North of Triboar Trail. Infested with goblins. (Klarg (npc): goblin boss) is the boss.

We followed the goblin trail, ambushed (Klarg: +dead), and found (Sildar: -missing +injured) inside Cragmaw Hideout. Sildar told us Gundren was taken to (Cragmaw Castle (location): An old keep infested by goblins. North of Cragmaw Hideout).

We found somem goblin loot, but the barbarian ate the egg for breakfast.

party:
gold += 350; silver += 3
inventory += 7 goblin ears, ruby
inventory -= large egg
```

The header before the first `:` defines an entity. Aliases (`| Sildar`) and types (`(character)`) work in any order. References to known entity names in later prose are recognised automatically. See [docs/format.md](docs/format.md) for the full spec.

## Features

- **CLI**: `lore list`, `lore query`, `lore refs`, `lore search`, `lore check`.
- **LSP server** (Go): hover, go-to-definition, find-references, autocomplete, diagnostics for undefined references.
- **VSCode extension**: semantic highlighting with stable per-entity colours, inline state directives, definition styling, configurable hover.
- **State tracking**: tags and fields on entities (`+captured`, `hp = 12`) resolved across the document timeline.
- **Relations**: typed, directional links between entities (`father -> Doug`, `members -> Aragorn, Bilbo`), declared once and rendered on both endpoints. See [Relations](#relations) below.

## Install

### CLI

Pre-built binaries are attached to each [release](../../releases). Pick the one for your platform, drop it on your `PATH`, and you're done.

```bash
# macOS Apple Silicon
curl -L -o lore https://github.com/franklin-ross/lore/releases/latest/download/lore-darwin-arm64
chmod +x lore && mv lore ~/bin/

# Linux x86_64
curl -L -o lore https://github.com/franklin-ross/lore/releases/latest/download/lore-linux-amd64
chmod +x lore && mv lore ~/.local/bin/
```

Or build from source:

```bash
git clone https://github.com/franklin-ross/lore
cd lore
task install      # builds + installs CLI and VSCode extension
```

### VSCode Extension

Download `lore.vsix` from the latest [release](../../releases) and install:

```bash
code --install-extension lore.vsix
```

The extension finds the `lore` binary via `lore.serverPath` (falls back to `lore` on `PATH`).

## Quick Start

```bash
mkdir my-campaign && cd my-campaign
lore config init                    # writes lore.toml
echo 'Sildar (character): Fighter. Member of Lords Alliance.' > notes.md
lore list                           # → Sildar (character)
lore query Sildar                   # description + references
lore check                          # report undefined references
```

Open the folder in VSCode and the extension activates on `lore.toml`.

## Relations

A relation is a typed, directional link between two entities, written with `->`
alongside tags and fields. Declare it once; it renders on both endpoints.

```markdown
Sarah (person): father -> Doug
Party (group): members -> Aragorn, Bilbo
Guild (group): Borin storms out. members -/> Borin
```

Multiple targets are comma-separated; relations accumulate across sessions and
retract with `-/>`. An undefined label still works as a generic edge — the
vocabulary is pure upgrade.

**The one rule.** The label names _what the target is to the subject_:

```
A: rel -> B    reads as    "B is A's rel."
```

So `Sarah: father -> Doug` means Doug is Sarah's father, and his card shows the
reciprocal (`child -> Sarah`) automatically. To write it subject-first, every
noun has a synthesised genitive: `Sarah: daughter-of -> Doug` records the same
edge from the other end (`daughter-of` resolves to the reciprocal, `parent`).

**Configuring vocabulary** (`lore.toml`, optional) lets you set reciprocals,
noun aliases, raw aliases (verbs/locatives taken as-is), and irregular plurals:

```toml
[relations.parent]
reciprocal = "child"
aliases = ["father", "mother", "dad", "mum"]

[relations.child]
aliases = ["daughter", "son"]

[relations.possession]
reciprocal = "owner"
aliases = ["belongings", "property"]  # nouns; `<noun>-of` genitive is synthesised
raw_aliases = ["owns"]                # taken as-is (verbs, locatives); no genitive/plural

[plurals]                             # plurals are automatic; pin only what the inflector can't know
drow = "drow"                         # invariant plural (would otherwise be "drows")
```

Four rules keep a custom relation well-behaved:

1. **Reciprocals are one-to-one** — two relations must not share one. Model
   gendered variants (`aunt`/`uncle`) as _aliases of one canonical_, not two.
2. **Reciprocals are mutual** — if `A` reverses `B`, `B` reverses `A`.
3. **Canonicals and aliases are nouns** — they get pluralised for headers and
   suffixed for genitives, so a verb breaks it (`contains` → "containses"). Make
   the canonical a noun (`contents`) and put the verb in `raw_aliases`
   (`contains`, `holds`).
4. **A raw alias sits on the side whose _subject_ performs it** — the opposite
   side from the matching noun. `A: leader -> B` = "B is A's leader", so `serves`
   belongs to `leader` and `leads` to `follower`.

Built-ins cover familial, social, membership, containment, residence, ownership,
hierarchy, mentorship, and authorship relations. The full rundown, with worked
examples, is in [docs/format.md](docs/format.md#relations).

See [docs/contributing.md](docs/contributing.md) for commit conventions and [docs/design.md](docs/design.md) for the architecture. See the release procedure at [RELEASING.md](RELEASING.md).

Public domain — see [LICENSE.md](LICENSE.md).
