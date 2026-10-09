# 💻 Development

Use the [quick start](../README.md#-quick-start) for setup. Commands below are defined in the [Makefile](../Makefile); Go commands work in the dev shell or locally with Go 1.26 and the needed tools. Outside the container, Go targets print one short warning; help and version output stay quiet.

## Make commands

| Command | Purpose |
|---|---|
| `make` / `make help` | List commands from target descriptions in the Makefile. |
| `make up` | On host: start Compose and attach a zsh dev shell. |
| `make down` | On host: stop Compose. |
| `make build` | On host: stop Compose, then rebuild the dev image. |
| `make shell` | On host: attach zsh to the running dev container. |
| `make start` | Run `go run ./cmd/template-go-agent`; CLI or Discord based on config. |
| `make vet` | Run `go vet ./...`. |
| `make test` | Vet, then run all tests verbosely without cached results. |
| `make version` / `make print-version` | Display version / emit the plain version for scripts. |
| `make publish` | Replace `dist/` with binary-only archives for five platforms (requires tar/zip). |
| `make docker-build` | Build the production `runtime` image with version and branch tags. |
| `make docker-push` | Push all local tags for `rolfwessels/template-go-agent`. |
| `make docker-publish` | Build and push; run `docker login` first. |

For CLI chat, use `make start` with `DISCORD_TOKEN` empty. Deployment is handled separately from image publishing. To compile locally, use `go build ./...`; `make build` rebuilds Compose.

## Versioning and PRs

Use `feature/` or `bug/` branches off `main`, then open a PR against `main`. Run `make test` before review.
Versions follow a SemVer-style scheme: `versionPrefix` in the Makefile sets major/minor manually (`0.1` currently); patch is the full Git commit count on `main`.
Off `main`, versions are `0.1.<origin/main-count>.<branch-commit-count>-alpha`. Keep full Git history and an `origin/main` ref for meaningful versions; fallback counts are zero when Git data is unavailable.
PR builds upload archives as workflow artifacts. A push to `main` creates a versioned [GitHub release](https://github.com/rolfwessels/template-go-agent/releases).
`make publish` builds Linux amd64/arm64, macOS amd64/arm64, and Windows amd64. Unix archives are `.tar.gz`; Windows uses `.zip`. Prompts are embedded; archives contain only the executable.

## CI

The [workflow](../.github/workflows/github-action.yml) runs on pushes to `main` and PRs targeting `main`, with full-history checkouts.

| Job | Runs after | Work |
|---|---|---|
| `test` | — | Go 1.26; `make test` and `go build ./cmd/template-go-agent`. |
| `build` | `test` | `make publish`; upload archives with 90-day retention. |
| `release` | `build` | On `main`, download archives and publish release `v<version>`. |
| `docker` | `test` | Only upstream pushes to `main`; Buildx/QEMU build and Docker Hub push. |

## Docker publishing

The [Dockerfile](../Dockerfile) has shared `base`, compiler `build`, minimal Alpine `runtime`, and full-toolchain `dev` stages. [Compose](../docker-compose.yml) uses `dev`; publishing uses `runtime` with the version embedded in the binary.
CI publishes `linux/amd64` and `linux/arm64` images to `rolfwessels/template-go-agent` after tests pass. Docker publishing does not run for PRs or forks.

| Main image tag | Value |
|---|---|
| `alpha` | Updated on each upstream push to `main`. |
| `latest` | Updated on each upstream push to `main`. |
| `v<version>` | `v` plus `make -s print-version`, using full Git history. |
| `<short-sha>` | 8-character short SHA via `git rev-parse --short=8 HEAD`. |

Configure repository Actions secrets `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` (a Docker Hub token with push access).
Local publishing: run `docker login`, then `make docker-publish` (or `make docker-build docker-push`). CI authenticates with `docker/login-action` using those secrets. Off `main`, local tags are `alpha`, `<short-sha>`, and `v<version-full>`; `latest` is only added on `main`. Local `docker-build` builds for the local platform; CI handles both architectures.
Runtime state is under `.storage/` in the working directory; mount persistent storage when running a production container.
