# Go Agent Template

A reusable Go template for building hybrid AI agents — conversational at the surface, with autonomous multi-step tool execution within a single user turn.

## Language

**Agent**:
A running instance of the template scoped to a single user — receives messages, reasons over them using an LLM, executes tools as needed, and responds.
_Avoid_: Bot, assistant, service

**AgentPool**:
The component that owns the `user_id → Agent` map, manages Agent lifecycle (create on first contact, evict on inactivity), and routes incoming messages to the correct Agent. Triggers Memory Sweep on eviction and shutdown.
_Avoid_: Agent manager, session manager, router

**Session**:
A persistent conversation thread between a user and the agent. Created on first contact; ends only on explicit reset (via tool call or `--clean-session` flag at startup). Survives application restarts and inactivity periods. Identified by a zero-padded Unix nanosecond timestamp; the latest session file for a user is always the current Session.
_Avoid_: Thread, conversation

**Session Log**:
The durable, append-only record of every message in a Session, written to disk as a JSONL file. Survives application restarts. Serves as the source from which Conversation History is reconstructed.
_Avoid_: Chat log, message history, session store

**Conversation History**:
The rolling window of recent messages loaded from the Session Log into the Agent's context when the Agent is created or recreated. Configurable count (default: last 20 messages). Fed into the LLM's context window each turn.
_Avoid_: Context, full history, message log

**Storage Layout**:
All per-user assets live under `.storage/user/{user_id}/`: `memory/` for Long-term Memory files, `sessions/` for Session Logs and Sweep Cursors, `schedule/` for the Schedule File, and `metrics/` for the Cost Ledger. Application logs go to `.storage/logs/app.log`.
_Avoid_: Flat per-asset directories at the storage root

**Cost Ledger**:
An append-only JSONL file at `.storage/user/{user_id}/metrics/costs.jsonl` that records one entry per LLM call completion. Each entry contains: `timestamp`, `component` (`agent` or `distiller`), `model`, `prompt_tokens`, `completion_tokens`, `total_tokens`, `cost_usd`, and `session_id`. Written by the Usage Tracker after each LLM call by reading `ResponseMeta.Usage` on the returned message.
_Avoid_: Usage log, token log, billing log

**Usage Tracker**:
The component (`internal/usage`) that records LLM call completions by reading `ResponseMeta.Usage` on returned messages, calculates cost using a hardcoded pricing table, writes entries to the Cost Ledger, logs to slog, and maintains process-lifetime atomic token and cost counters for the console status line. Injected into the Agent via `WithUsageTracker` (closing over `userID` and `sessionID`) and into the Memory Distiller via `WithDistillerTracker`; the Sweeper injects `userID`/`sessionID` into the context before calling the Distiller so cost entries are attributed correctly. Unknown model names are priced at $0 with a warning logged.
_Avoid_: Token tracker, billing tracker, usage logger

**Usage Record**:
A single entry in the Cost Ledger. Represents one LLM call completion — either an Agent turn or a Memory Distiller invocation.
_Avoid_: Token record, cost entry

**Long-term Memory**:
Distilled facts, preferences, or outcomes extracted from the Session Log by the Memory Sweep. Survives across Sessions. Stored under `memory/` as three file types: `MEMORY.md` (index, always loaded into the system prompt), `general.md` (stable facts, always loaded), and `daily/{date}.md` (all facts recorded on that date, browsed on demand via `read_memory_file`). The Distiller classifies each fact as `[general]` or untagged; **every fact is written to the daily file for the date of the messages it was extracted from**; `[general]` facts are additionally written to `general.md`. This layered model means daily files hold full detail, `MEMORY.md` holds navigable summaries, and `general.md` holds stable cross-day patterns that are always in context.
_Avoid_: Memory (unqualified), knowledge base

**Memory Distiller**:
The LLM component responsible for a single combined operation per date per Memory Sweep: given the existing content of `daily/{date}.md` and the new messages for that date, it extracts new facts not already recorded, classifies each as `[general]` or untagged, and produces one updated summary sentence covering all facts in the daily file. Interface: `DistillAndSummarize(ctx, existingDaily string, messages) (facts, summary, err)`.
_Avoid_: Distiller (unqualified when the operation is ambiguous), summarizer

**Memory Sweep**:
The summarization pass that reads unprocessed messages from the Session Log and distills what is worth keeping into Long-term Memory. Triggered by: inactivity timeout, explicit session reset (tool call), or application shutdown (SIGTERM/SIGINT). Processes only messages since the last sweep (tracked via the Sweep Cursor); on explicit reset it sweeps before starting the new Session.
_Avoid_: Summarization, flush, persist

**Sweep Cursor**:
A line-count integer stored in a `.swept_until` file alongside each Session Log. Records how many lines of the Session Log have already been distilled. The Memory Sweep reads only from this offset to end-of-file and advances the cursor only after a successful distillation write. Prevents re-distilling already-processed messages when the sweep fires multiple times within a long-lived Session.
_Avoid_: Offset, pointer, checkpoint

**Platform Adapter**:
Implements the `MessagePlatform` interface (`Connect`, `SendMessage`, `ReceiveMessages`, `Disconnect`). Bridges an external messaging surface (CLI, Discord, etc.) to the agent loop. Swapping adapters requires no changes to agent logic. Adapters may accept supplementary dependencies (e.g. a `Transcriber` for audio-to-text) that are injected at construction time via the constructor, keeping the core interface stable.
_Avoid_: Connector, transport, integration

**Transcriber**:
An interface (`Transcribe(ctx, audioURL) (string, error)`) consumed by the Discord Platform Adapter to convert audio attachments into text before the message enters the agent pipeline. The default implementation calls OpenAI's Whisper API. Injected into the adapter at construction time; nil disables transcription.
_Avoid_: Speech-to-text, STT, voice converter

**Tool**:
A typed, callable capability registered with the agent (e.g. web search). Invoked by the LLM during the ReAct loop within a single turn.
_Avoid_: Function, plugin, skill

**Schedule**:
A single scheduled job entry owned by a user. Contains a prompt to inject, a `next_fire_at` Unix timestamp, an optional `interval_seconds` for recurrence, a `channel_id` for delivery, and a unique ID. One-shot Schedules have no `interval_seconds`; recurring Schedules advance `next_fire_at` by `interval_seconds` after each firing.
_Avoid_: Reminder (in code), cron job, timer

**Scheduler**:
The component that manages all Schedules across all users. Loaded from each user's Schedule File on startup; kept in memory as the source of truth at runtime. A background goroutine ticks every 60 seconds, finds due Schedules, injects their prompt into the AgentPool, delivers the response via the Platform Adapter, and advances or removes the Schedule.
_Avoid_: Cron, job runner, task queue

**Schedule File**:
A per-user JSON file at `.storage/user/{user_id}/schedule/schedule.json` that durably stores all Schedules for that user. Written on every mutation (add or cancel). Read only on startup to hydrate the Scheduler's in-memory state. Nothing else writes to this file at runtime.
_Avoid_: Schedule store, schedule log

## Relationships

- A **Session** belongs to exactly one user; there is always exactly one active Session per user
- A **Session** has one **Session Log**; **Conversation History** is a bounded view over that log
- **Long-term Memory** entries are tagged with `user_id` and `session_id` for attribution
- An **Agent** executes zero or more **Tools** per user turn before producing a response
- Each user gets exactly one **Agent** instance at a time; the AgentPool evicts it on inactivity but recreates it on the next message, reloading Conversation History from the Session Log

## Example dialogue

> **Dev:** "Should we store every message the user sends in Long-term Memory?"
> **Domain expert:** "No — the Session Log is the raw record. Long-term Memory is written deliberately by the Memory Sweep, only distilling what's worth keeping."

> **Dev:** "If the app restarts, does the user lose their conversation?"
> **Domain expert:** "No — the Agent reloads the last 20 messages from the Session Log. The Session persists; only the in-memory Agent instance is recreated."

## Platform adapter delivery order

1. **CLI** — stdin/stdout, ships in MVP. Used for local dev and integration tests.
2. **Discord** — second adapter, demonstrates real platform integration.
3. **GraphQL** — stretch goal; enables web and mobile clients via a streaming endpoint.

## Known risks requiring validation

- **Eino spike required** (see ADR-0002): before building AgentPool or any agent logic, verify that Eino's OpenAI provider + Tavily tool + ReAct loop works end-to-end in a throwaway program. If the spike fails, the framework choice must be revisited.

## Stretch goals (not day-one scope)

- **Topic Distiller** — a periodic process that scans `general.md`, identifies fact clusters (e.g. "wines", "games"), promotes them into `topics/{name}.md` files, and feeds those topic files as context back to the Memory Distiller. Allows long-term memory to self-organise into labelled topics rather than a flat list. `general.md` acts as a staging area until a cluster is large enough to warrant its own topic.

- **Langfuse observability** — integration work done but activation deferred. When added, opt-in via `LANGFUSE_SECRET_KEY` env var; absent = no-op callback.
- **GraphQL adapter** — see platform delivery order above.
- **Slack adapter** — after Discord.

## Technology defaults

- **LLM provider**: OpenAI, model configurable (default: `gpt-5.5`). Accessed via Eino's OpenAI provider; model ID is a runtime config value, not hardcoded.
- **Agent prompt files**: `prompts/soul.md` (character, tone, identity) and `prompts/instructions.md` (tool usage, guardrails, workflow). Loaded at startup and composed into the system prompt. Tool descriptions live in code, not in these files.
- **Config**: `.env` file for local dev, env vars in production. `.env.example` committed to the repo documents every required variable. Secrets never go in config files.
- **Web search**: Tavily (default). Provider is a config value; Brave is the documented alternative.

## Key operational constraints

- **Graceful shutdown is load-bearing**: the Memory Sweep must be triggered on SIGTERM/SIGINT, not only on inactivity timeout. Running in Docker means the container may be stopped at any time; unswept Sessions lose their Long-term Memory distillation.

## Flagged ambiguities

- "memory" used without qualification could mean **Conversation History**, **Session Log**, or **Long-term Memory** — always qualify.
- "session history" is ambiguous — use **Session Log** for the full durable record, **Conversation History** for the in-context window.
