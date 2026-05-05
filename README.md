# 🌐 template-go-agent

[![GitHub release](https://img.shields.io/github/v/release/rolfwessels/template-go-agent)](https://github.com/rolfwessels/template-go-agent/releases)
[![Go CI](https://github.com/rolfwessels/template-go-agent/actions/workflows/github-action.yml/badge.svg)](https://github.com/rolfwessels/template-go-agent/actions)

A Go template for hybrid AI agents — conversational at the surface, with autonomous multi-step tool execution within a single user turn. Powered by [Eino](https://github.com/cloudwego/eino) (ReAct loop), OpenAI, and Tavily web search.

## ✨ How it works

The agent reads a question from stdin, runs a ReAct reasoning loop (calling web search via Tavily as needed), and prints a grounded answer.

```bash
# copy and populate env vars
cp .env.example .env
$EDITOR .env

# start the agent (inside the dev container)
make start
```

Type a question and press Enter. `Ctrl+C` to quit.

## 📦 Technology

- [Eino](https://github.com/cloudwego/eino) — ReAct agent loop and OpenAI provider
- [Tavily](https://tavily.com) — web search tool
- [chromem-go](https://github.com/philippgille/chromem-go) — embedded vector store for long-term memory (upcoming)
- [Ollama](https://ollama.com) — local embeddings via docker-compose
- Docker for the dev environment
- MakeFile because it just works!

## 🔑 Environment variables

Copy `.env.example` to `.env` and fill in:

| Variable | Required | Default | Description |
|---|---|---|---|
| `OPENAI_API_KEY` | ✅ | — | OpenAI API key |
| `OPENAI_MODEL` | | `gpt-5.5` | Model to use |
| `TAVILY_API_KEY` | ✅ | — | Tavily search API key |
| `OLLAMA_BASE_URL` | | `http://localhost:11434` | Ollama endpoint |
| `SESSION_TIMEOUT_MINUTES` | | `30` | Inactivity timeout |

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

## FAQ

**Can I use this on Windows/macOS/Linux?**  
Yes — binaries are published for all three platforms.

**How do I update to the latest version?**  
Re-run the install command (`install.sh` or `install.ps1`). It overwrites the binary in place from the latest release.

## Research

- [What is a Makefile?](https://opensource.com/article/18/8/what-how-makefile)
