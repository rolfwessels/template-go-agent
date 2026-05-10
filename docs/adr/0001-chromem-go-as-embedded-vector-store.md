# chromem-go as the embedded default vector store

**Status: Superseded** — the vector store was removed entirely. See commit `c2c694c` (Remove vector store and Ollama embedding dependency). Long-term memory is now stored as plain Markdown files (`general.md` + `daily/{date}.md`) with keyword-based search via `search_memory` tool, replacing semantic retrieval.

---

Long-term Memory requires a vector store for semantic retrieval. We chose chromem-go as the default embedded store because it is pure Go (no CGO, no external server), has zero third-party dependencies, and persists to gob/gzip files that sit alongside the Markdown memory files — keeping the storage format portable and human-inspectable.

## Considered Options

- **sqlite-vec (viant)**: pure Go, Apache-2.0, SQL-queryable — rejected because SQL is a heavier API for pure vector workloads and chromem-go's simpler API better fits the template abstraction layer.
- **Milvus Lite**: official Eino support, exact parity with the Milvus server — rejected because the Go variant spawns a subprocess binary (~25–55 MB per platform), introducing server-like complexity that defeats the "zero infra" goal.
- **LanceDB**: production-tested, cloud storage support — rejected due to CGO (Rust core) requirement.

## Consequences

chromem-go is MPL-2.0 licensed. This is acceptable for application use but would be a concern if the vector store code were distributed as a library. The `VectorStore` interface isolates this dependency; projects that require a more permissive license can swap in sqlite-vec or Milvus without touching agent logic.
