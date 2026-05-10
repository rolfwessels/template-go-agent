---
name: dev-loop
description: Drive a TDD development loop using sub-agents with model-tiered tasks (plan/refactor-plan on Opus, the rest on Sonnet). Use when user says "run dev loop", "loop through issues", "drive the backlog", or wants automated end-to-end issue work covering planning, implementation, verification, refactor, and shipping.
---

# Dev Loop

Drives one issue through pick → plan → implement → verify → refactor-plan → refactor → ship using sub-agents, each on the right model tier. Loops until no eligible issue remains or the user stops it.

## Trust model

Trust each sub-agent's report. Do **not** re-read the diff, re-run tests, or re-check the issue file to verify a sub-agent's claim — only react if the sub-agent itself reports failure. We sharpen the sub-skills over time, not the orchestrator. Cheap, fast iteration.

## State and handoff

The issue file is the contract. Each sub-agent reads the predecessor's section and writes its own:

| Section | Written by |
|---|---|
| `Status:` | orchestrator boundaries (pick → `in-progress`, ship → `done`) |
| `## Plan` | `plan-issue` |
| `## Implementation notes` | `tdd-implement` |
| `## Run log` | `verify-run` |
| `## Refactor plan` | `plan-refactor` |
| `## Refactor notes` | `refactor` |

The orchestrator only passes the issue path forward — never re-summarises.

## Models

| Step | Skill | Model |
|---|---|---|
| 1. Pick | `pick-next-issue` | sonnet |
| 2. Plan | `plan-issue` | **opus** |
| 3. Implement | `tdd-implement` | sonnet |
| 4. Verify run | `verify-run` | sonnet |
| 5. Plan refactor | `plan-refactor` | **opus** |
| 6. Refactor | `refactor` | sonnet |
| 7. Ship | `ship-issue` | sonnet |

## Loop

Repeat until exit condition (see below):

### Step 1 — Pick

Invoke the `pick-next-issue` skill **directly in this context** (no sub-agent — needs to interact with the user for confirmation, branch creation, working-tree checks).

If it reports `NO_ISSUE`: print a one-line summary of completed iterations and stop.

Capture `issue_path=<path>` from its output.

### Steps 2–7 — Sub-agent dispatch

For each step below, spawn an `Agent` with `subagent_type: general-purpose`, the listed model, and the prompt template. Wait for its report before moving on.

```
Agent({
  subagent_type: "general-purpose",
  model: "<model>",
  description: "<step name>",
  prompt: "Use the <skill-name> skill on issue path: <issue_path>. Report back with the skill's standard output line."
})
```

Step-specific notes:

- **Step 4 (verify-run)**: if the sub-agent reports `verdict=FAIL` or a build/run error, stop the loop and print the failure. Do not proceed to refactor or ship — the user needs to intervene.
- **Step 5 (plan-refactor)**: if the sub-agent reports `no refactor needed`, skip step 6 entirely.
- **Step 7 (ship-issue)**: this is the only step that can fail "softly" — if the user declines to push/merge, leave the branch as-is and stop the loop with a note.

### Step 8 — Iteration summary

Print one line: `Issue NN-slug: shipped` (or `: failed at <step>`).

Then back to Step 1.

## Exit conditions

The loop stops when any of these happen:
- `pick-next-issue` reports `NO_ISSUE`
- Any sub-agent reports a hard failure
- `verify-run` reports `FAIL`
- The user interrupts or declines to ship

On exit, print:
```
Dev loop done.
- Issues shipped: <count> (<list of issue numbers>)
- Stopped because: <reason>
```

## Manual override

If the user wants to run a single step (e.g. just plan, just refactor), invoke that skill directly rather than running the full loop.
