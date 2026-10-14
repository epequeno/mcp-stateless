# Results

Every number here is produced by a command in this repo and dated. If a row has no
date, it has not been measured.

## Headline

Measured 2026-10-13 on darwin/arm64, Go 1.27.2, single machine. The bench
harness is `bench/main.go`; run it with `just bench` (which starts three
instances — two stateless on :8080/:8082, one pinned on :8081 — runs all
phases, and tears them down). Base commit: `279c80d` — the spec-conformance
commit. The original run was taken from the working tree that became this commit,
and was reproduced at this SHA on 2026-10-13.

| Metric | Stateless core | Session-pinned | n | Command | Date |
|---|---|---|---|---|---|
| p50 latency, tools/call (ms) | 0.042 | 0.035 | 1000 | `just bench` | 2026-10-13 |
| p99 latency, tools/call (ms) | 0.149 | 0.090 | 1000 | `just bench` | 2026-10-13 |
| Throughput, tools/call (req/s) | 58,611 | 56,747 | 5000 | `just bench` | 2026-10-13 |
| `tools/list` cold round trip (ms) | 0.034 | 0.033 | 1000 | `just bench` | 2026-10-13 |
| `tools/list` with warm `ttlMs` cache (ms) | ~0 | n/a | — | `just bench` | 2026-10-13 |

## Independent reproduction

Re-run at `279c80d` (same machine, same command) to check the table above holds:

| Metric | Stateless core | Session-pinned |
|---|---|---|
| p50 / p99, `tools/call` (ms) | 0.045 / 0.168 | 0.038 / 0.070 |
| min / **max**, `tools/call` (ms) | 0.033 / **2.523** | 0.028 / 0.545 |
| Throughput (req/s) | 57,785 | 56,071 |
| Cross-instance (200 reqs) | 100 / 100, 0 failed | n/a |

p50 and throughput reproduce within a few percent; the cross-instance result
reproduces exactly. The reproduction's contribution is the **max** column, which
is what the tail claim below depends on.

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
is **not attributed**: the harness has no warmup phase, so the first request is
inside the measurement, and in the reproduction a single 2.5 ms request out of
1,000 lands in the stateless column while nothing comparable lands in the pinned
column. That one outlier is enough on its own to explain the whole gap. Treat the
tail difference as inconclusive until a warmup phase separates cold-start cost
from the request path.

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
