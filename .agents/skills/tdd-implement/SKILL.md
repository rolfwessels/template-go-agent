---
name: tdd-implement
description: Read an issue's `## Plan` section and implement it using TDD, then write `## Implementation notes`. Use when the dev-loop orchestrator is in the implement step, or when user asks to implement against an existing plan.
---

# TDD Implement

Implements an issue using test-driven development, guided by the `## Plan` section already written into the issue file.

## Input

The issue file path.

## Step 1 — Read plan

Read the issue file. You need:
- `## What to build`
- `## Acceptance criteria`
- `## Plan`

If `## Plan` is missing, print `NO_PLAN` and stop. Do not improvise — call the planning skill first.

Also read `CONTEXT.md` and any ADRs the plan references. Use project domain vocabulary throughout the implementation.

## Step 2 — TDD per criterion

For each acceptance criterion (in the order listed):

1. **Red** — write a failing test that captures the behavior. Use public interfaces only, never internal implementation details.
2. **Green** — write the smallest code change that makes the test pass. Don't add functionality the test doesn't demand.
3. **Refactor** — only if needed for clarity. Heavy refactor belongs in the refactor step, not here.
4. Confirm the test passes before moving to the next criterion.

After all criteria pass, run the full test suite. Use the project's standard command:
- `Makefile` with `test` target → `make test`
- Dev container in use → `docker compose exec -T dev make test` (this project uses one — see the `dev-container` skill)
- Otherwise infer (`go test ./...`, `cargo test`, etc.)

If a test fails that you didn't introduce, stop and report — don't paper over it.

## Step 3 — Write implementation notes

Append a `## Implementation notes` section to the issue file. Tight — under 20 lines:

```markdown
## Implementation notes

- What got built (one or two bullets)
- Any deviation from the plan + why
- Anything follow-up worth a future issue
```

## Output

Print one of:
- `implemented <issue-path>` — success
- `NO_PLAN` — plan section missing
- `BLOCKED: <one-line reason>` — couldn't proceed (e.g. dependency not present, plan doesn't match codebase reality)

Do **not** flip the issue status to `done` — that's the ship-issue skill's job.
