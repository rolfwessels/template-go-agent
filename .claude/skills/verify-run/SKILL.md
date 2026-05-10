---
name: verify-run
description: Build and run the application, exercise the issue's acceptance criteria against the running app, and append `## Run log` to the issue file. Use when the dev-loop orchestrator is in the verify step or when user asks to validate an implementation by actually running it.
---

# Verify Run

Tests confirm units; this confirms the system. Builds the app, runs it, drives it through each acceptance criterion, and records the verdict. Honest about skipping when the environment can't run the app (missing credentials, hardware, etc.).

## Input

The issue file path.

## Step 1 — Determine how to run

Inspect the project to find:
- **Build command**: `make build`, `go build`, `cargo build`, etc.
- **Run command**: `./bin/app`, `make run`, `docker compose up`, etc.
- **Required env vars**: read `README.md`, `.env.example`, `Makefile`. Note any that look like credentials or external-service config.

Sources to check, in order: `Makefile`, `README.md`, `docker-compose.yml`, the CI config.

If running requires credentials that aren't available locally, jump to Step 4 with verdict `SKIPPED` and the reason.

## Step 2 — Build

Run the build. If the project uses a dev container, build inside it. If the build fails, jump to Step 4 with verdict `FAIL` — do not attempt to run.

## Step 3 — Exercise acceptance criteria

For each acceptance criterion in the issue, drive the app to exercise it:
- **CLI app**: invoke with the relevant flags / subcommands; capture stdout, stderr, exit code
- **Service**: fire requests; capture responses
- **Library**: not applicable — verify-run skip with reason `library, no runnable surface`

Keep runs short. Don't open interactive sessions or long-running processes. If a criterion needs a flow, scripted input is fine; manual input is not.

Capture concrete evidence per criterion: a command invocation and one line of its output, an error message, an exit code, a file written.

## Step 4 — Write run log

Append a `## Run log` section to the issue file:

```markdown
## Run log

**Build**: pass | fail (one-line note)

- AC 1 — pass | fail | skipped (one line of evidence: command + output snippet, or skip reason)
- AC 2 — ...

**Verdict**: PASS | FAIL | SKIPPED (with one-line reason if not PASS)
```

Keep under 30 lines. Truncate long output to the meaningful slice.

## Output

Print one of:
- `verify <issue-path>: PASS`
- `verify <issue-path>: FAIL — <one-line reason>`
- `verify <issue-path>: SKIPPED — <one-line reason>`

The orchestrator stops the loop on FAIL.
