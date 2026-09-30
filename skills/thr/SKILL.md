---
name: thr
description: Use thr for durable memory across coding sessions. Recall saved context when prior knowledge may help, proactively store verified preferences, decisions, non-obvious constraints, or reusable lessons, and correct stale memories when new information is verified. Also use when the user asks to remember, update, remove, or review stored information.
---

<!-- thr:managed-skill:v2 -->

# thr Memory Management

Use `thr` to recall useful context and preserve valuable knowledge learned during coding tasks. Store verified, durable context that improves future decisions or avoids meaningful rediscovery, so future sessions can benefit from what you learn. Also use it when the user asks you to remember, update, or forget something.

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
- Proactively store qualifying, non-sensitive information without asking for confirmation each time. Respect user instructions against saving information.
- Before completing a task, assess what you learned and save information that meets the relevance criteria below. Capture significant discoveries or corrections once they are verified, while the necessary context is still available.
- Assess relevance even when recall found nothing. Empty recall is neither a reason to skip assessment nor a reason to create a memory. There is no quota; save nothing when nothing qualifies.
- Before writing, check relevant recall results or run `thr --format json-v2 ask "<candidate topic>"` or `thr --format json-v2 search "<keywords>"` for existing memories. Choose whether to skip, update, or create using the rules below.
- Store in the least-broad scope covering every context where the fact is explicitly intended to apply.
- In a repository, an unqualified `thr add` writes to that repository. Use explicit `--scope user` only for facts intended to apply across repositories. Outside a repository, writes require `--scope user`.
- Keep each memory short, standalone, and factual. Include the actionable lesson, its applicability, and the rationale when it explains a decision or workaround; omit the investigation transcript.
- Wrap multiword direct text in ASCII double quotes, for example `thr add "Run integration tests with --workers 1 in this repository to avoid shared fixture collisions"`. Prefer stdin for long or multiline text: `thr add -`.

### Create or update

- Skip a write when an existing memory already captures the same information, even if the wording differs.
- Update an existing memory when verified information corrects or materially refines the same fact, preference, decision, or recommendation in the same applicable scope. Useful rationale or additional details about that fact belong in the existing entry.
- Create a new memory for a distinct, independently useful fact or a different scope or applicability. A related topic alone is not a reason to merge entries. For example, a repository-specific exception to a user-wide preference belongs in a repository memory, without overwriting the broader preference.
- Before editing, inspect the full entry with `thr --format json-v2 show <id>`. Use `thr edit <id> -` to replace its text, preserving still-valid details and removing superseded guidance. Editing retains the memory ID and scope.

### Relevance criteria

Save information likely to remain useful in future sessions when it improves decisions or avoids repeating meaningful investigation. Prioritize:

- Established user preferences or corrections that should guide future work.
- Confirmed decisions and their rationale, especially tradeoffs not apparent from the implementation.
- Non-obvious constraints, recurring workflow pitfalls, and verified workarounds with their cause and applicability.
- Reusable lessons from substantial investigation that would be costly to rediscover.

Skip facts an agent can cheaply discover from repository files, such as the programming language, obvious directory structure, or ordinary manifest contents. Routine documentation does not need to be copied into memory; preserve useful context or rationale that is missing or difficult to find.

Good memories:

- `User prefers concise CLI documentation with examples.`
- `In this repository, integration tests share a database fixture; run with --workers 1 to avoid intermittent collisions.`
- `This repository keeps migrations compatible with the previous release because deployments run old and new workers concurrently.`

Avoid storing:

- Secrets, credentials, tokens, private keys, or passwords.
- Sensitive personal data unless the user explicitly asks to save it.
- Temporary logs, command output, stack traces, or one-off task state.
- Transient failures without a verified, reusable lesson.
- Guesses, unresolved assumptions, or facts you have not verified.

## Maintain memories

- When current user clarification or verified project evidence shows that a recalled memory is out of date, proactively correct it using the create-or-update rules above. Do this even when you have no separate new memory to save; an explicit request to remember is not required.
- If a contradiction is only suspected, verify it before editing. Different contexts or scopes can explain differing facts; do not overwrite a memory based on an assumption.
- Use `thr --format json-v2 list --last 20` to inspect recent memories, IDs, and scopes.
- Use `thr --format json-v2 show <id>` before changing or deleting a memory when the exact current text matters.
- Use `thr edit <id> -` to correct an existing memory.
- Use `thr forget <id>` only when the user explicitly asks to remove a memory.
- Use `thr move <id> --to user`, `--to repo`, or `--to repo:<id>` to correct scope without changing the memory ID or text.

## Report usage

- After successfully saving or updating a memory, briefly mention what you preserved. Do not claim a write succeeded if it failed.
- When memories materially influenced your work, mention the relevant fact briefly.
- Do not report empty recall results or an assessment that found nothing to save.
- If `thr` is not installed or not on PATH, say that memory lookup is unavailable and continue with the task.
