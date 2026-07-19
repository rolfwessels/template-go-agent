---
name: plan-refactor
description: Review the diff just implemented for an issue, scope refactoring opportunities tightly to that diff, and write `## Refactor plan` into the issue file. Use when the dev-loop orchestrator is in the refactor-planning step. Designed to run on Opus. Often the right answer is "no refactor needed".
---

# Plan Refactor

Reviews the changes made for an issue against the project's domain model and architecture, and plans focused, scoped refactoring. Honest about the common case: most issues don't need a refactor pass, especially if the implementation skill kept things clean.

## Input

The issue file path.

## Step 1 — Read context

Read:
- The issue file — especially `## Plan`, `## Implementation notes`, and `## Run log`
- `CONTEXT.md` — domain vocabulary
- `docs/adr/` — recent ADRs touching the affected area
- The diff: `git diff main...HEAD` (or `git diff $(git merge-base HEAD main)...HEAD` if main has moved)

## Step 2 — Identify smells, scoped to the diff

Look for, in priority order:
1. **Duplication** with code that already exists elsewhere (DRY violation across the diff and pre-existing code)
2. **Domain misalignment** — types, function names, or package placement that don't match `CONTEXT.md`
3. **Functions over ~15 lines** that aren't broken into named helpers
4. **Tests bound to internals** rather than public interface
5. **Misplaced abstractions** — concerns that belong in another package or layer

**Scope strictly to the new diff.** Do not propose refactors of pre-existing code unless the new diff makes the existing code's flaws materially worse. If you find pre-existing smells, note them as "follow-up issue" rather than including them in the refactor plan.

## Step 3 — Write the plan

Append a `## Refactor plan` section to the issue file. Two valid forms:

**A. Refactors needed** — bulleted list, each bullet one line:
```markdown
## Refactor plan

- `path/to/file.go:42-58` — extract `validateOrder` from `processOrder` (15+ line function, mixing validation and side effects)
- `internal/foo/bar.go` — rename `DoThing` to `RunSweep` to match CONTEXT.md term "memory sweep"
- ...
```

Keep under 15 bullets. If you have more, you're proposing a redesign — split it into a follow-up issue.

**B. No refactor** — the literal line:
```markdown
## Refactor plan

No refactor needed.
```

Use form B if the diff is already clean by domain and structure. This is a frequent and correct outcome.

## Output

Print one of:
- `refactor plan written to <issue-path>` — form A
- `refactor plan: no refactor needed` — form B
