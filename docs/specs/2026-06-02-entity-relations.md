# Entity Relations

Status: implemented (engine, `lore query`, hover) · 2026-06-02

## Motivation

People already write relations between entities as either prose (`Sarah
(person): daughter of Doug`) or state (`Party (group): members +=
Aragorn`). Both work — the parser finds the references wherever they sit
— but relations get lost in the sea of ordinary mentions, and reciprocal
relations (`Sarah daughterOf Doug` implies `Doug parentOf Sarah`) have to
be written twice or only show on one side.

This spec adds **typed, directional edges between entities**, declared
once and rendered on both endpoints, in a dedicated Relations section.

This is a deliberate departure from the line in `design.md`: _"No
triples, no predicates, no fact extraction. Just entities and
cross-references."_ Relations are the first typed predicate in Lore. We
keep the scope narrow on purpose (see Non-Goals) so the freeform
character of the format survives.

## The Core Split

Two different things, kept apart:

- **Relation edges** are **world-state**. `Sarah daughterOf Doug` lives
  in prose, on the timeline, and can change over sessions (a friend
  becomes an enemy). Edges behave like the existing state directives.
- **Relation definitions** are **config**. The vocabulary — what
  `parent` means, that it's the reciprocal of `child`, that its aliases
  are `father`/`mother` — is static, lives in `lore.toml`, and stays out
  of your way while you write.

Putting definitions in config (not inline) is intentional: a definition
is set-once vocabulary, not something that accretes on the session
timeline. Mixing the two was rejected during design — see "Rejected:
Relations as Entities".

## The `->` Operator

A new directive, alongside tags and fields, for declaring an edge:

```
Sarah (person): father -> Doug
```

- Left of `->` is a **relation label** (resolved against the relation
  vocabulary).
- Right of `->` is one or more **entity references**.
- Terminates at the first of `.`, `!`, `?`, `;`, or end-of-line, exactly
  like existing state directives.

Multiple targets and accumulation work like list fields:

```
Party (group): members -> Aragorn, Bilbo
Party (group): members -> Frodo            # accumulates: Aragorn, Bilbo, Frodo
```

Because edges are world-state, you remove them as the world changes — a
member leaves a guild, a friend turns enemy:

```
Guild (group): members -/> Borin
```

Removal is reciprocal-aware (see Edge Identity). Either endpoint retracts
the edge with either label:

```
Sarah: father -> Doug          # add
Doug: daughter -/> Sarah       # removes the same edge
```

Removing an edge that was never set is a no-op and emits a soft
diagnostic, mirroring the existing "remove a tag that isn't set" warning.
For a generic (undefined) relation the system has no reciprocal
knowledge, so a remove phrased from the opposite label can't match — it
just warns.

(The `-/>` glyph is provisional; the capability is settled.)

### Reciprocal Rendering

One declaration renders on both endpoints. The reverse side shows the
**incoming** edge with its source named, so no reverse label is required:

```
Sarah: father -> Doug
```

- Sarah's card: `father → Doug`
- Doug's card: `child → Sarah` (when `father`→`parent`, `parent`⁻¹ =
  `child`; see config)

If the relation is undefined (generic edge), Doug's side falls back to
the raw incoming form: `Sarah → father`.

### Edge Identity

An edge is keyed by its **canonical relation and its endpoints' roles**,
not by the label or direction you happened to type. With `father` and
`daughter` both normalising to the reciprocal pair `parent`/`child`, all
four of these name the same edge:

```
Sarah: father -> Doug      Sarah: parent -> Doug
Doug: child -> Sarah       Doug: daughter -> Sarah
```

Add or remove from either side, with either label, and they converge on
one edge. (A generic, undefined relation has no reciprocal, so only the
exact label and direction match — the other phrasings are separate
edges.)

Re-declaring an edge is idempotent — no duplication, exactly like
defining a relation twice across normal entity definitions. When
declarations disagree on an endpoint's surface label (`daughter -> Doug`
then `child -> Doug`), most-recent wins by file order, just like a state
field.

The storage orientation that makes this deterministic is an
implementation detail — see Open Questions.

### The Other Side, By Hand

There is no per-edge reverse override. When the canonical reverse
(`parent`⁻¹ = `child`) isn't the word you want on the far side — a
gendered reverse, say — declare that side yourself:

```
Sarah: father -> Doug
Doug: daughter -> Sarah
```

Both resolve to the same canonical edge, so nothing is duplicated; each
endpoint just keeps the surface label you wrote there. Sarah's card shows
`father → Doug`, Doug's shows `daughter → Sarah`. Declare only one side
and the other falls back to the canonical reciprocal.

Or collapse both declarations onto one line with an **inline aside** —
Lore's existing `(Name: body)` construct (`inline_aside.go`), which
defines or extends another entity in place and renders as just its name
in the host prose:

```
Sarah: father -> (Doug: daughter -> Sarah)
```

The aside's body attributes to Doug, not Sarah — when Sarah's own
directives are parsed the aside range is blanked, and the aside is
re-extracted as a synthetic definition for Doug. So `daughter -> Sarah`
applies to Doug, the aside resolves to `Doug` in the target slot, and
both sides land as before: Sarah's card reads `father → Doug`, Doug's
reads `daughter → Sarah`.

(This needs an aside in *target* position to resolve to its entity, and
the `->` directive to parse inside aside bodies. Both follow from treating
`->` as an ordinary directive and asides as ordinary definitions — listed
in Build Sketch. A bare paren without a `Name: body` header, like `(it
was raining)`, stays prose.)

## Relation Definitions in `lore.toml`

Optional vocabulary that _enriches_ relations you reuse enough to care
about:

```toml
[relations.parent]
reciprocal = "child"        # bidirectional: child gets reciprocal = parent for free
aliases = ["father", "mother", "dad", "mum"]

[relations.child]
plural = "children"
aliases = ["son", "daughter"]

[relations.spouse]
reciprocal = "spouse"       # self-reciprocal = symmetric
aliases = ["husband", "wife"]
```

- **`reciprocal`** — the canonical reverse label. Bidirectional: defining
  it one way implies the other. A relation whose reciprocal is itself is
  symmetric.
- **`aliases`** — surface labels that normalise to this relation.
  Display-preserving: storage and rendering keep the word you typed
  (`father`), the canonical (`parent`) is used only for reciprocity and
  merging — exactly how entity aliases already work.
- **`plural`** — used in merged display headers (`child` → `children`).
  Defaults to `name + "s"`.

Built-in vocabulary for common familial/social/membership relations ships
by default; users extend or override by defining the same name.

### Gender Variants Are Aliases, Not Canonicals

A reciprocal is a **one-to-one back-pointer**, so two canonicals must never
share one. Modelling `aunt` and `uncle` as separate canonicals that both
reciprocate `nibling` breaks edge identity — the shared reciprocal can only
point back to one of them (first registered wins), and `uncle` edges end up
on a different canonical key than their `nibling` reverse, so they neither
dedup nor cancel on removal.

Gender variants are therefore **aliases of a single canonical**, exactly as
`father`/`mother` are aliases of `parent`. Where English lacks a common
neutral word, the built-ins use the linguistic neutrals: `pibling`
(aunt/uncle) reciprocal `nibling` (niece/nephew), each carrying the gendered
aliases. Because display preserves the typed label, an author who writes
`uncle` still sees `uncle`; the neutral canonical only surfaces on an
*undeclared* reverse side — exactly the case where gender is unknown, so a
neutral term is correct rather than a wart.

Built-in familial set: parent/child, sibling, pibling/nibling,
cousin (symmetric), grandparent/grandchild, spouse.

### Validation

Because a shared reciprocal silently corrupts edge identity,
`ValidateRelations` checks the effective definition set (built-ins +
config) for two failures and reports them:

- **Many-to-one reciprocal** — two or more relations reciprocate the same
  relation (`aunt` and `uncle` both → `nibling`). The message names the
  culprits and points at the fix (one canonical with aliases).
- **Non-mutual reciprocal** — `A` reciprocates `B` but `B` reciprocates
  some `C ≠ A`.

Surfaced in both places: `lore check` lists them against `lore.toml`, and
the LSP raises a `window/showMessage` warning toast at project load and
reload (when relation config can change), so a bad `lore.toml` is caught
the moment it's saved rather than producing silent mis-rendering.

### Optional, Not a Gate

An undefined label is **not** an error. It renders as a **generic edge**:
both sides shown, reverse as named-incoming, no alias/plural/custom
reciprocal:

```
Sarah: bestie -> Mary       # bestie undefined → generic edge
```

- Sarah's card: `bestie → Mary`
- Mary's card: `Sarah → bestie`

`->` always works. The config is pure upgrade. Forcing a definition
before you can record a relation would reintroduce exactly the
context-break the format exists to avoid.

## Display Rules

Relations render in a dedicated **Relations** section on each entity, so
they don't drown in ordinary mentions.

Edges are **grouped by canonical relation**, with an **adaptive header**:

- All items share one surface alias → use it, pluralised:

    ```
    daughter -> Sarah, daughter -> Mary    →    daughters → Sarah, Mary
    ```

- Mixed surfaces → canonical header, annotate only the deviations:

    ```
    daughter -> Sarah, child -> Tim        →    children → Sarah (daughter), Tim
    ```

    `Tim` (written `child`, the canonical) gets no annotation; `Sarah`
    (written `daughter`) gets one because it deviates. Noise appears only
    where it carries meaning. The annotation is simply each edge keeping
    the surface label you wrote for it.

The same rule applies to authored and reciprocal sides alike: bucket by
canonical, adapt the header, annotate deviations. The renderer does not
track who authored what.

### Resolution and Hover

Edges are state events, so they resolve exactly like tags and fields,
through the same `ResolveStateAt` cutoff: adds and removes fold in file
order up to and including the cursor position. Hovering an entity shows
its relations **as of that point in the timeline** — the net set after
removals, not every edge ever written. Earlier files fold in full; the
cursor's own file folds up to the hovered line.

Reciprocal (incoming) edges declared on other entities fold by the same
cutoff: if `Sarah: father -> Doug` sits in a file ordered after the hover
point, it doesn't yet show on Doug. One timeline, both directions.

## Namespacing

`->` is also the namespace switch. Disambiguation is by slot:

```
Sarah: father -> Doug
       ^^^^^^    ^^^^
       relation  entity
```

Left of `->` resolves against the relation vocabulary; right of `->`
resolves against entities. An entity named `Son` and a relation alias
`son` never collide because they live in different slots.

## Non-Goals

Deliberately out of scope, with reasons:

- **Transitivity** (`parent` → `grandparent`). Graph inference, not
  vocabulary lookup — surprising results, large complexity. May be
  revisited later; the static-lookup design does not preclude it.
- **Conditional / gender-aware reciprocity** (`daughter` when the child
  is female). Couples relation rendering to entity attributes. Declare
  the far side by hand when you want its exact label (see "The Other
  Side, By Hand").
- **Relations as entities.** Reusing the entity grammar for relation defs
  was considered and rejected — see below.
- **Edge metadata / state** (a description or state on the edge itself).
  Rare in practice. If it ever lands, the path is "the edge becomes an
  entity", not "every edge carries a payload".
- **Inline relation definitions.** Vocabulary stays in `lore.toml`. The
  freeform path is using relations, not defining them.

## Rejected: Relations as Entities

We considered modelling a relation def as a `(relation)`-typed entity
(`child (relation) | daughter | son: reciprocal = parent, plural =
children`), reusing the entity grammar wholesale. Rejected because:

- In the current format, `type` is **value-agnostic** — its only job is
  to distinguish a definition from free text. `(relation)` would be the
  first type whose _value_ carries engine semantics, turning a cosmetic
  discriminator into a reserved keyword.
- Entities are **mutable world-state on a timeline**; relation defs are
  **static config**. Same container, opposite temporality.
- Core entity behaviours — description, state accretion, graph node,
  reference detection — don't apply to a relation def. Each non-applying
  feature is an abstraction leak.

Reusing the _grammar_ was neat and could be pursued later; conflating the
_kind_ was not. The `lore.toml` route keeps definitions cleanly separate
from prose.

## Open Questions

- Edge removal glyph (`-/>` vs alternatives). The capability is settled;
  only the spelling is open.
- Canonical storage orientation for an edge, so that adds and removes
  from either side and either label converge deterministically on one
  edge (see Edge Identity).
- Pluralisation beyond `+ "s"` for built-ins (`person` → `people`):
  per-relation `plural` covers it, but the built-in set needs auditing.
- Whether generic (undefined) edges should emit a soft diagnostic
  ("relation `bestie` is undefined") or stay silent.
- Autocomplete: relation labels in label position should complete from
  the vocabulary; entities in target position from the entity list.

## Build Sketch

1. Parse `[relations.*]` from `lore.toml`; build the vocabulary (canonical
   names, alias map, reciprocal map, plurals); ship built-ins.
2. Parse the `->` / `-/>` directives in descriptions (label, targets);
   accumulate and retract edges in the graph.
3. Resolve labels to canonical (or generic fallback); compute reciprocal
   edges. An inline aside in target position resolves to its entity; a
   `->` inside an aside body attributes to the aside's entity (the
   blanking in `merge.go` already isolates it).
4. Render the Relations section: group by canonical, adaptive header,
   deviation annotations.
5. LSP: highlight `->`, complete labels vs targets by slot, hover the
   relation, go-to-definition on both endpoints.

## Implementation Status

Built (`internal/lore` + wiring):

- `[relations.*]` config parsing and the built-in vocabulary
  (`relation.go`): aliases, bidirectional reciprocity, configurable plural,
  case-insensitive lookup with display-preserving casing.
- `->` / `-/>` directive parsing (`directive.go`), tried before `-=` so the
  arrows aren't mistaken for a field remove.
- Cross-entity edge resolution with canonical identity and the
  ResolveStateAt cursor cutoff (`relation_resolve.go`): either side, either
  label, add or remove converge on one edge; reciprocal/incoming computed.
- Display: grouped by canonical, adaptive header, deviation annotations
  (`relation_display.go`). Shared-surface pluralisation uses a trailing "s"
  unless the alias already ends in one (so "members" doesn't become
  "memberss"); only the canonical carries a configurable plural.
- Wired into `lore query` and LSP hover (relations resolve at the cursor,
  same timeline as state).

Deferred (separable follow-ups, none load-bearing for the engine):

- LSP editor features for relation labels: semantic highlighting,
  slot-aware completion (labels vs targets), go-to-definition on endpoints.
- Inline aside in *target* position (`father -> (Doug: daughter -> Sarah)`).
  The aside mechanism exists; resolving an aside to its entity in the
  target slot is not yet wired, so use two blocks for a custom far side.
- The optional "relation `bestie` is undefined" soft diagnostic.

Done since first cut:

- Project `[relations.*]` config now threads into the LSP world, so hover
  and relation resolution use the full config-overlaid vocabulary, matching
  `lore query`.
- Vocabulary validation (reciprocity conflicts) surfaced via `lore check`
  and an LSP warning toast.
