# Memory Scopes

Status: proposed feature design

## Summary

Scopes define where a memory is true.

`thr` is used primarily by AI agents. An agent must be able to determine, without
guessing:

1. Which context it is operating in.
2. Where a new memory will be stored.
3. Which scopes a recall operation will search.
4. Which scope each returned memory came from.
5. How to correct a memory stored in the wrong scope.

The first version of scopes has two scope kinds:

| Scope | Meaning | Example |
| --- | --- | --- |
| `user` | True across all work in the selected `thr` database | The user prefers concise explanations. |
| `repo` | True only for one logical Git repository | This repository uses pnpm. |

Every memory belongs to exactly one scope. There are no unscoped memories.

Inside a recognized repository, an unqualified write goes to that repository
and an unqualified recall searches the repository and user scopes. Outside a
repository, recall searches the user scope, but an unqualified write fails and
requires the caller to explicitly select `user`.

This design makes the narrow, safer choice convenient while preventing an agent
from accidentally turning repository-specific information into a user-wide
instruction.

## Product Goal

Scopes are an applicability boundary, not a folder system:

> A scope answers "where is this memory true?"

The feature should improve recall precision without requiring an agent to
manually manage scope state before every operation. Automatic context should
handle the common case, while explicit selectors and complete output metadata
make every decision inspectable.

The desired agent workflow is:

```text
resolve context -> recall visible memories -> perform work -> store in the
least-broad scope that covers every context where the fact is explicitly
intended to apply
```

## Goals

- Prevent memories from unrelated repositories from contaminating recall.
- Make repository-specific writes low friction.
- Make broad writes deliberate.
- Keep scope resolution deterministic and visible.
- Let agents inspect and operate in another repository without changing the
  process working directory.
- Keep human-readable commands concise.
- Provide a stable machine contract for context and scope metadata.
- Preserve current memories and IDs during migration.
- Support future `project` scopes without requiring them in the first release.

## Non-goals

- Scopes are not access control or encryption boundaries.
- Scopes do not make secrets safe to store.
- Scopes do not synchronize memories between machines or users.
- Scopes do not infer whether a statement is true.
- Scopes do not automatically resolve contradictory memories.
- Scopes do not replace tags. Tags describe a subject; scopes define
  applicability.
- Branches, worktrees, directories, and agent sessions are not durable scope
  kinds in the first release.
- Arbitrary nested scope trees are not supported.

## Core Invariants

The following rules are part of the product contract:

1. Every memory has one non-null scope ID.
2. A memory cannot belong to multiple scopes.
3. Memory IDs remain unique across the entire selected database.
4. A write resolves to one exact scope before any text or embedding is stored.
5. Failure to resolve a write scope never falls back to a broader scope.
6. A default recall searches only the scopes visible from the current context.
7. Other repository scopes are never searched implicitly.
8. Explicit scope selectors mean exact scopes; they do not imply ancestors.
9. Every human and JSON v2 memory result identifies its scope.
10. Every JSON v2 recall response identifies all scopes searched, including an
    empty response.
11. Moving a memory between scopes is explicit and atomic.
12. Scope filtering happens before candidate limits and ranking.
13. Read-only context detection does not create a scope or database.
14. Concurrent first writes for the same repository converge on one scope.
15. After scope migration, older binaries fail closed instead of reading or
    writing scoped data without filters.

These invariants prevent the most difficult ambiguous state: a stored memory
whose applicability cannot be determined.

## Why Every Memory Must Have a Scope

An optional scope creates an undefined third category:

```text
repo-scoped
user-scoped
not scoped
```

Every query would then need a policy for unscoped memories:

- Include them in every scope, which recreates cross-repository contamination.
- Exclude them, which makes valid memories appear to disappear.
- Include them only in all-scope queries, which turns them into a migration
  backlog rather than usable memory.
- Guess their scope from their text, which is unreliable.

None of these policies is intuitive for an agent. A non-null scope is therefore
required at the storage boundary. If `thr` cannot determine a valid scope for a
write, it returns an error before creating the memory or embedding.

## Scope Model

### User Scope

Each selected database has exactly one `user` scope. It contains memories that
are useful across repositories, such as:

- Communication preferences.
- Stable personal workflows.
- Cross-project tool preferences.
- Instructions explicitly stated to apply broadly.

`user` means "visible from every context in this `thr` database." It does not
mean public, shared, operating-system global, or synchronized.

The selected database remains the outer isolation boundary:

```bash
thr --db ~/.thr/client-a.db ...
thr --db ~/.thr/personal.db ...
```

Those databases contain separate user and repository scopes. `--db` is not a
scope selector.

### Repository Scope

A `repo` scope represents one logical Git repository. It is not a branch,
worktree, checkout directory, or repository basename.

Examples of repository memories include:

- Build and test commands.
- Architecture decisions.
- Repository-specific conventions.
- Release procedures.
- Known constraints that remain relevant across branches and worktrees.

A repository scope has an immutable internal ID and a mutable display label:

```text
id:    repo:01KABCDEF123
label: github.com/acme/payments
```

Agents use symbolic selectors for the current repository and stable IDs for
other repositories. Labels are for recognition and may change; they are not
identity.

### Future Project Scope

A later release may add an explicit `project` scope for a product composed of
multiple repositories:

```text
user
  project:payments-platform
    repo:payments-api
    repo:payments-web
```

In that model, repository recall would search `repo + project + user`.

Projects are intentionally deferred. Inferring projects from a parent folder,
Git hosting organization, or repository name would create surprising context
sharing. Project membership must be explicit, and a repository should have at
most one project parent until a more complex visibility model is justified.

## Visibility Model

Scope visibility flows from broad to narrow:

```text
user memories are visible inside repositories
repository memories are not visible outside that repository
repository memories are not visible to sibling repositories
```

This is query visibility, not copying or inheritance. A user memory remains one
row in the user scope when recalled from a repository.

### Default Scope Behavior

| Context | Default write | Default recall | Default list |
| --- | --- | --- | --- |
| Recognized repository | Current `repo` | Current `repo` + `user` | Current `repo` + `user` |
| Prospective repository | New `repo` created by first write | `user` until the repository scope exists | `user` until the repository scope exists |
| Outside a repository | No implicit scope | `user` | `user` |
| Ambiguous repository | Error | Error | Error |
| Explicit `--scope` | Exact selected scope | Exact selected scope or scopes | Exact selected scope or scopes |

The default list follows the same visible set as recall so an agent does not
need to learn a second implicit visibility model. Every list row includes its
scope.

Administrative operations such as all-scope statistics and indexing can inspect
the full database, but they must report per-scope information.

### Exact Selectors

The initial selectors are:

| Selector | Meaning |
| --- | --- |
| `user` | The exact user scope in the selected database |
| `repo` | The exact repository resolved from `--cwd` or the current directory |
| `repo:<id>` | A specific registered repository scope |

No selector means automatic visible scopes. An explicit selector means exactly
that scope, not its ancestors.

```bash
thr ask "release process"                 # current repo + user
thr ask --scope repo "release process"    # current repo only
thr ask --scope user "response style"     # user only
thr ask --scope repo:01KABC "deployment"  # one other repo
```

Repeated selectors form an explicit union:

```bash
thr ask \
  --scope repo:01KABC \
  --scope repo:01KXYZ \
  "shared release process"
```

`--all-scopes` searches every registered scope and is mutually exclusive with
`--scope`. It is intended for administration and exceptional cross-project
questions, not normal agent recall.

### Command and Selector Matrix

| Command | No selector | Explicit selector support |
| --- | --- | --- |
| `add` | Current repository; unresolved outside a repository | Exactly one `--scope`; no `--all-scopes` |
| `ask`, `search`, `list` | Visible scopes for the current context | Repeated `--scope` union or `--all-scopes` |
| `show`, `edit`, `forget` | Exact global memory ID | No retrieval selector; future `--if-scope` is a precondition only |
| `move` | Invalid without destination | Exactly one `--to`; no `--all-scopes` |
| `stats`, `index` | All registered scopes, preserving current database-wide behavior | Repeated `--scope` union or `--all-scopes` |
| `context` | Resolve from `--cwd` or current directory | No scope selector |
| `scope list` | All registered scopes | Management filters may be added later |
| `migrate` | Explicitly upgrades an older database after creating a verified backup | Scope-independent; scope flags are rejected |
| `prefetch`, `setup`, `version` | Scope-independent | Scope flags are rejected |

`stats` against a missing database succeeds with zero persisted scopes and
reports the logical user selector. `index` against a missing database retains
the current `no memories stored` behavior and creates nothing. Exact ID commands
bypass context resolution because the ID is already an unambiguous target.

For a prospective repository, unqualified `add`, `add --scope repo`, and
`move --to repo` create and bind the repository scope transactionally. Exact
read or administrative selectors such as `ask --scope repo`, `list --scope
repo`, `stats --scope repo`, `index --scope repo`, and `scope show repo` return
`repository_scope_unpersisted` with a suggestion to use automatic user recall,
store the first repository memory, or run `scope create repo`. They do not
create an empty scope as a read side effect.

For `matched_unbound`, the symbolic `repo` selector resolves to the existing
scope and reads may use it immediately. The first successful write persists the
checkout binding in the same transaction as the memory operation.

## Write Experience

### Repository Fact

Inside a recognized repository:

```bash
thr add "Integration tests require Docker"
```

The narrow default is the current repository:

```text
stored memory 42 in [repo:payments/01KABC]
```

The equivalent explicit command is:

```bash
thr add --scope repo "Integration tests require Docker"
```

### User-wide Fact

Broad writes are explicit even when the current directory is a repository:

```bash
thr add --scope user "The user prefers concise explanations"
```

An agent should apply this decision rule:

> Store a memory in the least-broad scope that covers every context where the
> statement is explicitly intended to apply.

If the agent cannot determine whether a statement is repository-specific or
user-wide, it should use the repository scope or ask the user. It must not
silently broaden the scope based on semantic interpretation.

### Write Outside a Repository

An unqualified write outside a repository fails:

```bash
thr add "Use pnpm"
```

```text
cannot determine a default write scope outside a repository; use --scope user
```

The caller can make the broad intent explicit:

```bash
thr add --scope user "The user prefers pnpm"
```

This friction is intentional. Accidentally storing a memory too narrowly can be
corrected with a move. Accidentally storing a repository fact in `user` can
mislead every future agent session.

### Failed Resolution

Input validation and scope resolution happen before model initialization,
database creation, or embedding. An invalid or ambiguous scope must have no
storage or model-cache side effects.

## Recall Experience

Inside a recognized repository:

```bash
thr ask "How do I run integration tests?"
thr search "Docker tests"
```

Both commands search the current repository and user scope. Results identify
their source:

```text
42  [repo:payments/01KABC]  Integration tests require Docker
17  [user]           The user prefers commands with concise output
```

An agent can restrict the search when the distinction matters:

```bash
thr ask --scope repo "How do I run integration tests?"
thr search --scope user "preferred output"
```

No result is still a successful operation. Machine output must state which
scopes were searched so the agent can distinguish "no matching memory" from
"the repository was not searched."

## Context Introspection

`thr context` explains the active scope model without changing it:

```bash
thr context
thr --format json-v2 context
thr --cwd /work/payments --format json-v2 context
```

It reports:

- The selected database path and whether it exists and is compatible.
- The context directory.
- The detected Git root.
- The current repository scope, if any.
- The default write scope.
- The default read scopes.
- How the repository was identified.
- Identity drift or ambiguity warnings.

Example:

```json
{
  "api_version": "thr.cli/v2",
  "ok": true,
  "command": "context.show",
  "context": {
    "database": {
      "path": "/home/agent/.thr/thr.db",
      "status": "compatible"
    },
    "cwd": "/work/payments",
    "current_scope": {
      "id": "repo:01KABC",
      "kind": "repo",
      "label": "github.com/acme/payments",
      "status": "persisted"
    },
    "default_write_scope": {
      "id": "repo:01KABC",
      "kind": "repo",
      "label": "github.com/acme/payments",
      "status": "persisted"
    },
    "default_read_scopes": [
      {
        "id": "repo:01KABC",
        "kind": "repo",
        "label": "github.com/acme/payments",
        "status": "persisted"
      },
      {
        "id": "user",
        "kind": "user",
        "label": "user",
        "status": "persisted"
      }
    ],
    "resolution": {
      "source": "git_common_dir",
      "status": "bound"
    }
  },
  "result": {},
  "error": null,
  "warnings": []
}
```

`thr context` is read-only. It does not create the database, a repository
scope, or a checkout binding. It uses a non-mutating compatibility inspection
path that does not run migrations, change journal mode, update usage metadata,
or prepare the embedding model.

An unseen repository has no stable scope ID until the first write commits. In
that case, `context` reports a prospective destination instead of pretending a
scope already exists:

```json
{
  "api_version": "thr.cli/v2",
  "ok": true,
  "command": "context.show",
  "context": {
    "database": {
      "path": "/home/agent/.thr/thr.db",
      "status": "missing"
    },
    "cwd": "/work/new-repo",
    "current_scope": null,
    "prospective_scope": {
      "id": null,
      "kind": "repo",
      "label": "github.com/acme/new-repo",
      "status": "prospective"
    },
    "default_write_scope": {
      "id": null,
      "status": "prospective",
      "kind": "repo",
      "label": "github.com/acme/new-repo"
    },
    "default_read_scopes": [
      {
        "id": "user",
        "kind": "user",
        "label": "user",
        "status": "logical"
      }
    ],
    "resolution": {
      "source": "canonical_origin",
      "status": "prospective"
    }
  },
  "result": {},
  "error": null,
  "warnings": []
}
```

Database and repository resolution use separate state enums:

| `database.status` | Meaning |
| --- | --- |
| `missing` | The selected database does not exist; context remains read-only. |
| `compatible` | The database can be inspected by this binary without mutation. |
| `migration_required` | The database requires an explicit backed-up upgrade. |
| `incompatible` | The binary cannot safely read this database format. |

| `resolution.status` | Meaning |
| --- | --- |
| `bound` | The checkout is bound to a persisted repository scope. |
| `matched_unbound` | The checkout uniquely matches an existing scope but has not been persisted as a binding. Reads may use that scope. |
| `prospective` | A repository is recognized but has no persisted scope yet. |
| `outside_repository` | No repository context exists; only user recall is visible. |
| `ambiguous` | More than one persisted scope could match; automatic operations fail. |
| `unavailable` | Git could not safely resolve the requested context. |

The singleton `user` selector is a logical destination even before the database
exists. A prospective repository receives its immutable ID only inside the
transaction that stores its first memory.

Context scope descriptors always contain `id`, `kind`, `label`, and `status`.
`id` is null only for a prospective scope. `status` is `persisted`, `logical`,
or `prospective`; the object shape does not change between context states.

### Context Override

`--cwd` is a global context flag:

```bash
thr --cwd /work/another-repo --format json-v2 ask "release process"
```

It changes repository detection without changing the process directory or the
selected database. This avoids shell state and makes multi-repository agent
workflows deterministic.

An explicit `--scope repo:<id>` does not require `--cwd`. Explicit scope always
wins over automatic context.

## Repository Resolution

Repository identity is the most important source of scope correctness. A path
alone is too fragile, while a remote URL alone is too easy to change or
misinterpret.

### Identity Data

A repository scope records:

- An immutable opaque ID.
- A mutable human-readable label.
- One or more local checkout bindings.
- The Git common directory for each binding.
- Canonical remote aliases when available.
- The source and time of identity observations.
- Drift and ambiguity status.

Credentials must be removed from remotes before storage or display.

### Resolution Order

Repository resolution follows this order:

1. Honor an explicit scope ID.
2. Ask Git to resolve the repository for `--cwd` or the current directory.
3. Resolve the worktree root and Git common directory.
4. Reuse an existing binding for that common directory.
5. Otherwise inspect the configured identity remote, then `origin`, then a sole
   fetch remote.
6. Canonicalize the remote conservatively.
7. Match an existing repository scope only when exactly one scope matches. A
   read reports `matched_unbound` and does not persist the checkout binding.
8. If no scope matches, represent a new repository context and persist it only
   on the first write or an explicit create-and-bind operation.
9. If multiple scopes match, fail with an ambiguity error and suggested
   management commands.

Once a checkout is bound, later remote changes do not silently switch its
scope. `thr` preserves the binding and reports identity drift.

First-write registration is transactional. It acquires a database write lock,
re-evaluates bindings and remote aliases, reuses a unique matched scope or
creates one only if no unique match appeared, and stores the binding and memory
in the same transaction. A Git common directory can be owned by only one active
scope. Remote aliases are not globally unique because an explicit split may
intentionally produce two scopes for the same remote.

### Remote Canonicalization

Canonicalization may:

- Remove credentials.
- Lowercase DNS hostnames.
- Remove a trailing `.git` where the hosting provider treats it as equivalent.
- Normalize SSH and HTTPS forms for known providers.
- Normalize scp-style SSH syntax for known providers.

Canonicalization must preserve non-default ports and repository path case unless
the provider guarantees case-insensitive identity. No network request is needed
for detection. Each stored canonical alias records the normalization algorithm
version. A future normalization change must re-evaluate aliases explicitly and
must not silently merge scopes.

### Repository Edge Cases

| Case | Behavior |
| --- | --- |
| Linked Git worktrees | Share one repository scope through the Git common directory. |
| Detached HEAD | Uses the same repository scope; branch is irrelevant. |
| Nested repository | The innermost Git repository wins. |
| Submodule | Has its own repository scope. |
| Independent clone with the same canonical `origin` | Reuses an existing scope only when the match is unique. |
| Fork with `origin` set to the fork | Uses a separate scope; `upstream` is not automatic identity. |
| Repository remote changes | Keeps its binding and reports drift. |
| Repository has no remote | Uses a local binding and can be linked explicitly later. |
| Multiple remotes without a clear identity | Fails closed instead of guessing. |
| Repository moves on disk | Reuses the scope if identity evidence is sufficient; otherwise requires an explicit bind. |
| Symlinked working directory | Uses a canonical path internally and the requested path for display. |
| Bare repository | May have a repository scope even without a worktree. |
| Git reports unsafe ownership | Returns a context error; never falls back to `user`. |
| Same remote intentionally needs isolated memory | Requires an explicit scope split. |

Branches are not scopes. Worktrees are not scopes. Making either durable would
fragment repository knowledge around temporary implementation state.

## Scope Management

Agents and humans can inspect registered scopes:

```bash
thr scope list
thr --format json-v2 scope list
thr scope show repo
thr scope show repo:01KABC
```

Scope output includes:

- Stable ID.
- Kind.
- Label.
- Known aliases.
- Bound checkouts.
- Memory count.
- Latest memory update time.
- Whether it is current.
- Identity warnings.

Example:

```json
{
  "api_version": "thr.cli/v2",
  "ok": true,
  "command": "scope.list",
  "context": {
    "database": {
      "path": "/home/agent/.thr/thr.db",
      "status": "compatible"
    },
    "cwd": "/work/payments",
    "scope_selection": {
      "mode": "all",
      "requested": [],
      "resolved": ["repo:01KABC", "user"]
    }
  },
  "result": {
    "scopes": [
      {
        "id": "repo:01KABC",
        "kind": "repo",
        "label": "github.com/acme/payments",
        "memory_count": 28,
        "current": true,
        "warnings": []
      },
      {
        "id": "user",
        "kind": "user",
        "label": "user",
        "memory_count": 11,
        "current": false,
        "warnings": []
      }
    ]
  },
  "error": null,
  "warnings": []
}
```

Management operations needed for repository edge cases include:

```bash
thr scope create repo --cwd /work/payments
thr scope bind repo:01KABC --cwd /work/payments
thr scope unbind --cwd /work/old-checkout
thr scope rebind repo:01KABC --cwd /work/payments
thr scope rename repo:01KABC payments
thr scope split --cwd /work/isolated-payments
```

These are exceptional operations. The normal agent workflow should not need
them.

Their postconditions are explicit:

| Operation | Behavior |
| --- | --- |
| `create repo` | Creates and binds a scope for a prospective repository. If its remote uniquely matches an existing scope, creation fails and suggests `bind`; creating an intentional duplicate requires `split`. |
| `bind` | Adds an unbound Git common directory to the named scope. It fails if another scope owns the binding. It never moves memories. |
| `rebind` | Atomically transfers an existing common-directory binding to the named scope after reporting both scopes. It never moves memories. |
| `unbind` | Removes the common-directory binding only. It does not delete the scope, memories, or remote aliases. For linked worktrees, the command affects every worktree sharing that common directory. |
| `rename` | Changes only the display label. Scope identity and bindings remain unchanged. |
| `split` | Creates a new empty scope and rebinds the selected common directory, including all linked worktrees that share it. It copies no memories. Remote evidence is retained but marked intentionally non-unique, so future clones may require an explicit choice. |

All operations are atomic, return old and new binding state, and refuse an
ambiguous target. No scope-management command moves or copies memories
implicitly. An `unbind` that would leave a local-only scope without any binding
is allowed only after an explicit warning or machine-readable confirmation flag;
the scope remains discoverable through `scope list`.

Remote evidence belongs to checkout bindings and records its normalization
version. `create` and `bind` attach the checkout's observed evidence to the
target scope. `rebind` moves the binding and its active evidence to the target;
the source retains only non-matching historical audit data. `unbind` makes that
binding's evidence historical and ineligible for future automatic matching.
`split` moves the binding and active evidence to the new scope and marks the
same remote intentionally non-unique wherever another active binding still
uses it. Future clone resolution then fails as ambiguous until a scope is
selected explicitly.

Scope deletion is not part of the first release. An abandoned repository scope
can remain registered without participating in default recall. Removing a scope
requires a deliberate future policy for its memories and bindings; it must never
orphan memory rows.

## Memory Operations

### Add

`add` resolves one destination scope and returns it:

```bash
thr add "Uses PostgreSQL"
thr add --scope repo "Uses PostgreSQL"
thr add --scope user "The user prefers PostgreSQL"
```

### Show

Memory IDs are globally unique in the selected database:

```bash
thr show 42
```

An exact ID remains addressable when the working directory changes. `show`
always returns the memory's scope. Ambient scope affects discovery, not exact
identity.

### Edit

Editing changes text but preserves scope:

```bash
thr edit 42 "Uses PostgreSQL 18"
```

`edit --scope` is not supported because it would hide a scope change inside a
content operation.

### Move

Moving is explicit:

```bash
thr move 42 --to user
thr move 42 --to repo
thr move 42 --to repo:01KXYZ
```

A move:

- Preserves memory ID, text, and creation time.
- Preserves the content `updated_at` timestamp so a scope change does not create
  an artificial relevance boost.
- Changes the scope atomically.
- Does not require re-embedding because the text did not change.
- Updates vector scope metadata atomically with the memory row.
- Records a separate `scope_updated_at` timestamp.
- Reports the source and destination scopes.
- Is an idempotent success when source and destination are the same.

Moving to a broader scope is never automatic. A future `promote` command may be
an alias for `move --to`, but promotion does not have separate semantics.

### Forget

`forget` deletes the exact memory ID regardless of ambient context and reports
its former scope. The agent skill should continue to require `show` before a
destructive operation when the exact content matters.

Later, optional `--if-scope` and revision preconditions can protect concurrent
agent operations:

```bash
thr edit 42 --if-scope repo:01KABC --if-revision 3 ...
thr forget 42 --if-revision 3
```

## Retrieval and Ranking

Scope filtering must happen before retrieval limits are applied.

Incorrect:

```text
search all memories -> take top N -> remove memories from ineligible scopes
```

That approach lets unrelated repositories consume candidate slots, underfill
results, and potentially leak cross-scope relevance.

Correct:

```text
resolve eligible scopes -> retrieve within those scopes -> merge candidates ->
rank -> remove repeated candidate IDs -> apply final limit
```

### Semantic Recall

Semantic KNN retrieval must constrain candidates by scope inside the vector
query or query each eligible scope independently before merging. Filtering a
global top-K after retrieval is not correct.

Every vector row has filterable scope eligibility metadata that must match its
memory row. Add, move, forget, and migration update both representations in one
transaction. Index health treats missing or mismatched scope metadata as an
invalid embedding and repairs it before that vector participates in recall.

The final ranking order is:

1. Distance rounded to six decimal places, ascending.
2. Scope specificity, with repository before user.
3. Raw distance, ascending.
4. Updated time, descending.
5. Memory ID, ascending, for deterministic ordering.

A highly relevant user memory should outrank a weak repository memory. Scope
specificity applies only after the fixed six-decimal relevance comparison, not
as an arbitrary score boost.

### Text Recall

FTS candidates, recent substring candidates, and fuzzy candidates all receive
scope constraints before their respective candidate limits. The bounded recent
window must be evaluated within eligible scopes, not across the database.

### Duplicate Text

The scope feature does not remove existing duplicate rows or prohibit future
duplicates. Storage deduplication and keyed memories are separate features.

The first scope release does not collapse duplicate records during `ask`,
`search`, or `list`. Returning both IDs and scopes is more transparent than
hiding one record with an implicit winner policy. Duplicate suppression can be
added later with explicit metadata listing every collapsed record.

Similar but non-identical memories remain separate. `thr` must not use embedding
distance to decide that memories are duplicates.

### Contradictions

Unstructured memories cannot safely express override relationships. If the user
scope says "Use npm" and the repository scope says "This repository uses pnpm,"
both may be relevant and both should be returned with scope metadata.

Narrower scope does not automatically suppress broader text, including exact
duplicates. Future keyed memories can add deterministic override semantics, but
scopes alone must not guess.

### Index Health

`ask` checks index health only for the scopes included in that query. A stale
embedding in an unrelated repository must not block current repository recall.

Administrative commands can operate on exact or all scopes:

```bash
thr stats
thr stats --scope repo
thr stats --all-scopes
thr index --scope repo
thr index --all-scopes
```

Statistics include per-scope memory, indexed, stale, and missing counts.

## Machine Interface

Scope-aware commands need a versioned JSON envelope. Bare result arrays cannot
explain context or searched scopes when there are no matches.

Every JSON response includes:

- API version.
- Command or operation name.
- Result or structured error.
- Selected database path and compatibility state.
- Resolved context.
- Requested and resolved scopes for scope-sensitive memory operations.
- Warnings.

The base `context` object always contains `database` and `cwd`. Scope-sensitive
commands also contain `scope_selection` with one stable shape. Command-specific
context fields may be added, but a field never changes type between bound,
prospective, successful, or empty states. Scope collections always contain scope
objects; selector strings are used only inside `scope_selection.requested` and
`scope_selection.resolved`.

Example recall response:

```json
{
  "api_version": "thr.cli/v2",
  "ok": true,
  "command": "memory.ask",
  "context": {
    "database": {
      "path": "/home/agent/.thr/thr.db",
      "status": "compatible"
    },
    "cwd": "/work/payments",
    "scope_selection": {
      "mode": "automatic_read",
      "requested": [],
      "resolved": ["repo:01KABC", "user"]
    }
  },
  "result": {
    "query": "How do I run integration tests?",
    "matches": [
      {
        "rank": 1,
        "memory": {
          "id": "42",
          "text": "Integration tests require Docker",
          "scope": {
            "id": "repo:01KABC",
            "kind": "repo",
            "label": "github.com/acme/payments"
          },
          "scope_assignment": "automatic_context",
          "created_at": "2026-07-17T12:00:00Z",
          "updated_at": "2026-07-17T12:00:00Z"
        },
        "match": {
          "kind": "semantic",
          "distance": 0.184322
        }
      }
    ]
  },
  "error": null,
  "warnings": []
}
```

Example empty response:

```json
{
  "api_version": "thr.cli/v2",
  "ok": true,
  "command": "memory.ask",
  "context": {
    "database": {
      "path": "/home/agent/.thr/thr.db",
      "status": "compatible"
    },
    "cwd": "/work/payments",
    "scope_selection": {
      "mode": "automatic_read",
      "requested": [],
      "resolved": ["repo:01KABC", "user"]
    }
  },
  "result": {
    "query": "deployment freeze",
    "matches": []
  },
  "error": null,
  "warnings": []
}
```

Example write response:

```json
{
  "api_version": "thr.cli/v2",
  "ok": true,
  "command": "memory.add",
  "context": {
    "database": {
      "path": "/home/agent/.thr/thr.db",
      "status": "compatible"
    },
    "cwd": "/work/payments",
    "scope_selection": {
      "mode": "automatic_write",
      "requested": [],
      "resolved": ["repo:01KABC"]
    }
  },
  "result": {
    "memory": {
      "id": "42",
      "text": "Integration tests require Docker",
      "scope": {
        "id": "repo:01KABC",
        "kind": "repo",
        "label": "github.com/acme/payments"
      },
      "scope_assignment": "automatic_context",
      "created_at": "2026-07-17T12:00:00Z",
      "updated_at": "2026-07-17T12:00:00Z"
    }
  },
  "error": null,
  "warnings": []
}
```

Machine-output requirements:

- Exactly one JSON document per command.
- Empty collections are `[]`, not `null`.
- IDs are opaque strings in JSON.
- Timestamps are UTC RFC 3339.
- No prompts, progress bars, or prose mixed into JSON.
- Errors have stable codes and actionable details.
- Scope IDs are stable; labels may change.
- New optional fields may be added within an API version, but existing fields
  do not change type or meaning.

The current bare JSON format is advertised as stable. Replacing it silently in
a patch release would violate that expectation. The scope-aware envelope is
therefore selected explicitly with `--format json-v2`, and the bundled agent
skill uses that format. Existing `--json` output remains available as
`legacy-json` during a documented transition window. A future announced
breaking release may make JSON v2 the default and retain `--format legacy-json`
for a bounded compatibility period.

Legacy JSON remains scope-filtered using the same automatic or explicit scope
selection, but preserves its current bare arrays and objects. It is explicitly
exempt from the v2 guarantees for searched-scope metadata, structured warnings,
and empty-result context. It is not suitable for new agent integrations.
`--json` and `--format` are mutually exclusive. Legacy mode is deprecated in the
scope release and will not be removed before v1.0 and at least one release that
documents the deprecation without adding output to legacy JSON streams.

Success writes exactly one JSON document to stdout, writes nothing to stderr,
and exits with status zero. Failure writes nothing to stdout, writes exactly one
JSON error envelope to stderr, and exits nonzero. No command writes usage text,
progress output, or model-preparation messages into either JSON stream.

Warnings are structured objects rather than strings:

```json
{
  "code": "legacy_scope_assignment",
  "message": "One returned memory has a legacy user-scope assignment.",
  "details": {
    "memory_ids": ["17"]
  }
}
```

Warning codes and detail field types are stable within the API version. Initial
codes include `legacy_scope_assignment`, `repository_identity_drift`, and
`repository_identity_ambiguous`, and `managed_skill_outdated`.

### Structured Errors

Relevant error codes include:

| Code | Meaning |
| --- | --- |
| `write_scope_unresolved` | A write has no safe implicit destination. |
| `repository_ambiguous` | More than one scope could represent the current repository. |
| `repository_unavailable` | Git could not safely resolve the requested context. |
| `repository_scope_unpersisted` | `repo` was selected for a prospective repository that has no stored scope yet. |
| `scope_not_found` | An explicit scope ID does not exist. |
| `scope_selector_invalid` | A scope selector is malformed or unsupported. |
| `scope_conflict` | A conditional operation found a different scope. |
| `memory_not_found` | The exact memory ID does not exist. |
| `index_stale` | An eligible scope needs semantic reindexing. |
| `database_migration_required` | Read-only inspection found a database that needs an explicit upgrade. |
| `database_version_incompatible` | The binary cannot safely use this scoped database format. |

Errors should include a suggested next command when one deterministic action can
resolve the problem.

`retryable` means the unchanged request may succeed later, such as after a
transient database lock. It is false when the caller must change the command or
arguments.

Example error:

```json
{
  "api_version": "thr.cli/v2",
  "ok": false,
  "command": "memory.add",
  "context": {
    "database": {
      "path": "/home/agent/.thr/thr.db",
      "status": "compatible"
    },
    "cwd": "/tmp",
    "scope_selection": {
      "mode": "automatic_write",
      "requested": [],
      "resolved": []
    }
  },
  "result": null,
  "error": {
    "code": "write_scope_unresolved",
    "message": "A write outside a repository requires an explicit scope.",
    "retryable": false,
    "suggested_command": "thr add --scope user <text>"
  },
  "warnings": []
}
```

## Human Experience

Human output uses short scope markers:

```text
[user]
[repo:payments/01KABC]
```

Repository markers include a stable, unambiguous short scope ID because labels
can change or collide. The displayed prefix starts at six characters and extends
until it is unique among scopes in the selected database. `scope show` returns
the complete ID. Absolute paths and remote identity evidence need not appear in
every recall result. Labels, remotes, paths, warnings, and memory text receive
the same terminal-control sanitization as current human memory output.

The common workflow remains short:

```bash
thr add "This repository uses pnpm"
thr ask "Which package manager should I use?"
thr list
```

Broad storage is explicit but understandable:

```bash
thr add --scope user "I prefer pnpm when a project has no convention"
```

Humans should receive the same deterministic behavior as agents. Interactive
prompts are not required for normal scope resolution and must never appear in
JSON mode.

## Agent Guidance

The bundled skill should teach four rules:

1. Use default recall in the current repository; it already includes user
   memories.
2. Store in the least-broad scope covering every context where the statement is
   explicitly intended to apply.
3. Use `--scope user` only for facts and preferences that apply across
   repositories.
4. Run `thr --format json-v2 context` when repository context is unclear or
   when operating outside the current working directory.

Examples:

| Memory | Action |
| --- | --- |
| This repository uses pnpm. | Store in `repo`. |
| Integration tests require Docker here. | Store in `repo`. |
| The user prefers concise explanations. | Store in `user`. |
| The user wants tests run before every commit. | Store in `user` if explicitly broad. |
| This temporary branch is failing CI. | Do not store as durable memory. |
| Several repositories share this process. | Keep separate until an explicit `project` scope exists. |

Agents should not infer broad scope from wording alone. User intent and current
task context remain authoritative.

### Managed Skill Upgrades

The scope release changes write defaults and the preferred machine format, so an
older installed skill is behaviorally stale even when the binary is current.

The release installer automatically updates existing skill files carrying a
recognized `thr` managed marker. It does not require the optional new-skill
prompt for an update and never replaces unmanaged files. Direct binary users can
update each target with the existing setup commands:

```bash
thr setup claude-code
thr setup opencode
thr setup codex
```

`thr migrate`, `thr context`, and `thr stats` inspect known managed skill paths
without modifying them. If a stale managed version is present, JSON v2 emits a
`managed_skill_outdated` warning with the exact setup command for each target.
Human output shows the same corrective commands. Unmanaged files are reported
only as informational and are never rewritten automatically.

## Migration

Scope migration is an explicit format upgrade rather than an automatic side
effect of `list`, `ask`, or another routine command. An older compatible
database reports `database_migration_required` and suggests:

```bash
thr migrate
```

`thr migrate` acquires an exclusive database lock, creates a private timestamped
backup beside the database, verifies that the backup opens and contains the
expected memory and embedding row counts, and only then mutates the original.
If backup creation or verification fails, migration makes no format change. JSON
v2 migration output includes the backup path, old and new format versions, and
validated row counts. A new empty installation creates the scoped format
directly and does not need this command.

Existing memories have no recorded scope but currently participate in recall
from every directory. Migration assigns them to `user` because that preserves
existing visibility.

Migration preserves:

- Memory IDs.
- Text.
- Creation and update timestamps.
- Existing embeddings and model identity.
- Existing duplicate rows.

Migration does not infer repository scope from:

- The process working directory during upgrade.
- The database path.
- Repository names mentioned in memory text.
- Semantic similarity.

Legacy assignment provenance is mandatory and persistent. Migrated memories use
`scope_assignment: legacy_default` in JSON and a `[user:legacy]` marker in human
output. Recall emits an aggregate warning whenever it returns one or more legacy
assignments. This prevents a repository-specific legacy memory from appearing
indistinguishable from an intentionally broad user instruction.

Assignment provenance uses three initial values: `automatic_context` for an
implicit repository destination, `explicit` for a named destination or reviewed
move, and `legacy_default` for migrated rows.

The user can review and confirm or move them:

```bash
thr list --scope user --legacy
thr move 42 --to repo
thr move 17 --to user
```

Moving a legacy memory to its existing user scope confirms that assignment and
changes its assignment provenance to `explicit`; subsequent identical moves are
ordinary idempotent successes. Editing text does not clear legacy provenance
because a content edit does not prove that the broad scope was reviewed.

Assigning legacy rows to `user` has a known cost: old repository-specific
memories remain broadly visible until reviewed. Quarantining them would avoid
that contamination but would make existing installations appear to lose all
memories. Preserving current behavior is the less surprising default.

Schema migration must not require re-embedding. Scope metadata changes the
eligible audience, not the semantic representation of memory text.

The vector virtual table must be rebuilt or extended with filterable scope
metadata. Existing vectors and row IDs are copied, scope metadata is populated
from the migrated memory rows, and the old representation is replaced
transactionally. A migration failure leaves the original database usable by the
original binary.

The FTS external-content table and its insert, update, and delete triggers are
rebuilt against the new scoped memory table in the same migration. Memory and
FTS row IDs are preserved. Before commit, migration validates row counts and
proves that representative existing rows remain discoverable through FTS;
integration tests cover FTS, recent substring, and fuzzy recall before and after
the format upgrade.

### Compatibility and Downgrade Safety

An older `thr` binary does not know how to filter by scope. Allowing it to open a
migrated database would make every repository visible again and could create
new broad memories. The storage migration must therefore make older binaries
fail closed for both reads and writes.

The implementation must use a database format boundary that old SQL cannot
silently traverse, such as moving scoped data to new table names and removing
the legacy tables. It must not leave a writable compatibility view or a default
scope value that lets an old writer create user-wide rows.

Downgrading requires restoring the verified pre-scope backup; a migrated
database is not directly backward compatible. Release notes and migration
errors must state this explicitly.

## Design Traps and Decisions

### Can a memory have no scope?

No. An unscoped memory has undefined recall behavior. A write without a safe
scope fails before storage.

### Can a memory belong to multiple scopes?

No. Multiple ownership makes editing, deletion, precedence, and provenance
ambiguous. Choose the least-broad scope that covers every context where the
memory is explicitly intended to apply. Create explicit copies only when
independently managed records are intentional.

### Why not use tags for repository grouping?

Tags are free-form and inconsistent. Agents may produce `test`, `tests`, and
`testing`, and retrieval can accidentally omit the tag filter. Scope is a
required normalized relationship enforced by storage and every query.

### Why `user` instead of `global`?

`global` is ambiguous: it could mean machine-wide, public, shared, or all
databases. `user` means broadly applicable within the selected local profile.

### Why not have both `user` and `global`?

In the current single-user local product they have no reliable behavioral
difference. Adding both would force agents to make a distinction that the
system cannot explain. A future team or shared scope needs a separate trust,
storage, and synchronization design.

### Why not default every write to `user` for compatibility?

It makes the most harmful mistake the easiest one. Repository facts would leak
into every future context. Existing rows migrate to `user` for compatibility,
but new writes use safer context-aware defaults.

### Why not require an explicit scope on every write?

That is safe but unnecessarily repetitive for the dominant repository workflow.
Defaulting to the current repository is both intuitive and safely narrow.

### What if an agent wants to store a user preference while inside a repo?

It uses `--scope user`. Every write response repeats the destination, and a
mistake can be corrected with `thr move`.

### What if repository detection fails?

Default repository operations fail with an actionable error. `thr` never falls
back to `user`. An explicit `--scope user` operation can still proceed when that
is truly intended.

### What does context show before a repository scope exists?

It reports a prospective repository with no ID. The first successful write
allocates the ID transactionally and returns it. Read-only inspection never
creates a placeholder scope.

### What if two agents create the first memory concurrently?

First-write registration is serialized and rechecks repository identity inside
the write transaction. Both agents converge on one repository scope unless an
explicit split already makes the remote identity ambiguous.

### What does `--scope repo` search?

Only the current repository. It does not include `user`. Omitting `--scope`
uses automatic visible scopes and includes both.

### How does an agent search another repository?

It can discover the stable ID with `thr --format json-v2 scope list`, then use
`--scope repo:<id>`, or use `--cwd` to resolve that checkout's normal context.

### Do exact ID operations depend on the current scope?

No. Scope affects discovery and default writes. A globally unique ID is an
explicit handle and remains valid across working directories. The operation
always returns the memory's scope.

### Do repository memories override user memories?

Not generally. Free-form text has no safe override key. Exact duplicate text can
appear more than once during recall; each result keeps its ID and scope.
Non-identical memories are also ranked and returned with scope metadata. Keyed
overrides and duplicate suppression are future features.

### Does a repository scope follow branches and worktrees?

Yes. Branches and linked worktrees share the logical repository scope.

### Does a repository scope follow clones?

It can when the canonical identity remote maps to exactly one existing scope.
Ambiguous matches fail instead of merging automatically.

### What happens if the remote changes?

An existing checkout remains bound to its scope and reports drift. Remote
changes never silently move repository memory.

### Can a repository scope be deleted?

Not initially. Deleting a scope without defining what happens to its memories
would violate the non-null scope invariant. Unused scopes do not participate in
default recall.

### Are scopes private or secure?

No. A process that can read the SQLite database can read all scopes. Scope
selection prevents accidental retrieval, not malicious access. Separate
databases or operating-system isolation remain the confidentiality boundary.

### Can all-scope search leak unrelated repository context?

Yes. That is why `--all-scopes` is explicit, clearly represented in output, and
not recommended in the agent's default workflow.

### What happens when an unrelated repository has a stale index?

It does not block current recall. Index health is checked only for eligible
scopes. All-scope administration can still report and repair every scope.

### Do scopes solve stale or contradictory memories?

No. Scopes reduce applicability mistakes. Provenance, verification, expiration,
supersession, and keyed conflict handling are separate future features.

## Intentional Frictions

The design introduces several deliberate costs:

| Friction | Reason |
| --- | --- |
| Broad writes require `--scope user`. | Prevents accidental cross-repository instructions. |
| Writes outside a repository require a scope. | There is no safe narrow default. |
| Ambiguous repository identity fails. | Silent scope merging is difficult to detect and undo. |
| Moving a memory uses a separate command. | Scope changes should be visible and auditable. |
| Results contain scope metadata. | Agents need provenance even though output becomes larger. |
| Empty JSON responses use an envelope. | Agents must know which scopes were searched. |
| Legacy user memories may need review. | Their original applicability was never recorded. |
| Cross-repository search is explicit. | Normal recall must not expose unrelated project context. |
| Existing databases require `thr migrate`. | The format boundary needs consent, a verified backup, and downgrade safety. |

These frictions protect durable memory quality. The common repository recall and
write path remains shorter than the exceptional broad or cross-project path.

## Known Limits

- Repository detection depends on local Git metadata and configured remotes.
- Remoteless repositories may need manual rebinding after moves or new clones.
- Two independent contexts using the same remote may require an explicit split.
- Free-form memories cannot implement reliable override semantics.
- Existing user-wide rows may contain repository facts after migration.
- Scope labels can become stale even though IDs and bindings remain valid.
- Scope-aware JSON requires a contract transition from current bare arrays.
- Migrated databases require their verified backup to downgrade to an older binary.
- Scopes do not identify which agent or source created a memory.
- Scopes do not prevent prompt injection or untrusted memory content.
- Scopes do not address long-memory embedding truncation.
- `project`, team, session, and package-level scopes are deferred.
- All scopes share one database and encryption posture.

## Rejected Alternatives

| Alternative | Reason rejected |
| --- | --- |
| Optional scope column | Creates undefined unscoped recall behavior. |
| Separate database per repository | Makes user-wide recall, clone linking, migration, and administration cumbersome. |
| Path as repository identity | Breaks on moves, symlinks, clones, and worktrees. |
| Remote URL as the only identity | Breaks on remote changes, forks, mirrors, and ambiguous reuse. |
| Repository basename as identity | Collides across owners, hosts, and unrelated local repositories. |
| Branch scopes | Fragments durable facts around temporary branches. |
| Worktree scopes | Prevents consistent recall across parallel worktrees. |
| Search all scopes and boost current | Still leaks unrelated context and lets it consume candidate limits. |
| Infer scope from memory text | Language is ambiguous and repository names can be mentioned incidentally. |
| Automatic broadening by recall count | Popularity does not imply broader applicability. |
| Arbitrary group hierarchy | Creates visibility graphs that agents cannot predict. |
| Automatic project inference | Folder and organization structure do not prove shared applicability. |
| Treat scopes as permissions | All rows remain readable from one local database. |

## Rollout

The phases below are implementation sequencing behind one release gate, not
independently shippable product releases. The scope feature does not ship until
context inspection, explicit selectors, machine metadata, migration safety,
binding recovery, memory movement, and the updated agent skill are available
together.

### Phase 1: Scope Foundation

- Add mandatory scope ownership to every memory.
- Create the singleton user scope.
- Migrate existing memories to user without re-embedding.
- Require explicit migration with a verified downgrade backup and fail-closed
  behavior for older binaries.
- Add repository registry and context resolution.
- Make text and semantic retrieval scope-aware before candidate limits.
- Include scope metadata in human output.

### Phase 2: Agent Contract

- Add `--cwd` and `thr context`.
- Add scope management and explicit selectors.
- Add versioned JSON envelopes and structured scope errors.
- Update the bundled agent skill.
- Add scope-aware stats and indexing.

### Phase 3: Memory Movement and Hardening

- Add atomic `thr move`.
- Add repository bind, unbind, and split workflows.
- Add optimistic revision and scope preconditions.
- Add tests for clones, worktrees, forks, moves, remotes, symlinks, nested
  repositories, and concurrent operations.

### Later

- Explicit multi-repository project scopes.
- Keyed memories with deterministic narrow-over-broad overrides.
- Provenance, verification, expiration, and supersession.
- Tags and metadata filters.
- Shared scopes only with a separate trust and synchronization design.

## Acceptance Criteria

The scope feature is ready when all of the following are true:

1. No stored memory can have a null or unknown scope.
2. An agent in a repository can add and recall repository memory without a
   scope flag.
3. An agent cannot accidentally create a user-wide memory because repository
   detection failed.
4. A default repository recall searches exactly the repository and user scopes.
5. An unrelated repository cannot consume text or semantic candidate limits.
6. Every human and JSON v2 result and mutation identifies its resolved scope.
7. Empty JSON v2 results identify every searched scope.
8. Worktrees share repository memory.
9. Forks with different origins do not share repository memory automatically.
10. Ambiguous repository identity returns a structured, actionable error.
11. Existing memories retain IDs, text, timestamps, and embeddings after
    migration.
12. Moving a memory preserves its ID and embedding while changing scope
    atomically.
13. A stale index in an unrelated repository does not block current recall.
14. `thr context` has no storage side effects.
15. The agent skill explains narrow writes, broad writes, context inspection,
    and cross-repository lookup.
16. Concurrent first writes from matching clones converge on one scope.
17. Migration creates and reports a verified backup before changing format.
18. Older binaries fail closed on the scoped database format.
19. FTS and vector row IDs and recall behavior survive migration.
20. Every JSON v2 envelope identifies the selected database.
21. Existing managed agent skills are updated by the installer or reported with
    an exact corrective setup command.

The central usability test is whether an agent can always answer:

```text
Where am I?
Where will this memory be stored?
Which scopes did this query search?
Where did each result come from?
```

If any answer is implicit or unavailable, the scope design is not complete.
