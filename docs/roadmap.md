# 📝 Roadmap

Possible follow-up work, not shipped configuration or promised scope:

| Gap / extension | Value |
|---|---|
| Conversation tracing | Trace IDs across agent turns, tools, and distillation; evaluate OpenTelemetry or Langfuse. No tracing integration is present. |
| LLM retry/backoff | Add explicit handling for transient 429/5xx failures in agent and distiller calls. |
| Topic Distiller | Cluster stable facts from `general.md` into `topics/{name}.md` and feed topics back to distillation. |
| GraphQL / Slack adapters | Additional messaging surfaces after CLI and Discord. |

Streaming output was deliberately deferred: Discord would need debounced message edits and changes to the platform interface. Current delivery uses a typing indicator and complete responses.
See [domain context](../CONTEXT.md) and [architecture decisions](adr/) before extending lifecycle, memory, or scheduling.
