# 🌐 HTTP fetch security

`http_fetch` is enabled by default for public research using GET/HEAD over HTTP:80 or HTTPS:443. It has a dedicated transport with no environment proxy or cookie jar. [Configuration](configuration.md) lists all policy settings.

## Destinations

Each new connection resolves all A/AAAA answers within the request deadline, rejects the entire set if **any** address is forbidden, and dials an approved IP literal. HTTP Host and TLS hostname verification keep the destination hostname. Allowlists never override mandatory blocks.

| Always blocked | Categories |
|---|---|
| Local/private destinations | Unspecified/current-network, loopback, private/unique-local, link-local, site-local, CGNAT. |
| Special-purpose addresses | Documentation, benchmarking, protocol assignments, special services, translation/transition, discard/dummy, reserved segment routing. |
| Non-public address space | Multicast, reserved/broadcast, non-global-unicast; IPv6 outside `2000::/3`. |
| Cloud infrastructure | Metadata endpoints and Azure platform IP `168.63.129.16`. |
| Local names | Single-label DNS; `localhost`, `local`, `internal`, `lan`, `home`, `test`, `invalid`, and their subdomains. |
| Metadata names | `metadata`, `metadata.goog` and subdomains, `metadata.google.internal`, `instance-data`, `instance-data.ec2.internal`, `metadata.azure.internal`. |
| Invalid/ambiguous destinations | IP zones, invalid hosts, noncanonical integer/octal/hex IP notation, URL credentials, schemes other than HTTP/HTTPS. |

The conservative [address table](../internal/agent/http_fetch_policy.go) blocks special-purpose exceptions even when globally reachable. IPv4-mapped IPv6 is normalized **before** checks: mapped public IPv4 works, mapped private IPv4 fails.
Host denials take precedence over host allowlists. Additional public ports must be explicitly listed; HTTP:443 and HTTPS:80 are always rejected.

## Requests, redirects, and limits

| Rule | Policy |
|---|---|
| Methods/bodies | GET/HEAD cannot have bodies. POST/PUT/PATCH/DELETE require `HTTP_FETCH_ALLOW_MUTATIONS=true` and mutation hosts; ordinary host/port policy still applies. Mutation bodies: at most 64 KiB. All other methods, including CONNECT/TRACE, are rejected. |
| Caller headers | Only `Accept`, `Accept-Language`; mutations may add `Content-Type`. Each value ≤8 KiB with no CR/LF/NUL. User-Agent is fixed. |
| Forbidden headers | All others, including Authorization, Cookie, API-key, Host, proxy/forwarding, metadata-token, and hop-by-hop headers. |
| Read redirects | Up to 5 by default; destination/port policy at every hop and DNS policy at each new connection. Cross-host redirects are allowed; origin changes reset headers to safe defaults. HTTPS→HTTP is rejected. |
| Mutation redirects | Never followed, including redirects that would convert the request to GET. |
| Deadline | 30 seconds total by default for DNS, redirects, headers, and body; caller can shorten it. Dial, TLS handshake, and response-header waits each have a 10-second cap within that deadline. |
| Size limits | Response headers: 64 KiB. Default decoded response: 512 KiB; returned text: 64 KiB; title: at most 1 KiB. Body reads detect overflow with limit-plus-one reads. |

## Output

Results are JSON with `status`, `headers`, `body`, `final_url`, `content_type`, `title`, and `truncated`. HTTP error statuses are returned as results; policy/transport failures are tool errors. Returned headers are limited to Content-Type, Content-Language, Last-Modified, and ETag.
`body` contains readable text. HTML/XHTML is charset-decoded and parsed into headings, paragraphs, lists, and links; scripts, styles, forms, navigation, common boilerplate, and hidden elements are removed. No JavaScript or subresources are fetched.
Plain text, Markdown, CSV, JSON (including `+json`), XML, RSS, Atom, and YAML are supported. Missing Content-Type is sniffed from at most 512 bytes. Binaries, SVG, images, and PDFs return metadata and an unsupported-type message; empty responses return metadata with an empty body.
Gzip and charset expansion are bounded. Response/text overflow sets `truncated`; incomplete text is incomplete evidence. Use search snippets for unsupported or JavaScript-rendered pages.

## Tavily client

`web_search` uses a [separate client](../internal/agent/tavily.go), independent of the fetch destination policy. It uses normal DNS and standard `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` settings via a clone of `http.DefaultTransport`.
Its key is sent only in a POST body to `https://api.tavily.com/search`; redirects are disabled. Requests honor cancellation and a 30-second timeout, with a 512 KiB decoded response cap. Overflow and non-2xx responses are errors; TLS/header waits are capped at 10 seconds and response headers at 64 KiB.

## Logging and trade-offs

Fetch logs contain tool name, normalized hostname, method, status, duration, and safe error codes. They omit raw arguments, URL paths/queries, headers, bodies, and raw transport errors; generic tool-call logs record only tool name/event. This is log redaction, not output redaction: `final_url` and source text are returned to the agent.
All fetched text/metadata and Tavily results are untrusted source material. [Prompt instructions](../prompts/instructions.md) tell the agent to ignore embedded commands and keep secrets out of research requests; a source cannot authorize mutations.
Public GETs can still disclose data in URLs or trigger poorly designed endpoints with side effects. SSRF controls do not eliminate prompt injection; stronger deployments should also enforce network egress rules.
Authenticated APIs need dedicated, narrowly scoped tools. Conservative blocking trades access to some public special-purpose services for a smaller attack surface. Fetch tests use injected resolver/dialer hooks and local test listeners, without live internet or API keys.
