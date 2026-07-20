---
name: thr
description: Use thr for durable memory across coding sessions, including retrieving saved preferences or project facts and managing memories by storing, editing, deleting, or listing them. Use when a question could plausibly be answered from memory, or when the user asks to remember, update, remove, or review stored information.
---

<!-- thr:managed-skill:v2 -->

# thr Memory Management

Use `thr` when a coding task may depend on durable memory from previous sessions, or when the user asks you to remember, update, or forget something.

## Resolve context

- Run `thr --format json-v2 context` when repository context is unclear. It is read-only and reports default read and write scopes.
- Use `--cwd /path/to/repo` to resolve another checkout without changing the process working directory, for example `thr --cwd /work/api --format json-v2 context`.

## Recall memories

- In a repository, default recall searches that repository plus the user scope. Outside a repository, it searches the user scope.
- At the start of work that may depend on prior preferences, repository decisions, or recurring workflows, run `thr --format json-v2 ask "<question>"`.
- Use `thr --format json-v2 search "<keywords>"` for exact identifiers, project names, tools, file names, or short phrases.
- Use JSON v2 for new agent integrations because it includes searched scopes and each result's scope, including empty recall results.
- Treat `thr ask` as retrieval only. It returns matching memories, not a generated answer.
- If semantic retrieval reports stale or missing embeddings, run `thr index` once, then retry the lookup.
- For another registered repository, get its stable ID with `thr --format json-v2 scope list`, then recall with `--scope repo:<id>`. Memory IDs also remain valid across repositories.

## Store memories

- Store a memory when the user explicitly asks you to remember something.
- For inferred facts, preferences, or decisions, ask before storing unless the user has already given clear permission for proactive memory writes.
- Store in the least-broad scope covering every context where the fact is explicitly intended to apply.
- In a repository, an unqualified `thr add` writes to that repository. Use explicit `--scope user` only for facts intended to apply across repositories. Outside a repository, writes require `--scope user`.
- Save only durable information that is likely to help in future sessions.
- Keep each memory short, standalone, and factual.
- Prefer stdin for long or multiline text: `thr add -`.

Good memories:

- `User prefers concise CLI documentation with examples.`
- `This repository runs integration tests with Docker.`
- `For this repo, release commits must start with feat:, fix:, or chore: to trigger tagging.`

Avoid storing:

- Secrets, credentials, tokens, private keys, or passwords.
- Sensitive personal data unless the user explicitly asks to save it.
- Temporary logs, command output, stack traces, or one-off task state.
- Guesses, unresolved assumptions, or facts you have not verified.

## Maintain memories

- Use `thr --format json-v2 list --last 20` to inspect recent memories, IDs, and scopes.
- Use `thr --format json-v2 show <id>` before changing or deleting a memory when the exact current text matters.
- Use `thr edit <id> -` to correct an existing memory.
- Use `thr forget <id>` only when the user explicitly asks to remove a memory.
- Use `thr move <id> --to user`, `--to repo`, or `--to repo:<id>` to correct scope without changing the memory ID or text.

## Report usage

- When memories materially influenced your work, mention the relevant fact briefly.
- If no relevant memory exists, continue normally without making noise.
- If `thr` is not installed or not on PATH, say that memory lookup is unavailable and continue with the task.
