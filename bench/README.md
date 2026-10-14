# bench — the headline number

**Implemented.** `just bench` starts three server instances (two stateless on
:8080 and :8082, one pinned on :8081), runs all four phases, and tears them
down. The harness is `bench/main.go` — one client, one workload, both modes.

## The claim being tested

A stateless core lets a **plain round-robin load balancer** front N instances —
no sticky sessions, no session store, no affinity. The session-pinned shape
cannot, which is why most deployed MCP servers still need affinity.

## What to measure, and against what

| Metric | Stateless | Pinned | Why it matters |
|---|---|---|---|
| p50 / p99 latency | ☑ | ☑ | affinity adds a hop or a store lookup |
| Throughput at fixed concurrency | ☑ | ☑ | the scaling claim |
| Requests served by instance 2+ without a session | ☑ | n/a | the property itself |
| `tools/list` with warm `ttlMs` cache | ☑ | n/a | whether the hint is worth advertising |

The third row is the one that actually tests the thesis. Latency and throughput
can both improve for reasons unrelated to statelessness; only cross-instance
serving without a session distinguishes the mechanism from a coincidence.

## Same-client rule

Use **one** client for both modes and for every concurrency level — do not fork a
per-mode harness. This is the same principle behind the shared load generator in
`full-stack-serving` (its ADR 0007): applying an identical client to each
comparison is what makes the levels comparable. Reuse that generator if it fits
rather than writing a second one.

## Falsification

Before running, state what result would kill the claim:

- If the pinned mode serves **all** requests from one instance under equal load,
  statelessness bought nothing measurable and the honest result is "no effect".
- If p50 differs by less than the run-to-run spread, report the spread, not the
  difference.
- If the TTL cache shows no effect, say so; an advertised hint that does nothing
  is still a finding.

## Recording

Fill in `RESULTS.md` with the number, the exact command, the commit SHA, and the
date. A number without its command is not evidence.

## Scope

The bench covers `tools/*` — the stateless core. The `tasks/*` extension uses a
per-instance in-memory store by default, so the round-robin property does not
hold for `tasks/get` across instances without a shared store. See
[docs/DECISIONS.md](../docs/DECISIONS.md) for the `tasks.Store` interface and the
limitation.
