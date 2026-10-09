---
name: ship-issue
description: Update docs, run tests, commit on the feature branch, then either open a PR (if origin exists) or squash-merge into main. Flips issue status to `done`. Use when the user says "ship it", "commit this", "wrap up the issue", or when the dev-loop orchestrator reaches the ship step.
---

# Ship Issue

Wraps up an implemented issue: doc updates, tests, commit, then PR or squash-merge depending on whether origin exists. Final step that flips the issue to `done`.

## Input

The issue file path. Either:
- **Passed in** by the orchestrator (preferred)
- **Inferred from branch** — if on `feature/NN-slug`, look for `.scratch/*/issues/NN-slug.md`
- **Fallback** — most recently `in-progress` issue: `~/.claude/skills/loop-dev-issues/scripts/list-issues.sh --status in-progress | sort | tail -1`

If multiple `in-progress` issues exist and no path is passed, ask the user which one.

## Step 1 — Update documentation

Read the issue's `## What to build`, `## Implementation notes`, and `## Refactor notes`. Then:

1. **`CONTEXT.md`** — sharpen any domain terms introduced or clarified by this issue. Only touch entries that are incomplete, missing, or wrong. Do not rewrite accurate content.
2. **[README.md](../../../README.md)** — update quick start when startup/usage changes; document env vars in [configuration](../../../docs/configuration.md), commands in [development](../../../docs/development.md), and fetch policy in [HTTP fetch security](../../../docs/http-fetch-security.md).
3. Other docs referenced in the issue.

If nothing user-facing changed, say so and skip.

## Step 2 — Run tests

Run the full test suite. If the project uses a dev container (this one does — see the `dev-container` skill): `docker compose exec -T dev make test`. Otherwise infer (`make test`, `go test ./...`, etc.).

If integration tests exist (`make test-integration`, `*_integration_test.go`, dedicated script), run them too unless they require unavailable credentials — note any skip explicitly.

If anything fails, stop and report. Don't commit broken code.

## Step 3 — Commit on the feature branch

Run the branch-info script:

```sh
~/.claude/skills/ship-issue/scripts/branch-info.sh <issue-path>
```

It prints `current_branch`, `on_main`, `feature_branch`, `branch_exists`.

- If already on the correct feature branch: just commit.
- If on main or another branch: `git checkout -b feature/NN-slug` (or `git checkout feature/NN-slug` if it exists), then commit.

Stage with `git add -A`. Note: `.scratch/` issue files are typically gitignored, so the status flip won't appear in the commit — that's expected.

**Infer commit style** from `git log main --oneline -10`:
- Emoji prefix (`✨`, `🐛`, `📝`, etc.) — pick one that fits: ✨ feature · 🐛 fix · ♻️ refactor · 📝 docs · 🧪 tests · 🔧 config
- Conventional Commits (`feat:`, `fix:`, `chore:`) — follow that
- Other consistent prefix — follow it
- No clear pattern — default to emoji

Commit message format (HEREDOC for safe quoting):

```
<prefix> <issue title in sentence case>

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

## Step 4 — Flip status

Update the issue file's `Status:` line: `in-progress` → `done`. Append a final `## Implementation notes` line if missing (e.g. `Shipped on <date>`).

This is the **only** place the status moves to `done`. Don't let earlier skills do it.

## Step 5 — Push and PR, or squash-merge

Run:

```sh
~/.claude/skills/ship-issue/scripts/remote-info.sh
```

It prints `has_origin` and `origin_url`.

### `has_origin=true`

Ask: "There's a remote at `<origin_url>`. Push and open a PR?"

If yes:
1. `git push -u origin feature/NN-slug`
2. Create the PR (HEREDOC for the body):

   ```sh
   gh pr create --title "<issue title>" --body "$(cat <<'EOF'
   ## Summary
   <bullets from What to build>

   ## Implementation notes
   <from Implementation notes>

   🤖 Generated with [Claude Code](https://claude.com/claude-code)
   EOF
   )"
   ```
3. Print the PR URL.

### `has_origin=false`

Ask: "No remote origin found. Squash and merge `feature/NN-slug` into main?"

If yes:
1. `git checkout main`
2. `git merge --squash feature/NN-slug`
3. Commit with the same message format as Step 3.
4. `git branch -D feature/NN-slug` (uppercase `-D`; squash merges aren't "fully merged" by git, so `-d` always fails).
5. Confirm: "Squash-merged into main. Branch `feature/NN-slug` deleted."

If no: leave the branch as-is, tell the user where things stand, stop.

## Output

Print one of:
- The PR URL
- `squash-merged into main`
- `held: <one-line reason>` — user declined to push/merge or tests failed
