---
name: refactor
description: Apply the bullets in the issue file's `## Refactor plan` section, run tests after each, and write `## Refactor notes`. Use when the dev-loop orchestrator is in the refactor step.
---

# Refactor

Mechanical application of an already-written refactor plan. The thinking happened in `plan-refactor`; this skill just executes carefully and keeps tests green.

## Input

The issue file path.

## Step 1 — Read the plan

Read the issue's `## Refactor plan` section. If it's the literal `No refactor needed.`:

1. Append a one-line `## Refactor notes` section: `Skipped — no refactor planned.`
2. Print `refactor skipped <issue-path>` and stop.

## Step 2 — Apply each bullet

For each bullet in the refactor plan, in order:

1. Make the change exactly as described.
2. Re-run the test suite (or at minimum the test files touched). Use `docker compose exec -T dev make test` if the project uses a dev container.
3. If a test fails:
   - The refactor is wrong, not the test — fix the refactor.
   - If you can't make it work without changing the test's intent, **skip this bullet** and note the skip in step 3. Don't change the test to match a broken refactor.
4. If a refactor turns out to conflict with pre-existing code in a way the plan didn't anticipate, skip it and note.

After all bullets are applied, run the full test suite once more.

## Step 3 — Write notes

Append `## Refactor notes` to the issue file:

```markdown
## Refactor notes

- `<short description of bullet 1>` — applied
- `<short description of bullet 2>` — skipped (one-line reason)
- ...
```

## Output

Print one of:
- `refactor applied to <issue-path>` — at least one bullet applied successfully
- `refactor skipped <issue-path>` — plan said no refactor or every bullet was skipped
- `BLOCKED: <one-line reason>` — couldn't proceed (e.g. tests broken before refactor started)
