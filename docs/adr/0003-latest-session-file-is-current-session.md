# Latest session file is the current session

Session identity must survive application restarts without a separate pointer file. We chose to derive the current Session implicitly: the lexicographically latest JSONL file in the user's session directory is always the active Session. Session IDs are zero-padded Unix nanosecond timestamps, making filename sort order reliable. A new Session (explicit reset) is created by writing a new file with a later timestamp; no pointer needs updating.

## Considered Options

- **Pointer file (`session.json` per user)**: explicit, O(1) lookup — rejected because it introduces a second source of truth that can diverge from the actual session files, and offers no meaningful performance benefit at this scale.
