# ADR-0005: Scheduler uses in-memory state loaded from a per-user JSON file

## Status
Accepted

## Context
The Scheduler needs to track due Schedules across all users and fire them at the right time. Two approaches were considered:

1. **Poll from disk each tick** — every 60 seconds read each user's `schedule.json` and check for due entries
2. **In-memory + file backing** — load all Schedules from `schedule.json` on startup; keep them in memory as the source of truth; write to file on every mutation; goroutine ticks against memory only

## Decision
In-memory state loaded from disk on startup, with `schedule.json` as the durable backing store.

## Rationale
- Only the Scheduler writes to `schedule.json`; there is no concurrent writer to coordinate with
- Reading disk on every 60-second tick adds I/O with no benefit when memory is already authoritative
- The file is written on every mutation (add/cancel), so restarts always recover full state
- This mirrors how the AgentPool manages agents — ephemeral in-memory instances, durable backing store for recovery

## Consequences
- The Scheduler must be initialised before the agent loop starts (to hydrate from disk)
- A crash between a mutation and the file write could lose one Schedule; acceptable given the use case (reminders, not financial transactions)
