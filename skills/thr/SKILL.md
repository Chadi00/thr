---
name: thr
description: Use thr whenever the task involves durable memory in any way: retrieving information that may have been stored from previous sessions, or managing memories by storing, editing, deleting, or listing them. If the user asks a question that could plausibly be answered from memory, check thr first. If the user asks to remember, update, remove, or review stored information, use thr to manage it.
---

<!-- thr:managed-skill:v1 -->

# thr Memory Management

Use `thr` when a coding task may depend on durable memory from previous sessions, or when the user asks you to remember, update, or forget something.

## Recall memories

- At the start of work that may depend on prior preferences, project decisions, or recurring workflows, run `thr ask --json "<question>"`.
- Use semantic questions for meaning, such as `thr ask --json "what does the user prefer for CLI output?"`.
- Use `thr search --json "<keywords>"` for exact identifiers, project names, tools, file names, or short phrases.
- Treat `thr ask` as retrieval only. It returns matching memories, not a generated answer.
- If semantic retrieval reports stale or missing embeddings, run `thr index` once, then retry the lookup.

## Store memories

- Store a memory when the user explicitly asks you to remember something.
- For inferred facts, preferences, or decisions, ask before storing unless the user has already given clear permission for proactive memory writes.
- Save only durable information that is likely to help in future sessions.
- Keep each memory short, standalone, and factual.
- Prefer stdin for long or multiline text: `thr add -`.

Good memories:

- `User prefers concise CLI documentation with examples.`
- `Project thr stores local memories in plaintext SQLite at ~/.thr/thr.db by default.`
- `For this repo, release commits must start with feat:, fix:, or chore: to trigger tagging.`

Avoid storing:

- Secrets, credentials, tokens, private keys, or passwords.
- Sensitive personal data unless the user explicitly asks to save it.
- Temporary logs, command output, stack traces, or one-off task state.
- Guesses, unresolved assumptions, or facts you have not verified.

## Maintain memories

- Use `thr list --last 20 --json` to inspect recent memories and ids. Adjust the count when the user asks for a smaller recent window, such as `thr list --last 4 --json`.
- Use `thr show --json <id>` before changing or deleting a memory when the exact current text matters.
- Use `thr edit <id> -` to correct an existing memory.
- Use `thr forget <id>` only when the user explicitly asks to remove a memory.

## Report usage

- When memories materially influenced your work, mention the relevant fact briefly.
- If no relevant memory exists, continue normally without making noise.
- If `thr` is not installed or not on PATH, say that memory lookup is unavailable and continue with the task.
