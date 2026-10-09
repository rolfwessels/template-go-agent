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

From the repository root:

```bash
cp .env.example .env
$EDITOR .env
make up
```

Set `OPENAI_API_KEY` and `TAVILY_API_KEY` in `.env` before continuing.
`make up` builds/starts the dev container and attaches its shell. Inside that shell:

```bash
make test
make start
```

Type a question and press Enter; `Ctrl+C` quits and triggers a Memory Sweep.
Set `DISCORD_TOKEN` to use Discord instead of the CLI, then run `make start`.

Prefer running locally? With Go 1.26 installed, `make test` and `make start` also work from the repository root.
Released binaries for Linux, macOS, and Windows are on the [releases page](https://github.com/rolfwessels/template-go-agent/releases); run `template-go-agent` (`template-go-agent.exe` on Windows) from a directory containing `.env`, or supply environment variables.

State and logs live in `.storage/` under the working directory; keep it to preserve sessions, memory, reminders, and usage records.
On the host, `make down` stops the dev container.

## 🗂 Docs

- [Configuration](docs/configuration.md) — environment variables and prompt overrides.
- [HTTP fetch security](docs/http-fetch-security.md) — public research policy and limits.
- [Development](docs/development.md) — make commands, versions, CI, and Docker publishing.
- [Roadmap](docs/roadmap.md) — remaining gaps and possible extensions.
- [Domain context](CONTEXT.md) — vocabulary, storage, and lifecycle.
- [Architecture decisions](docs/adr/) — decisions and their trade-offs.
