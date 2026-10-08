# 🌐 template-go-agent

[![GitHub release](https://img.shields.io/github/v/release/rolfwessels/template-go-agent)](https://github.com/rolfwessels/template-go-agent/releases)
[![Go CI](https://github.com/rolfwessels/template-go-agent/actions/workflows/github-action.yml/badge.svg)](https://github.com/rolfwessels/template-go-agent/actions)

A Go template for hybrid AI agents — conversational at the surface, with autonomous multi-step tool execution within a single user turn. Powered by [Eino](https://github.com/cloudwego/eino) (ReAct loop), OpenAI, and Tavily web search. Supports CLI and Discord out of the box.

## ✨ How it works

The agent receives a message from a platform adapter (CLI or Discord), runs a ReAct reasoning loop (calling tools as needed), and sends back a grounded answer. Its persona and instructions come from `prompts/soul.md` and `prompts/instructions.md`, embedded into the binary at build time. Edit those files and rebuild to customise the defaults, or set `PROMPTS_DIR` to a directory containing edited copies of both files for runtime overrides.

Empty/unset `PROMPTS_DIR`, or an override directory with neither file present, uses the embedded defaults. Providing only one file, or a file that cannot be read, causes agent construction to fail. Use an absolute path for overrides to avoid depending on the working directory; relative override paths resolve from the working directory. Docker runtime images and release archives need only the binary and work without a `prompts/` folder.

Built-in tools: `web_search`, `http_fetch`, `get_current_time`, `date_math`, `calculate`, `new_session`, `schedule_reminder`, `list_reminders`, `cancel_reminder`, `read_memory_file`, `search_memory`.

```bash
# copy and populate env vars
cp .env.example .env
$EDITOR .env

# start the agent (inside the dev container)
make start
```

Type a question and press Enter. `Ctrl+C` to quit.

## 🗂 What's in place

| Module | Status | Notes |
|--------|--------|-------|
| Agent (Eino ReAct loop) | ✅ | Tools, pool, per-user history |
| Long-term memory | ✅ | Sweep + distill into daily/general files |
| Platform abstraction | ✅ | CLI and Discord adapters |
| Scheduler | ✅ | File-backed, background tick, LLM-controlled |
| Configuration | ✅ | Env-based with validation |
| Graceful shutdown | ✅ | SIGTERM triggers memory sweep pipeline |
| Voice transcription | ✅ | Discord voice notes via OpenAI Whisper |
| CI/CD + Docker | ✅ | Multi-stage image, GitHub Actions release pipeline |

## 📦 Technology

- [Eino](https://github.com/cloudwego/eino) — ReAct agent loop and OpenAI provider
- [Tavily](https://tavily.com) — web search tool
- Docker for the dev environment
- MakeFile because it just works!

## 🔑 Environment variables

Copy `.env.example` to `.env` and fill in:

| Variable | Required | Default | Description |
|---|---|---|---|
| `OPENAI_API_KEY` | ✅ | — | OpenAI API key |
| `OPENAI_MODEL` | | `gpt-5.5` | Model to use |
| `OPENAI_REASONING_EFFORT` | | `none` | Reasoning effort for the agent model (`none`, `low`, `medium`, `high`, `xhigh`) |
| `TAVILY_API_KEY` | ✅ | — | Tavily search API key |
| `PROMPTS_DIR` | | — | Optional directory containing both `soul.md` and `instructions.md`; empty/unset or neither file present uses embedded defaults |
| `SESSION_TIMEOUT_MINUTES` | | `30` | Inactivity timeout before agent eviction |
| `CONVERSATION_HISTORY_WINDOW_SIZE` | | `20` | Messages reloaded from Session Log on agent recreation |
| `DISCORD_TOKEN` | | — | Discord bot token. If set, Discord adapter is used instead of CLI |

## 🚀 Getting started with development

This project ships with a development container that has all the tooling required to build, test, and publish.

```bash
# bring up dev environment
make build up

# test the project
make test

# run the CLI
make start

# build release binaries for all platforms
make publish
```

To build and push a Docker image:

```bash
make docker-build docker-login docker-push
# or just
make docker-publish
```

## 🛠 Prerequisites

- [Docker](https://docs.docker.com/get-docker/) — for the dev container
- [Git](https://git-scm.com/) — for version control
- `make` — available via WSL on Windows, or natively on macOS/Linux

## 📋 Available make commands

### 💻 Commands outside the container

| Command      | Description                                          |
|--------------|------------------------------------------------------|
| `make up`    | Bring up the container & attach to the dev shell     |
| `make down`  | Stop the container                                   |
| `make build` | Rebuild the container                                |

### 🐳 Commands to run inside the container

| Command                      | Description                                    |
|------------------------------|------------------------------------------------|
| `make version`               | Show the current version                       |
| `make start`                 | Run template-go-agent                    |
| `make test`                  | Run tests                                      |
| `make publish`               | Build release archives for 5 platforms         |
| `make install`               | Symlink binary into ~/.local/bin               |
| `make uninstall`             | Remove the ~/.local/bin symlink                |
| `make docker-login`          | Login to Docker registry                       |
| `make docker-build`          | Build the production Docker image              |
| `make docker-push`           | Push the Docker image                          |
| `make docker-pull-short-tag` | Pull image by git short hash                   |
| `make docker-tag-env`        | Tag image for an environment                   |
| `make docker-publish`        | Full build + push workflow                     |
| `make deploy`                | Deploy template-go-agent                 |
| `make update-packages`       | Update Go dependencies to latest               |

## 💻 Development

### Versioning

This project follows [Semantic Versioning](https://semver.org/):

- **MAJOR**: Incompatible API changes
- **MINOR**: Backward-compatible new functionality
- **PATCH**: Backward-compatible bug fixes

`MAJOR` and `MINOR` are set manually via `versionPrefix` in the `Makefile`. `PATCH` is automatically derived from commit count.

```bash
make version
```

### Development Workflow

Feature branches are created off `main` with the prefix `feature/` or `bug/`.
PR builds attach archives as workflow artifacts. Merging to `main` publishes them as a new versioned [GitHub release](https://github.com/rolfwessels/template-go-agent/releases).

## 📝 To Do

### Gaps worth addressing

- [x] ~~**Streaming output**~~ — won't do. CLI streaming is trivial but Discord has no native streaming primitive; simulating it requires debounced message edits and a breaking change to the `MessagePlatform` interface. The typing indicator + complete response is the better UX for Discord anyway.
- [x] **Session log schema versioning** — `"v":1` added to all written JSONL lines; legacy lines (no `v`) still read cleanly
- [x] **Token/cost tracking** — no visibility into per-conversation or per-user token usage; essential at multi-user scale
- [ ] **Observability/tracing** — no trace IDs per conversation turn; Langfuse or OpenTelemetry would make debugging significantly easier
- [ ] **Langfuse observability** — mentioned in docs, not yet implemented; implement or remove the reference
- [ ] **Retry/backoff on LLM calls** — exponential backoff on 429/5xx prevents cascading failures when the upstream is degraded

## FAQ

**Can I use this on Windows/macOS/Linux?**  
Yes — binaries are published for all three platforms.

**How do I update to the latest version?**  
Re-run the install command (`install.sh` or `install.ps1`). It overwrites the binary in place from the latest release.

## Research

- [What is a Makefile?](https://opensource.com/article/18/8/what-how-makefile)
