# 🌐 template-go-agent

[![GitHub release](https://img.shields.io/github/v/release/rolfwessels/template-go-agent)](https://github.com/rolfwessels/template-go-agent/releases)
[![Go CI](https://github.com/rolfwessels/template-go-agent/actions/workflows/github-action.yml/badge.svg)](https://github.com/rolfwessels/template-go-agent/actions)

A Go template for developers building hybrid AI agents — conversational at the surface, with autonomous multi-step tool execution within a single user turn. Powered by [Eino](https://github.com/cloudwego/eino) (ReAct loop), OpenAI, and Tavily web search, with CLI and Discord adapters out of the box.

## ✨ How it works

- A platform adapter receives your message; a per-user agent reasons, calls tools, and replies.
- Tools cover web research, time/date math, calculation, session resets, reminders, and memory lookup.
- Session Logs survive restarts; Memory Sweeps distill useful facts into Markdown Long-term Memory.
- A file-backed scheduler delivers one-shot and recurring reminders through the same agent.
- Persona and instructions come from embedded [prompt files](prompts/); edit and rebuild, or set `PROMPTS_DIR` for runtime overrides.

## 🛠 Prerequisites

Docker with Compose, Git, and `make` (WSL on Windows). The dev container supplies Go and build tools.
You’ll also need OpenAI and Tavily API keys; Discord is optional.

## 🚀 Quick start

From the repository root, copy the sample configuration.

```sh
cp .env.example .env
```

Set `OPENAI_API_KEY` and `TAVILY_API_KEY` in `.env`. Leave `DISCORD_TOKEN` empty for CLI chat.

Run `make up` on the host to open the development shell, then start the agent in that shell.

```sh
make start
```

Type a question and press Enter. The agent replies in the same terminal. Press Ctrl+C to quit and trigger a Memory Sweep.

With Go 1.26.9 installed locally, you can run `make start` from the repository root. To use Discord, set `DISCORD_TOKEN` before starting the agent.

## 📋 Make commands

| Command | Purpose |
| --- | --- |
| `make up` | Start the development container and open its shell. |
| `make start` | Start the agent in CLI or Discord mode. |
| `make test` | Run Go vet and the tests. |
| `make down` | Stop the development container from the host. |

Run `make help` for more targets.

## 📦 Releases and deployment

Download an archive for Linux, macOS, or Windows from the [releases page](https://github.com/rolfwessels/template-go-agent/releases). Run the extracted binary from a directory containing `.env` or supply the environment variables directly.

Keep `.storage/` in the working directory to preserve sessions, memory, reminders, and usage records between runs.

## 🗂 Docs

- [Configuration](docs/configuration.md) — environment variables and prompt overrides.
- [HTTP fetch security](docs/http-fetch-security.md) — public research policy and limits.
- [Development](docs/development.md) — make commands, versions, CI, and Docker publishing.
- [Agent instructions](AGENTS.md) — project conventions and verification.
- [Roadmap](docs/roadmap.md) — remaining gaps and possible extensions.
- [Domain context](CONTEXT.md) — vocabulary, storage, and lifecycle.
- [Architecture decisions](docs/adr/) — decisions and their trade-offs.
