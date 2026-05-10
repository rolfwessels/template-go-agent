---
name: pick-next-issue
description: Find the next unblocked ready-for-agent issue, briefly summarise it, create the feature branch, and flip status to in-progress. Use when starting a new development cycle, when the dev-loop orchestrator is picking up the next issue, or when the user says "pick the next issue".
---

# Pick Next Issue

Finds the next unblocked `ready-for-agent` issue under `.scratch/`, creates the feature branch, flips status to `in-progress`, and emits the issue path for downstream skills.

## Step 1 — Find the next issue

Run from the repo root:

```sh
~/.claude/skills/loop-dev-issues/scripts/next-issue.sh
```

It prints the path of the next unblocked `ready-for-agent` issue (lowest number first), or exits non-zero if none.

If non-zero exit: print
```
NO_ISSUE
```
and stop. Do not invent work.

## Step 2 — Read and summarise

Read the issue file. Present briefly:
- **Issue**: file path
- **Title**: from the first `# ` heading or the filename
- **What to build**: one-line summary (compress the section, don't quote it whole)
- **Acceptance criteria**: count of items

Ask: "Pick this up? (y to proceed)". If the user declines, print `DECLINED` and stop.

## Step 3 — Working-tree check

If `git status --porcelain` is non-empty, stop and ask the user to stash or commit first. Don't proceed.

## Step 4 — Create feature branch

Derive the branch name from the issue file name: `feature/<NN>-<slug>` (e.g. `02-cli-adapter.md` → `feature/02-cli-adapter`).

```sh
git fetch origin 2>/dev/null || true
git checkout main && git reset --hard origin/main 2>/dev/null || git checkout main
git checkout -b feature/NN-slug 2>/dev/null || git checkout feature/NN-slug
```

- If `git fetch` fails (no remote), skip silently.
- If the branch already exists, switch to it instead of erroring.

## Step 5 — Flip status

Edit the issue file's `Status:` line: `ready-for-agent` → `in-progress`.

## Step 6 — Output

Print exactly these two lines as the final output:

```
issue_path=<path>
feature_branch=feature/NN-slug
```

Downstream skills and the orchestrator parse these.
