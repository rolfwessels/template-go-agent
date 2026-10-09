# Go Agent Template

Domain vocabulary for the hybrid AI agent template. See the [README](README.md) for setup and [ADRs](docs/adr/) for decision history.

## Language

Use these terms consistently; the last column lists ambiguous or misleading substitutes.

| Term | Meaning | Avoid |
|---|---|---|
| **Agent** | Running instance scoped to one user; receives messages, reasons with an LLM, executes Tools, and responds. | Bot, assistant, service |
| **AgentPool** | Owns the `user_id → Agent` map, creates on first contact, routes messages, evicts on inactivity, and triggers Memory Sweeps on eviction/shutdown. | Agent manager, session manager, router |
| **Session** | Persistent per-user conversation; ends on explicit reset via `new_session`, survives restarts/inactivity. ID is a zero-padded Unix nanosecond timestamp; latest session file is current. | Thread, conversation |
| **Session Log** | Durable append-only JSONL record of Session messages; source for rebuilding Conversation History. | Chat log, message history, session store |
| **Conversation History** | Recent Session Log messages reloaded on Agent creation/recreation (default 20), then extended during Agent turns and fed to the LLM. | Context, full history, message log |
| **Storage Layout** | Per-user assets under `.storage/user/{user_id}/`; app logs at `.storage/logs/app.log`. | Flat per-asset directories at storage root |
| **Cost Ledger** | Append-only `metrics/costs.jsonl` with one Usage Record per tracked LLM completion. | Usage log, token log, billing log |
| **Usage Tracker** | `internal/usage` component reading `ResponseMeta.Usage`, estimating cost, writing the Cost Ledger, logging usage, and updating console counters. | Token tracker, billing tracker, usage logger |
| **Usage Record** | One tracked Agent or Memory Distiller LLM call completion. | Token record, cost entry |
| **Long-term Memory** | Facts, preferences, and outcomes distilled from Session Logs; persists across Sessions as Markdown files. | Unqualified memory, knowledge base |
| **Memory Distiller** | One combined extraction/classification/summary operation per date per Memory Sweep, using existing daily content and new messages. | Ambiguous distiller, summarizer |
| **Memory Sweep** | Distills unprocessed Session Log messages on inactivity, explicit reset, or shutdown; reset sweeps before starting a new Session. | Summarization, flush, persist |
| **Sweep Cursor** | Line count in a Session's `.swept_until` file; selects new messages and advances only after facts/index are persisted. Failed sweeps leave the range available for retry. | Offset, pointer, checkpoint |
| **Platform Adapter** | Implements `MessagePlatform`: `Connect`, `SendMessage`, `ReceiveMessages`, `Disconnect`; bridges CLI/Discord to the agent loop. | Connector, transport, integration |
| **Transcriber** | `Transcribe(ctx, audioURL) (string, error)` dependency injected into Discord to convert audio attachments before agent processing; default uses OpenAI Whisper, nil disables it. | Speech-to-text, STT, voice converter |
| **Tool** | Typed callable capability registered with the Agent and invoked by the LLM within a ReAct turn. | Function, plugin, skill |
| **Schedule** | Per-user job with ID, prompt, delivery `channel_id`, `next_fire_at` Unix timestamp, and optional `interval_seconds`; no interval means one-shot. | Reminder in code, cron job, timer |
| **Scheduler** | Loads Schedule Files at startup; owns runtime state in memory, ticks every 60 seconds, sends due prompts through AgentPool/Platform Adapter, and advances/removes jobs. | Cron, job runner, task queue |
| **Schedule File** | Per-user `schedule/schedule.json`, written by Scheduler on add/cancel and after firing; read at startup. | Schedule store, schedule log |

## Storage and Long-term Memory

All paths below are relative to `.storage/user/{user_id}/`:

| Path | Contents / loading |
|---|---|
| `sessions/{session_id}.jsonl` | Session Log; new lines have schema version `v:1`, legacy unversioned lines remain readable. |
| `sessions/{session_id}.swept_until` | Sweep Cursor counting physical JSONL lines. |
| `memory/MEMORY.md` | Navigable index with daily summaries; loaded into the system prompt. |
| `memory/general.md` | Stable facts; always loaded into the system prompt. |
| `memory/daily/{date}.md` | Full facts for the date of the source messages; read on demand via `read_memory_file`. |
| `schedule/schedule.json` | Durable Schedule File. |
| `metrics/costs.jsonl` | Cost Ledger. |

The Memory Distiller classifies facts as `[general]` or untagged. Every fact goes into the appropriate daily file; `[general]` facts also go into `general.md`. Its interface is `DistillAndSummarize(ctx, existingDaily string, messages) (facts, summary, err)`; the summary covers all facts in that daily file.
`search_memory` performs keyword lookup across memory files; no vector store is required. User attribution comes from storage paths; sweep/usage operations also carry Session IDs.

## Usage tracking

Usage Records contain `timestamp`, `component` (`agent` or `distiller`), `model`, `prompt_tokens`, `completion_tokens`, `total_tokens`, `cost_usd`, and `session_id`.
`WithUsageTracker` binds user/Session IDs to the Agent. `WithDistillerTracker` injects tracking into the Memory Distiller; the Sweeper supplies those IDs in context.
Costs use a hardcoded model pricing table; unknown models record $0 with a warning. Atomic console counters cover the process lifetime, while the Cost Ledger persists across restarts.

## Relationships and lifecycle

- Each user has one current Session and one Agent at a time; a Session has one Session Log.
- Conversation History is the in-context view of that log, distinct from Long-term Memory.
- An Agent executes zero or more Tools per user turn before returning a response.
- Inactivity evicts the Agent and sweeps memory; the next message recreates it with history from the same Session.
- Restart preserves the Session; explicit reset sweeps and starts a new Session.
- Graceful shutdown on SIGTERM/SIGINT is load-bearing: it must trigger a Memory Sweep. Abrupt termination leaves logged messages awaiting later distillation.

## Technology defaults and scope

OpenAI is accessed through Eino; the runtime model defaults to `gpt-5.5`. Tavily is the implemented web-search provider. [Configuration](docs/configuration.md) covers environment values and embedded/runtime prompt loading; tool descriptions live in code.
CLI and Discord ship today. Platform-specific dependencies such as a Transcriber are injected through constructors, keeping the core messaging interface stable.
See the [roadmap](docs/roadmap.md) for tracing, retry/backoff, Topic Distiller, and additional adapters.
Always qualify “memory” as Conversation History, Session Log, or Long-term Memory; avoid “session history,” which conflates the durable log with the in-context window.
