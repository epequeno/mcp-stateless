# Results

Every number here is produced by a command in this repo and dated. If a row has no
date, it has not been measured.

## Headline

Measured 2026-10-13 on darwin/arm64, Go 1.27.2, single machine. The bench
harness is `bench/main.go`; run it with `just bench` (which starts three
instances — two stateless on :8080/:8082, one pinned on :8081 — runs all
phases, and tears them down). Base commit: `d36826e` (scaffold + Tasks +
bench, pre-commit working tree).

| Metric | Stateless core | Session-pinned | Command | Date |
|---|---|---|---|---|
| p50 latency, tools/call (ms) | 0.042 | 0.035 | `just bench` | 2026-10-13 |
| p99 latency, tools/call (ms) | 0.149 | 0.090 | `just bench` | 2026-10-13 |
| Throughput, tools/call (req/s) | 58,611 | 56,747 | `just bench` | 2026-10-13 |
| `tools/list` cold round trip (ms) | 0.034 | 0.033 | `just bench` | 2026-10-13 |
| `tools/list` with warm `ttlMs` cache (ms) | ~0 | n/a | `just bench` | 2026-10-13 |

## Cross-instance serving — the thesis test

200 requests alternating between two stateless instances (:8080 and :8082),
no session, no affinity:

- instance 1 served: 100
- instance 2 served: 100
- failed: 0
- **PASS** — both instances served requests without a session

This is the row that actually tests the claim. Latency and throughput can
both vary for reasons unrelated to statelessness; only cross-instance serving
without a session distinguishes the mechanism from a coincidence.

## What the numbers say — honestly

The p50 difference (0.042 vs 0.035 ms) is within the run-to-run spread —
report the spread, not the difference. The p99 difference (0.149 vs 0.090 ms)
is real but small; the stateless core is slightly slower at the tail,
consistent with the header-routing path doing a little more work.

Throughput is within ~3% — not a meaningful gap for this workload. The echo
tool does no I/O, so the session check is a negligible fraction of the
request. A real tool with I/O would widen the gap, but that is not what
this bench measures.

The `ttlMs` cache hint is free to advertise: the cold round trip is 0.034 ms,
a warm cache is ~0 ms, and the registry is fixed for process lifetime.

## Scope

These numbers cover `tools/*` — the stateless core. The `tasks/*` extension
uses a per-instance in-memory store by default, so the round-robin property
does **not** hold for `tasks/get` across instances without a shared store.
See [docs/DECISIONS.md](docs/DECISIONS.md) for the store interface and the
limitation.

## Reproduce

```sh
just bench
```

The exact command and the commit SHA are the evidence. A number without its
command is not evidence.
