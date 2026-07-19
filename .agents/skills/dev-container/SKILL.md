---
name: dev-container
description: Run all Go toolchain commands (test, vet, build, run, get) inside the project's Docker dev container, never on the host. Use when the agent needs to run `go test`, `go vet`, `go build`, `go run`, `go get`, `make test`, `make publish`, or any Go-based command in this repo. The host has no Go toolchain installed.
---

# Dev container

This repo runs all Go commands inside a long-running `dev` service defined in `docker-compose.yml`. The host shell has no `go` binary. Always use the container.

The dev image sets `WORKDIR /skillsync`, so commands run from the repo root by default.

## Quick start

```bash
docker compose exec -T dev go test ./...
docker compose exec -T dev go vet ./...
docker compose exec -T dev make publish
```

Use `-T` to disable TTY allocation when invoking from automation.

## Container lifecycle

Check first:

```bash
docker ps --filter "name=skillsync-dev" --format '{{.Status}}'
```

If empty, start it:

```bash
docker compose up -d dev
```

Do not run `make up` — it tries to attach an interactive zsh shell.

## Common commands

| Task | Command |
|------|---------|
| Run all tests | `docker compose exec -T dev go test ./...` |
| Single package | `docker compose exec -T dev go test ./internal/sync/...` |
| Single test | `docker compose exec -T dev go test ./internal/sync/... -run TestPush` |
| Vet | `docker compose exec -T dev go vet ./...` |
| Cross-compile binaries | `docker compose exec -T dev make publish` |
| Add dependency | `docker compose exec -T dev go get <module>` |
| Tidy modules | `docker compose exec -T dev go mod tidy` |

## Rules

- Never call `go ...` directly on the host
- Never use `make up` from automation (it's interactive)
- Always pass `-T` to `docker compose exec` for non-interactive invocation
- Build cache and module cache are persisted in named volumes, so subsequent runs are fast
