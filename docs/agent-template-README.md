# Go Agent Template

A reusable Go-lang template repository for spinning up new AI agent projects without rebuilding the same plumbing every time.

## What This Is

An opinionated starting point for building agents in Go. Clone it, configure it, and you have a working agent with web search, memory, and a chat interface ready to go. Every new project starts from the same solid foundation rather than reinventing the scaffolding each time.

## Why It Exists

Every agent project tends to need the same building blocks: an LLM client, a tool registry, somewhere to store memories, and a way for users to actually talk to it. Wiring those together from scratch is repetitive and error-prone. This template codifies those decisions once so future projects can focus on what makes them unique.

## Core Stack

### Framework: Eino

Eino is a Go-native LLM application framework from ByteDance's CloudWeGo team. We chose it over LangChainGo for a few reasons:

- **Go-first design** — built around Go conventions rather than ported from Python
- **Strongly typed components** — inputs and outputs are validated at compile time, no `map[string]any` with type assertions everywhere
- **Production-oriented** — designed for high-throughput, reliable AI applications
- **Composable** — chains, graphs, and agents as first-class building blocks

### Web Search

Plugged in through Eino's tool abstraction. The template will support a pluggable provider (Brave, SerpAPI, or Tavily) so you can swap based on your needs.

### Memory

A hybrid system that gets the best of both worlds:

- **Markdown files** for portability, human readability, and easy export
- **Milvus vector store** layered on top for semantic retrieval
- **Unified store** with metadata tagging — every memory entry is tagged with `user_id` and `session_id`
- **Single index, filtered queries** — query by user, by session, or both, without managing separate stores

This keeps infrastructure simple while giving you flexibility to scope memories however the use case demands.

### Observability: Langfuse

Agents are notoriously hard to debug without good tracing. Langfuse gives us prompt-level visibility into every LLM call, tool invocation, and chain step — which is essential when something goes wrong in a multi-step agent run.

Eino has a first-class Langfuse callback handler in `eino-ext`, so wiring it in is a matter of registering the handler with the agent. We get:

- **Trace inspection** — full request/response history per session
- **Cost and token tracking** — across providers
- **Latency breakdowns** — see which tool or model call is the bottleneck
- **Prompt iteration** — compare versions and outputs over time

### Chat Platform Integration

Eino deliberately doesn't prescribe a chat layer, which keeps that boundary clean. The template defines its own adapter interface so the agent logic stays platform-agnostic:

```go
type MessagePlatform interface {
    Connect(ctx context.Context) error
    SendMessage(ctx context.Context, msg Message) error
    ReceiveMessages(ctx context.Context) (<-chan Message, error)
    Disconnect() error
}
```

Initial adapters:

- **Discord** — via DiscordGo
- **GraphQL endpoint** — for streaming to web clients
- **Slack** — planned

Adding a new platform means writing one adapter, not touching the agent code.

## Architecture

```
┌─────────────────────────────────────────────┐
│  Chat Platforms (Discord, GraphQL, Slack)   │
└──────────────────┬──────────────────────────┘
                   │
         ┌─────────▼──────────┐
         │  Adapter Interface │
         └─────────┬──────────┘
                   │
         ┌─────────▼──────────┐         ┌──────────────┐
         │   Eino Agent Core  │────────▶│   Langfuse   │
         │  (LLM + Tools)     │  traces │ Observability│
         └────┬───────────┬───┘         └──────────────┘
              │           │
       ┌──────▼───┐   ┌───▼──────────┐
       │  Tools   │   │   Memory     │
       │  - Web   │   │  - Markdown  │
       │    Search│   │  - Milvus    │
       └──────────┘   └──────────────┘
```

## What It Solves

- **Setup friction** — no more rebuilding the same boilerplate per project
- **Type safety** — idiomatic Go throughout, no runtime surprises from generic maps
- **Platform agnosticism** — agent logic doesn't care whether it's driven by Discord, a web app, or something not yet imagined
- **Memory portability** — markdown-first storage means memories aren't locked into a vector database
- **Debuggability** — Langfuse tracing baked in from day one, so agent behaviour is observable rather than opaque
- **Consistent foundation** — every project built from this template has the same shape, making maintenance and knowledge transfer easier

## Status

Template under design. Reference architecture and adapter interfaces being defined.
