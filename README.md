# mcp-stateless

Most public MCP servers still assume a session. The 2026-07-28 revision moved the
protocol core to stateless — no `Mcp-Session-Id`, the method in a header, and a
`ttlMs` caching hint on `tools/list` — so any instance can serve any request and a
plain round-robin load balancer is enough. This is a Go implementation of that core.

> ### TODO — not yet measured
>
> **The headline number does not exist yet.** What this repo is aiming at:
> p50/p99 and throughput for the stateless core *versus* the session-pinned
> pattern, plus the `tools/list` `ttlMs` caching effect — measured on one client,
> same machine, same workload. Fill it in from `just bench` and record it in
> [RESULTS.md](RESULTS.md). Until then this README makes no claim.

## Shape

```
client ──▶ round-robin LB ──▶ mcp-stateless instance N   (no sticky sessions)
                 │                     │
                 │                     ├── internal/protocol   wire types + method routing
                 │                     ├── internal/tools      registry (the governance seam)
                 │                     └── internal/server     net/http wiring
                 └──▶ mcp-stateless instance M   (any instance, any request)
```

The two modes are deliberately in one binary so the comparison is fair:

| Mode | Flag | Behaviour |
|---|---|---|
| Stateless core | (default) | no session issued, method routed from `Mcp-Method`, `ttlMs` advertised |
| Session-pinned | `--pinned` | issues and requires a session id — the shape most servers still ship |

## Run

```sh
brew install go     # not yet installed on this machine
just verify         # vet + test
just up             # stateless core on :8080
just up-pinned      # session-pinned comparison on :8081
```

## Status

- [x] scaffold — this commit
- [ ] stateless core implemented (types, routing, no session)
- [ ] at least one extension surface (Tasks or MCP Apps)
- [ ] benchmark harness vs. the session-pinned pattern
- [ ] measured numbers in `RESULTS.md`

## What broke and how I found it

*Empty — populate as it happens. This section is the part reviewers actually read.*

## Notes

- Wire-type details are taken from the release-candidate announcement, not yet
  re-checked against the published spec. See [docs/SPEC-NOTES.md](docs/SPEC-NOTES.md).
- The registry is the seam where risk classification and approval gates land in
  the follow-on project; see [docs/DECISIONS.md](docs/DECISIONS.md).
