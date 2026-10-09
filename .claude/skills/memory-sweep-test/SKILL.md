---
name: memory-sweep-test
description: Verify memory sweeps with the integration tests and, when credentials are available, the real CLI in isolated storage. Use after agent wiring, distiller, sweeper, or usage tracking changes.
---

# Memory Sweep Test

## Automated checks

Run the existing tests first; `make test` includes them:

```bash
docker compose exec -T dev make test
```

For a focused run:

```bash
docker compose exec -T dev go test ./internal/memory/... ./internal/integration/... -v -count=1
```

These cover sweep lifecycle, date separation, accumulation, and cursor behaviour without API credentials. Local Go 1.26 can run the same commands without the Compose prefix.

## Live CLI check

The CLI uses user ID `cli` and stores state relative to its working directory. Use a fresh temporary directory so real user data stays untouched. Export `OPENAI_API_KEY` and `TAVILY_API_KEY` on the host before running; if unavailable, report the live check as skipped.

Build the real executable, then send a message and let EOF trigger shutdown and a Memory Sweep:

```bash
docker compose exec -T dev go build -o /tmp/template-go-agent-sweep ./cmd/template-go-agent
docker compose exec -T -e OPENAI_API_KEY -e TAVILY_API_KEY -e DISCORD_TOKEN= dev sh -c '
  set -e
  sweep_dir=$(mktemp -d)
  cd "$sweep_dir" || exit 1
  printf "%s\n" "Remember that I prefer black coffee." | /tmp/template-go-agent-sweep
  printf "Inspect sweep output in %s\n" "$sweep_dir"
'
```

Inspect the reported directory inside `dev` with `docker compose exec -T dev`. Reuse that directory for a second session to check accumulation. The CLI has no seed, clean, or user-selection flags; multi-day scenarios belong in the automated tests.

## Assess output

All paths below are relative to the temporary working directory:

| Path | Expected |
|---|---|
| `.storage/user/cli/memory/daily/{date}.md` | Distilled facts for the source date. |
| `.storage/user/cli/memory/general.md` | Stable facts classified as general. |
| `.storage/user/cli/memory/MEMORY.md` | Index with daily summaries. |
| `.storage/user/cli/sessions/*.swept_until` | Cursor advanced after a successful sweep. |
| `.storage/user/cli/metrics/costs.jsonl` | Agent and distiller records with Session IDs and token counts. |
| `.storage/logs/app.log` | Sweep completion and usage records; check any errors. |

A second session should preserve earlier facts without duplication. Cost depends on the configured model and pricing table; unknown models record zero.

Report commands, sweep results, and any skipped checks. Remove only the temporary directory you created and `/tmp/template-go-agent-sweep` when finished.
