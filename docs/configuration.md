# 🔑 Configuration

The app loads `.env` from the working directory; existing environment variables take precedence.
Defaults below come from [config.go](../internal/config/config.go); empty means unset unless stated otherwise.

| Name | Default | Purpose |
|---|---|---|
| `OPENAI_API_KEY` | empty; required | OpenAI model and Discord Whisper credentials. |
| `OPENAI_MODEL` | `gpt-5.5` | Agent and Memory Distiller model. |
| `OPENAI_REASONING_EFFORT` | `none` | Agent model reasoning effort, passed to the OpenAI provider. |
| `TAVILY_API_KEY` | empty; required | Tavily `web_search` credentials. |
| `DISCORD_TOKEN` | empty | Select Discord when set; otherwise use CLI. |
| `PROMPTS_DIR` | empty | Directory containing both prompt override files. |
| `SESSION_TIMEOUT_MINUTES` | `30` | Inactivity before AgentPool eviction and Memory Sweep; Session persists. |
| `CONVERSATION_HISTORY_WINDOW_SIZE` | `20` | Recent Session Log messages loaded when an Agent is recreated. |
| `HTTP_FETCH_ENABLED` | `true` | Register the `http_fetch` research tool. |
| `HTTP_FETCH_ALLOWED_HOSTS` | empty | Permit public hosts; a nonempty list restricts destinations. |
| `HTTP_FETCH_DENIED_HOSTS` | empty | Additional host denials; takes precedence over allowlists. |
| `HTTP_FETCH_ALLOWED_PORTS` | `80,443` | Allowed ports; HTTP on 443 and HTTPS on 80 remain blocked. |
| `HTTP_FETCH_ALLOW_MUTATIONS` | `false` | Enable POST/PUT/PATCH/DELETE; requires mutation hosts. |
| `HTTP_FETCH_MUTATION_ALLOWED_HOSTS` | empty | Required destination allowlist for enabled mutations. |
| `HTTP_FETCH_MAX_TIMEOUT_MS` | `30000` | Total request deadline; caller may shorten it. |
| `HTTP_FETCH_MAX_RESPONSE_BYTES` | `524288` | Response limit after gzip decoding and again after charset decoding. |
| `HTTP_FETCH_MAX_TEXT_BYTES` | `65536` | Returned readable text limit in UTF-8 bytes. |
| `HTTP_FETCH_MAX_REDIRECTS` | `5` | Read redirect limit; zero disables redirects. |

Both API keys are required at startup. `OPENAI_REASONING_EFFORT` is absent from [.env.example](../.env.example) but supported by the loader; `none` is the code default for tool compatibility. The loader does not validate effort values; support depends on the provider/model.
Empty model/effort values use defaults. Empty or malformed session/history integers also fall back to defaults; these two settings have no range validation.

## HTTP policy settings

Host lists are comma-separated ASCII DNS names (punycode for international names) or canonical IP literals. Names are lowercased and one trailing dot is removed. `*.example.com` matches all subdomains, excluding the apex; list `example.com` separately when needed.
URLs, credentials, embedded ports, IP zones, malformed wildcards, and ambiguous integer/octal/hex IP notation are invalid host entries.
HTTP booleans must be exactly `true` or `false`; numeric limits use canonical decimal integers and must be positive, except redirects may be zero. Ports must be 1–65535; empty host lists are valid, empty port lists are not.
Malformed or explicitly empty HTTP booleans/numbers fail startup even when the tool is disabled. Mandatory address blocks cannot be overridden; see [HTTP fetch security](http-fetch-security.md).

```dotenv
HTTP_FETCH_ALLOWED_HOSTS=example.com,*.example.com
HTTP_FETCH_DENIED_HOSTS=admin.example.com
```

## Prompt overrides

[soul.md](../prompts/soul.md) sets persona; [instructions.md](../prompts/instructions.md) sets tool usage and guardrails. Both are embedded at build time, so runtime images and release archives need only the binary.
Edit those files and rebuild to change defaults. For runtime overrides, set `PROMPTS_DIR` to a directory containing edited copies of **both** files.
Unset/empty `PROMPTS_DIR`, or neither file present, uses embedded defaults. A partial pair or unreadable file fails Agent construction. Relative paths resolve from the working directory; use an absolute path to avoid surprises.
