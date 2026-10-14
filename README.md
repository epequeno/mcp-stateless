# mcp-stateless

Most public MCP servers still assume a session. The 2026-07-28 revision moved the
protocol core to stateless — no `Mcp-Session-Id`, the method in a header, `resultType`
on every result, `_meta` on every request, and a `ttlMs`/`cacheScope` caching hint on
`tools/list` — so any instance can serve any request and a plain round-robin load
balancer is enough. This is a Go implementation of that core, verified against the
published specification.

> ### Measured
>
> p50/p99 and throughput for the stateless core *versus* the session-pinned
> pattern, plus the `tools/list` `ttlMs` caching effect — measured on one
> client, same machine, same workload. See [RESULTS.md](RESULTS.md).

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
- [x] stateless core implemented (types, routing, no session)
- [x] at least one extension surface (Tasks)
- [x] benchmark harness vs. the session-pinned pattern
- [x] measured numbers in `RESULTS.md`

## What broke and how I found it

*Empty — populate as it happens. This section is the part reviewers actually read.*

### 2026-10-13 · Spec verification found the implementation was not conformant

The scaffold was built from the release-candidate announcement, not the
published spec. Verification against the normative spec found 10+ divergences:
`initialize` was removed (replaced by `server/discover`), `resultType` is
required on every result, `_meta` is required on every request with
`protocolVersion` and `clientCapabilities`, `MCP-Protocol-Version` and
`Mcp-Name` headers are required on Streamable HTTP, `cacheScope` replaces
`Cached`, and the Tasks extension has a fundamentally different shape
(server-directed via `tools/call`, not a separate `tasks/create` method).
All load-bearing divergences were fixed. See
[docs/SPEC-NOTES.md](docs/SPEC-NOTES.md) for the full record.

## Notes

- Wire types verified against the published 2026-07-28 specification.
  See [docs/SPEC-NOTES.md](docs/SPEC-NOTES.md) for the verification record.
- The registry is the seam where risk classification and approval gates land in
  the follow-on project; see [docs/DECISIONS.md](docs/DECISIONS.md).
