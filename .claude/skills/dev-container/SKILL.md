---
name: dev-container
description: Use the project's Docker dev container for Go toolchain commands (test, vet, build, run, get), make test, and make publish. Local Go 1.26 is also supported when available.
---

# Dev container

Prefer the long-running `dev` service in `docker-compose.yml` for Go commands. A local Go 1.26 toolchain also works; Go Make targets warn outside the container.

The dev image sets `WORKDIR /template-go-agent`, so commands run from the repo root by default.

## Quick start

```bash
docker compose exec -T dev make test
docker compose exec -T dev make vet
docker compose exec -T dev make publish
```

Use `-T` to disable TTY allocation when invoking from automation.

## Container lifecycle

Check first:

```bash
docker compose ps dev
```

If `dev` isn't running, start it:

```bash
docker compose up -d dev
```

Do not run `make up` — it tries to attach an interactive zsh shell.

## Common commands

| Task | Command |
|------|---------|
| Run vet and all tests | `docker compose exec -T dev make test` |
| Single package | `docker compose exec -T dev go test ./internal/memory/...` |
| Single test | `docker compose exec -T dev go test ./internal/integration/... -run TestIntegration_TimeoutTriggersMemorySweep` |
| Vet | `docker compose exec -T dev go vet ./...` |
| Build the app | `docker compose exec -T dev go build -o ./dist/template-go-agent ./cmd/template-go-agent` |
| Run the app | `docker compose exec -T dev make start` |
| Cross-compile binaries | `docker compose exec -T dev make publish` |
| Add dependency | `docker compose exec -T dev go get <module>` |
| Tidy modules | `docker compose exec -T dev go mod tidy` |

## Rules

- Prefer the container; use local Go 1.26 when available if Docker is unavailable
- Never use `make up` from automation (it's interactive)
- Always pass `-T` to `docker compose exec` for non-interactive invocation
- Build cache and module cache are persisted in named volumes, so subsequent runs are fast
