---
name: plan-issue
description: Read an issue, ground in CONTEXT.md/ADRs, and write a tight `## Plan` section into the issue file. Use when the dev-loop orchestrator is in the planning step or when user asks to plan an issue without implementing it. Designed to run on Opus.
---

# Plan Issue

Reads the issue, project context, and relevant code, then writes a focused implementation plan into the issue file as a `## Plan` section. The plan is a route, not a spec — it points the implementer at the right files and boundaries; details emerge during TDD.

## Input

The issue file path. Either passed in by the orchestrator or, if invoked manually, derive from the current branch (`feature/NN-slug` → `.scratch/*/issues/NN-slug.md`).

## Step 1 — Ground in domain

Read in this order:
1. The issue file — `## What to build`, `## Acceptance criteria`, any `## Notes`
2. `CONTEXT.md` — to use the project's domain vocabulary
3. `docs/adr/` — only ADRs that touch areas this issue affects (skim titles, read relevant ones)
4. The existing files most relevant to this issue (look at imports, package layout)

## Step 2 — Plan

Plan with these constraints:
- Use project domain language. If a term in the issue doesn't fit, propose a better term.
- Tests are public-interface only.
- Smallest reasonable change. No future-issue scaffolding.

## Step 3 — Append to issue file

Append a `## Plan` section to the issue file (after `## Acceptance criteria`, before any later section). Format:

```markdown
## Plan

**Files to change**
- `path/to/file.go` — one-line change description
- ...

**New types / interfaces**
- `TypeName` — one-line purpose
- (or "None")

**Domain fit**
One short paragraph mapping the change to project vocabulary. Cite the term from CONTEXT.md.

**Test approach**
- Acceptance criterion 1 → test name / file
- ...
```

Keep the whole section under 40 lines. If you're writing more, you're spec'ing not planning.

## Output

Print: `plan written to <issue-path>`

If you cannot produce a coherent plan (e.g. the issue is ambiguous), print `NEEDS_CLARIFICATION: <one-line reason>` and stop without writing.
