# Eino as the agent framework

We chose Eino (github.com/cloudwego/eino) as the core LLM application framework over LangChainGo. Eino was designed from scratch for Go — it uses Go's type system and concurrency primitives rather than translating Python patterns. It was also battle-tested as an internal ByteDance project before being open-sourced, which gives it more production credibility than its community size alone would suggest.

## Considered Options

- **LangChainGo**: more widely known in the Western Go ecosystem, more community examples — rejected because it is a port from Python LangChain and shows it: heavy use of `map[string]any`, interface patterns that don't feel idiomatic in Go.
- **Roll our own**: maximum control — rejected because the agent loop, tool dispatch, and streaming plumbing are non-trivial to get right and not what this template is trying to solve.

## Consequences

Eino is less commonly used outside China, so community help and third-party examples are thinner. A spike (throwaway program wiring Eino's OpenAI provider to a Tavily tool through one ReAct turn) must pass before agent logic is built on top of it. If the spike fails, this ADR should be revisited.
