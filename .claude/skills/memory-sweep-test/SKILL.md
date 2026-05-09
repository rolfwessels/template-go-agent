---
name: memory-sweep-test
description: Integration-tests the memory sweep by building the chat binary, seeding realistic JSONL conversations, running one or more sessions, and assessing the resulting memory files. Use when verifying memory sweep behaviour, testing distiller prompt changes, checking multi-day fact separation, or validating accumulation and deduplication across sessions.
---

# Memory Sweep Test

## Quick start

1. Build the binary (always — ensures current source):
   ```
   docker compose exec dev go build -o chat ./cmd/chat
   ```
2. Write a seed file to `/tmp/sweep-test-seed.jsonl` (see format below).
3. Run a session:
   ```
   echo "hello" | ./chat --clean --user sweep-test --seed /tmp/sweep-test-seed.jsonl
   ```
4. Read and assess the memory output printed at the end.

Always use `--user sweep-test` (or another throwaway name). Never use a real user ID.

## Seed file format

One JSON object per line:
```
{"timestamp":"2026-05-09T10:00:00Z","role":"user","content":"I went to the gym today."}
{"timestamp":"2026-05-09T10:01:00Z","role":"assistant","content":"How was it?"}
{"timestamp":"2026-05-09T10:02:00Z","role":"user","content":"Great. Also, I always drink coffee every morning."}
```

- Use `role`: `user` or `assistant`
- Use realistic timestamps — the date portion determines which `daily/{date}.md` file facts land in
- Span multiple dates to test per-date distillation (change the date in the timestamp)

## Workflows

### Single-session test
```
echo "hello" | ./chat --clean --user sweep-test --seed /tmp/seed.jsonl
```
The `--clean` flag wipes previous data for the user. Use it on the first (or only) run.

### Multi-session accumulation test
```
# Session 1 — clean start
echo "hello" | ./chat --clean --user sweep-test --seed /tmp/seed1.jsonl

# Session 2 — accumulate, no --clean
echo "hello" | ./chat --user sweep-test --seed /tmp/seed2.jsonl
```
Verify that facts from session 1 persist and that duplicate facts are not re-recorded.

### Multi-day test
Put messages with different dates in a single seed file. The sweeper calls the distiller once per date and writes separate `daily/{date}.md` files.

## Assessing output

The chat binary prints all memory files on exit. Check:

| What | Expected |
|---|---|
| `daily/{date}.md` | All facts for that date — both `[general]` and untagged |
| `general.md` | Only `[general]`-tagged stable facts |
| `MEMORY.md` | One summary line per daily file, covering all facts in the file |
| Accumulation | Second session appends new facts; doesn't duplicate existing ones |
| Date separation | Multi-day seed produces one file per date, each with correct facts |

Report any deviation from expected behaviour with the actual file contents.
