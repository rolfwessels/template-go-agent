# Agent Instructions

Read [README.md](README.md) for project setup and commands. Read [CONTEXT.md](CONTEXT.md) for domain vocabulary, storage, and lifecycle, and consult the [architecture decisions](docs/adr/) before changing those boundaries.

## What we value

- Build the simplest solution that works. Prefer clear names, small functions, and straightforward control flow.
- Keep changes focused and reversible. Add abstractions when the code has a concrete need for them.

## Conventions and boundaries

- Ask before adding external dependencies or changing database migrations.
- Put detailed workflows in skills or nested `AGENTS.md` files.
- Branch from `main` using `feature/<name>` for features and `bug/<name>` for fixes. Open pull requests against `main`.
- Use **What**, **Why**, **Decisions**, **Testing**, and **Risks** as pull request headings.
- Keep platform-specific behavior in the CLI and Discord adapters and preserve the shared messaging interface.

## Verification and handoff

- For a behavior change, start with a failing test, make it pass with the smallest change, and then refactor.
- Run `make test` and any relevant configured checks for code changes.
- Use `make start` with `DISCORD_TOKEN` empty to exercise the CLI. For session or memory changes, check the resulting state under `.storage/`.
- Exercise the real entry point after meaningful behavior changes and before opening or updating the pull request. Check the output and relevant side effects.
- Mock external systems at their boundaries. Confirm what test or dry-run modes skip, preserve the coverage of failing checks, and report any verification gaps.
- Before opening a pull request, get an independent review of correctness and security when a fresh reviewer is available.
