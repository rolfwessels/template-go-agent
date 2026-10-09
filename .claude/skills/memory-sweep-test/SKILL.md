---
name: memory-sweep-test
description: Integration-tests the memory sweep by building the chat binary, seeding realistic JSONL conversations, running one or more sessions, and assessing the resulting memory files and Cost Ledger. Use when verifying memory sweep behaviour, testing distiller prompt changes, checking multi-day fact separation, validating accumulation and deduplication across sessions, or after wiring any new component into the agent, distiller, sweeper, or usage tracking pipeline.
---

# Memory Sweep Test

The `cmd/chat` test harness (with `--clean`, `--user` and `--seed`) is not in this repo yet; until it is, run `docker compose exec -T dev make test` instead (which covers the memory sweep integration tests).

Runs the real binary against real OpenAI so the full stack is exercised: agent → LLM → memory sweep → distiller → Cost Ledger. This is the final verification step after any change to agent wiring, the distiller, the sweeper, or usage tracking.

## Quick start

1. Build the binary (always — ensures current source):
   ```
   docker compose exec -T dev go build -o chat ./cmd/chat
   ```
2. Write a seed file to `.storage/sweep-test-seed.jsonl` (gitignored — stays out of the repo).
3. Run a session:
   ```
   echo "hello" | ./chat --clean --user sweep-test --seed .storage/sweep-test-seed.jsonl
   ```
4. Read and assess the memory and Cost Ledger output.

Always use `--user sweep-test` (or another throwaway name). Never use a real user ID — `--clean` wipes all data for that user.

## Seed file format

One JSON object per line, saved to `.storage/`:
```
{"timestamp":"2026-05-09T10:00:00Z","role":"user","content":"I went to the gym today for the first time in months."}
{"timestamp":"2026-05-09T10:01:00Z","role":"assistant","content":"How was it?"}
{"timestamp":"2026-05-09T10:02:00Z","role":"user","content":"Great. I always drink black coffee every morning — it's a ritual."}
{"timestamp":"2026-05-10T09:00:00Z","role":"user","content":"Today I finished reading Meditations by Marcus Aurelius. I prefer non-fiction in general."}
{"timestamp":"2026-05-10T09:01:00Z","role":"assistant","content":"A great choice. What did you take from it?"}
```

- Use `role`: `user` or `assistant`
- Use realistic timestamps — the **date portion** determines which `daily/{date}.md` file facts land in
- Span multiple dates to test per-date distillation

## Workflows

### Single-session test
```
echo "hello" | ./chat --clean --user sweep-test --seed .storage/sweep-test-seed.jsonl
```
The `--clean` flag wipes previous data for the user. Use it on the first (or only) run.

### Multi-session accumulation test
```
# Session 1 — clean start
echo "hello" | ./chat --clean --user sweep-test --seed .storage/sweep-test-seed1.jsonl

# Session 2 — accumulate, no --clean
echo "hello" | ./chat --user sweep-test --seed .storage/sweep-test-seed2.jsonl
```
Verify that facts from session 1 persist and that duplicate facts are not re-recorded.

### Multi-day test
Put messages with different dates in a single seed file. The sweeper calls the distiller once per date and writes separate `daily/{date}.md` files.

## Assessing output

The chat binary prints all memory files on exit. The log lines (written to stdout during the run) show every usage record as it is written.

### Memory files

| What | Expected |
|---|---|
| `daily/{date}.md` | All facts for that date — both `[general]` and untagged |
| `general.md` | Only `[general]`-tagged stable facts |
| `MEMORY.md` | One summary line per daily file, covering all facts in the file |
| Accumulation | Second session appends new facts; doesn't duplicate existing ones |
| Date separation | Multi-day seed produces one file per date, each with correct facts |

### Cost Ledger

Check `.storage/user/sweep-test/metrics/costs.jsonl` after the run:

```
cat .storage/user/sweep-test/metrics/costs.jsonl
```

| What | Expected |
|---|---|
| `component=agent` entries | One per user turn (the "hello" message + any from the seed that triggered the agent) |
| `component=distiller` entries | One per date swept — the distiller is called once per date in the session log |
| `session_id` | Same value across all entries from the same session |
| `prompt_tokens` / `completion_tokens` | Non-zero real counts from OpenAI |
| `cost_usd` | Non-zero, calculated from the pricing table in `internal/usage/pricing.go` |

Example of a healthy ledger after a two-date seed:
```json
{"timestamp":"...","component":"agent","model":"gpt-5.5","prompt_tokens":2007,"completion_tokens":44,"total_tokens":2051,"cost_usd":0.011355,"session_id":"..."}
{"timestamp":"...","component":"distiller","model":"gpt-5.5","prompt_tokens":241,"completion_tokens":148,"total_tokens":389,"cost_usd":0.005645,"session_id":"..."}
{"timestamp":"...","component":"distiller","model":"gpt-5.5","prompt_tokens":225,"completion_tokens":148,"total_tokens":373,"cost_usd":0.005565,"session_id":"..."}
```

### Log lines to look for

During the run the binary logs each usage record in real time:
```
INFO usage component=agent   model=gpt-5.5 prompt_tokens=... completion_tokens=... cost_usd=... session_id=...
INFO usage component=distiller model=gpt-5.5 prompt_tokens=... completion_tokens=... cost_usd=... session_id=...
```
Missing log lines mean the tracker was not wired correctly to that component.

## Cleanup

The binary (`./chat`) and seed files (`.storage/sweep-test-seed*.jsonl`) are both gitignored — no manual cleanup needed. Memory and Cost Ledger data land under `.storage/user/sweep-test/` which is also gitignored.
