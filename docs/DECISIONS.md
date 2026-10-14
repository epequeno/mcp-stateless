# Decisions

Short log of choices that would otherwise be re-litigated. Newest last.

## 2026-10-13 · Scaffold, and one binary with two modes

Two modes in one binary (`--pinned`) rather than two programs, so the benchmark
compares like with like: same registry, same routing code, same JSON encoding,
one behavioural difference. Two implementations would confound the comparison
with unrelated differences.

## 2026-10-13 · Method routing falls back to the body

`protocol.MethodFrom` prefers the header and falls back to the parsed body. The
header is what makes the core stateless; the fallback keeps a session-pinned
client working, which is what makes the A/B a comparison of *session handling*
rather than of two protocols.

Consequence to watch: the fallback means the server still reads the body. If a
later goal is to route without buffering, this is the line to revisit.

## 2026-10-13 · `Risk` on the registry, unused today

Every tool carries a `Risk` class now, though nothing enforces it yet. The
follow-on governance project classifies exactly these entries and requires every
reachable tool to be classified; adding the field later would mean touching every
call site. Cheaper to carry an unused string now.

## 2026-10-13 · MIT

Matches `support-triage`. The point of the repo is that a reviewer can read and
reuse it.

## 2026-10-13 · Tasks extension, in-memory store behind an interface

The Tasks surface (`tasks/create`, `tasks/get`, `tasks/cancel`) is implemented
as an asynchronous tool invocation: create resolves a tool from the existing
registry, runs it in the background, and hands back a handle; get polls; cancel
abandons. Task state lives behind `tasks.Store` — an interface with an
in-memory default.

Consequence to watch: the in-memory store is per-process. Two instances behind
a round-robin load balancer do not share it, so a `tasks/get` routed to the
"other" instance misses. The stateless core's round-robin claim therefore does
not hold for `tasks/*` until a shared store (Redis, database) is plugged in.
That is the seam the interface exists for — swap the implementation, not the
call sites. The benchmark in `bench/` must call this out: the round-robin
property is demonstrated for `tools/*`, not for `tasks/*` with the default store.

A task handle is a random hex id, not a counter, so it carries no information
about which instance created it — the property a shared store needs to make
the get stateless.

## 2026-10-13 · Conformance rewrite — matching the published spec

The scaffold was built from the release-candidate announcement. Verification
against the published 2026-07-28 spec (modelcontextprotocol.io) found that the
implementation was not conformant. The load-bearing divergences were fixed:

- `initialize` → `server/discover` (spec: server/discover §Response)
- `resultType: "complete"` added to all results (spec: basic §ResultType)
- `_meta` with `protocolVersion` and `clientCapabilities` parsed from every
  request (spec: basic §_meta)
- `MCP-Protocol-Version` header required on stateless mode (spec: Streamable
  HTTP §Protocol Version Header)
- `Mcp-Method` header required in stateless mode; body fallback kept for the
  pinned counterfactual only (spec: Streamable HTTP §Standard Request Headers)
- `Mcp-Name` header sent for `tools/call` (spec: Streamable HTTP §Standard
  Request Headers)
- `cacheScope` (`"public"`/`"private"`) replaces the non-spec `Cached` boolean
  (spec: Caching §Cache Scope Field)
- `HeaderMismatch` error code `-32020` for missing headers (spec: basic §Error
  Codes)
- Tasks reshaped to SEP-2663: `tasks/create` removed; `tools/call` returns
  `CreateTaskResult` (`resultType: "task"`) when the tool is async and the
  client declares the `io.modelcontextprotocol/tasks` extension. Methods are
  `tasks/get`, `tasks/update`, `tasks/cancel`. Status values renamed to
  `working`/`completed`/`failed`/`cancelled`. Field `id` → `taskId`.
  `notifications/cancelled` removed from the stateless method set (stdio-only
  on Streamable HTTP).

The `Mcp-Method` body fallback is kept for the pinned mode only. The spec says
the header is REQUIRED, but the pinned mode represents pre-2026-07-28 servers
that don't send headers — keeping the fallback makes the A/B comparison fair
rather than making the pinned mode fail on a technicality.
