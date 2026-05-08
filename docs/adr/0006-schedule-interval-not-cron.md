# ADR-0006: Schedules use next_fire_at + interval_seconds, not cron expressions

## Status
Accepted

## Context
Recurring Schedules need a way to express "fire every N seconds/minutes/hours/days". Two approaches were considered:

1. **Cron expressions** (`0 9 * * *`) — industry-standard, precise day/weekday/month targeting, requires a cron parser dependency, LLM must generate valid cron syntax
2. **next_fire_at + interval_seconds** — the LLM calculates an absolute first fire time using existing `get_current_time` and `date_math` tools; the Scheduler advances `next_fire_at` by `interval_seconds` after each firing

## Decision
`next_fire_at` (Unix timestamp) plus an optional `interval_seconds` for recurrence.

## Rationale
- The agent already has `get_current_time` and `date_math` tools; it can compute "9am tomorrow" without a new dependency
- No cron parser library needed; no cron syntax errors from the LLM
- One-shot Schedules are a natural subset (omit `interval_seconds`)
- The use cases driving this feature ("remind me in a year", "daily news at 9am") don't require day-of-week or month-boundary precision
- "Every first Monday of the month" is out of scope; if needed, revisit with a cron library at that point

## Consequences
- Recurring Schedules drift slightly if the Scheduler tick fires late (e.g. under load); acceptable for reminder use cases
- "Every weekday" or "first of the month" patterns cannot be expressed; would require revisiting this decision
