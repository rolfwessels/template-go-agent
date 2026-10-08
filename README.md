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

## Public HTTP research

`http_fetch` remains enabled for reading public web pages with GET/HEAD. It uses its own transport with no environment proxy or cookie jar. Every new connection resolves all A/AAAA answers under the request deadline, rejects the entire answer set if any address is forbidden, and dials an approved IP literal. HTTP Host and TLS certificate verification retain the destination hostname. There is no private-network bypass in configuration; allowlists cannot override address blocking.

| Variable | Default | Meaning |
|---|---|---|
| `HTTP_FETCH_ENABLED` | `true` | Register the fetch tool |
| `HTTP_FETCH_ALLOWED_HOSTS` | empty | Public hosts permitted; otherwise restrict to listed hosts |
| `HTTP_FETCH_DENIED_HOSTS` | empty | Additional host denials, with precedence over allowlists |
| `HTTP_FETCH_ALLOWED_PORTS` | `80,443` | HTTP uses 80, HTTPS uses 443; additional ports must be explicitly listed |
| `HTTP_FETCH_ALLOW_MUTATIONS` | `false` | Enable POST/PUT/PATCH/DELETE only with mutation hosts listed |
| `HTTP_FETCH_MUTATION_ALLOWED_HOSTS` | empty | Required destination list when mutations are enabled |
| `HTTP_FETCH_MAX_TIMEOUT_MS` | `30000` | Total deadline for DNS, redirects, headers, and body; caller may shorten it |
| `HTTP_FETCH_MAX_RESPONSE_BYTES` | `524288` | Decoded response limit, including automatic gzip and charset expansion |
| `HTTP_FETCH_MAX_TEXT_BYTES` | `65536` | Returned readable text limit in UTF-8 bytes |
| `HTTP_FETCH_MAX_REDIRECTS` | `5` | Maximum followed redirects; zero disables them |

Host lists are comma-separated ASCII DNS names (use punycode for international names) or canonical IP literals. Names are lowercased and a single trailing dot is removed. Exact matching is the default; `*.example.com` matches subdomains, including deeper subdomains, but excludes `example.com`. URLs, credentials, embedded ports, malformed wildcard entries, zones, and ambiguous integer/octal/hex IP notation are rejected. Boolean values must be exactly `true` or `false`; integer limits must be positive, except redirects may be zero. Empty or malformed boolean/numeric settings fail startup, including when the tool is disabled. Empty host lists are valid. HTTP on 443 and HTTPS on 80 remain disallowed.

For example, to restrict research to a domain and its subdomains:

```dotenv
HTTP_FETCH_ALLOWED_HOSTS=example.com,*.example.com
HTTP_FETCH_DENIED_HOSTS=admin.example.com
```

Mutations are a deployment trust decision. An explicit example:

```dotenv
HTTP_FETCH_ALLOW_MUTATIONS=true
HTTP_FETCH_MUTATION_ALLOWED_HOSTS=api.example.com
```

The ordinary host/port policy still applies to mutations. CONNECT, TRACE, extension methods, credentials in URLs, Authorization, Cookie, API-key, Host, proxy, forwarding, metadata-token, and hop-by-hop headers are always rejected. Only caller `Accept` and `Accept-Language` are accepted; enabled mutations may also supply `Content-Type`. The User-Agent is fixed. GET/HEAD cannot have bodies. Mutation bodies are limited to 64 KiB and never follow redirects.

Read requests follow bounded public cross-host redirects with URL and DNS checks at each hop, reject HTTPS-to-HTTP downgrades, and reset headers to safe defaults on origin changes. Dial, TLS, and response-header waits are each capped at 10 seconds within the total deadline; response headers are limited to 64 KiB. Response headers are filtered to Content-Type, Content-Language, Last-Modified, and ETag.

Results retain `status`, `headers`, `body`, and `final_url`, and add `content_type` (parsed media type), `title`, and `truncated`. Both response and text limits detect overflow using limit-plus-one reads. HTML is parsed with `golang.org/x/net/html`, charset-decoded, and converted to readable headings, paragraphs, lists, and links. Scripts, styles, forms, navigation, common boilerplate, and hidden elements are removed; no JavaScript or subresources are fetched. Titles are capped at 1 KiB. Text/plain, Markdown, CSV, JSON (including `+json` types), XML, RSS, Atom, YAML, and XHTML are supported. An absent type is sniffed from at most 512 bytes. Binaries, SVG, images, and PDFs return status/metadata and an unsupported-type message. Use search snippets for unsupported or JavaScript-rendered pages.

All source text and metadata remain untrusted; prompt instructions tell the agent to ignore embedded commands. Logs contain tool name, normalized hostname, method, status, duration, and safe error codes, omitting raw arguments, URL paths/query strings, headers, bodies, and raw transport errors. Tavily uses a separate bounded 30-second context-aware client, a 512 KiB decoded response limit with an explicit overflow error, and no redirects; its credential is sent only in the request body to `https://api.tavily.com/search`. Its dedicated transport is cloned from `http.DefaultTransport`, uses normal DNS, and honours standard `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` environment variables. Tavily does not use the `http_fetch` destination policy. Non-2xx responses are errors, and search results remain marked as untrusted source material.

### Mandatory destination blocks

The conservative table in `internal/agent/http_fetch_policy.go` follows the [IANA IPv4](https://www.iana.org/assignments/iana-ipv4-special-registry/) and [IPv6 special-purpose registries](https://www.iana.org/assignments/iana-ipv6-special-registry/). Special-purpose exceptions are intentionally blocked even when globally reachable. IPv4-mapped IPv6 is normalized to IPv4 **before** checks, so mapped public addresses work and mapped private addresses fail.

| Category | Blocked ranges |
|---|---|
| IPv4 unspecified/current network, private, loopback | `0.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `127.0.0.0/8` |
| IPv4 link-local/metadata, CGNAT | `169.254.0.0/16`, `100.64.0.0/10` |
| IPv4 protocol/special services and transition | `192.0.0.0/24`, `192.31.196.0/24`, `192.52.193.0/24`, `192.88.99.0/24`, `192.175.48.0/24` |
| IPv4 documentation and benchmarking | `192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`, `198.18.0.0/15` |
| IPv4 multicast, reserved, broadcast | `224.0.0.0/4`, `240.0.0.0/4` (includes `255.255.255.255`) |
| Azure platform endpoint | `168.63.129.16/32` ([Azure documentation](https://learn.microsoft.com/en-us/azure/virtual-network/what-is-ip-address-168-63-129-16)) |
| IPv6 non-global, unspecified, loopback, local, multicast | Everything outside `2000::/3`; explicit entries include `::/96`, `fc00::/7`, `fe80::/10`, `fec0::/10`, `ff00::/8` |
| IPv6 translation/transition | `64:ff9b::/96`, `64:ff9b:1::/48`, `2002::/16`; mapped `::ffff:0:0/96` uses underlying IPv4 checks |
| IPv6 discard/dummy and reserved segment routing | `100::/64`, `100:0:0:1::/64`, `5f00::/16` |
| IPv6 protocol assignments, benchmark/ORCHID/Teredo and special services | `2001::/23`, `2620:4f:8000::/48` |
| IPv6 documentation | `2001:db8::/32`, `3fff::/20` |

Zones, invalid addresses, and non-global-unicast addresses are rejected. Single-label DNS names and localhost/local/internal/lan/home/test/invalid names and their subdomains are blocked, as are known metadata names (`metadata`, `metadata.google.internal`, `metadata.goog` and subdomains, `instance-data`, `instance-data.ec2.internal`, `metadata.azure.internal`). Address blocking also covers cloud metadata IP endpoints and Azure's platform IP.

Migration: existing uses of arbitrary methods, authenticated requests, custom headers, nondefault ports, or raw HTML must adapt to these restrictions. Use dedicated narrowly scoped tools for authenticated APIs. Public GET requests can still disclose data through URLs or trigger badly designed endpoints with side effects; these controls reduce SSRF exposure but do not eliminate prompt injection. Stronger deployments should enforce network egress rules too.

Tests use explicit unexported resolver/dialer hooks and socket-free local `httptest` listeners, with no live internet or API keys.

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
