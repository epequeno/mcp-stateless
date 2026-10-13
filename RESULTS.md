# Results

Every number here is produced by a command in this repo and dated. If a row has no
date, it has not been measured.

## Headline

| Metric | Stateless core | Session-pinned | Command | Date |
|---|---|---|---|---|
| p50 latency (ms) | TODO | TODO | `just bench` | — |
| p99 latency (ms) | TODO | TODO | `just bench` | — |
| Throughput (req/s) | TODO | TODO | `just bench` | — |
| `tools/list` with warm `ttlMs` cache (ms) | TODO | n/a | `just bench` | — |

## Reproduce

```sh
just up          # terminal 1 — stateless
just up-pinned   # terminal 2 — session-pinned
just bench       # terminal 3 — same client against both
```

Take p50/p99 from a **single-writer client** against each mode in turn, then record
the exact command line and the commit SHA next to the numbers. A number without its
command is not evidence.
