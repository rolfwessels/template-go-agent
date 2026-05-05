# Inactivity timeout triggers Memory Sweep, not Session end

The inactivity timer evicts the Agent from the AgentPool (freeing memory) and triggers a Memory Sweep, but does not end the Session. The Session remains the same; the next message recreates the Agent and reloads Conversation History from the Session Log. A Session ends only on explicit reset (tool call or `--clean-session` flag).

Previously, inactivity ended the Session, causing a new session file per restart and stranding Conversation History — users lost context after every inactivity period or application restart.

## Consequences

The Memory Sweep now runs multiple times within a single Session. A sweep cursor (implementation detail, not a domain concept) tracks the last swept position in the Session Log so each sweep processes only new messages.
