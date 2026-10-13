# deploy — the round-robin claim

**Not implemented.** This is the plan the benchmark is meant to justify.

## Target shape

```
        ┌──────────────┐
client ─▶ load balancer │  (no stickiness, no session affinity)
        └──────┬───────┘
        ┌──────┴──────┐
        ▼             ▼
   instance A     instance B     ← either may serve any request
```

## What must be true for this to work

1. No session is issued, so nothing needs to be routed back to its origin.
2. No session store is consulted, so there is no shared-state dependency.
3. The listing cache hint (`ttlMs`) is safe to reuse across instances, so
   per-instance divergence cannot produce a stale-but-valid-looking listing.

Point 3 is the one that bites in practice: cache hints are only honest if every
instance would answer identically.

## Contraindications to record

- Any tool whose correctness depends on per-instance state (it must not exist
  here, but say why).
- The `initialize` response: stateless means it cannot return a handle that
  later requests must present.
- Affinity creeping back in via the client (for example a client that pins to a
  connection for its own reasons) — measure, do not assume.
