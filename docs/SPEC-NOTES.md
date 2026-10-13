# Spec notes — the 2026-07-28 stateless core

**Status: unverified against the normative specification.** The deltas below are
taken from the release-candidate announcement (blog.modelcontextprotocol.io,
posts/2026-07-28-release-candidate), read 2026-10-13. Before this server is
called conformant, every row must be re-checked against the published spec and
this file updated with the section number that confirms it.

| # | Delta | How this repo encodes it | Verified? |
|---|---|---|---|
| 1 | No session at the protocol layer | `Mcp-Session-Id` is never issued or read; the pinned mode is the counterfactual | ☐ |
| 2 | Method routing via a header | `protocol.HeaderMethod` (`Mcp-Method`); body is the fallback | ☐ |
| 3 | `tools/list` carries a TTL cache hint | `ListResult.TTLMs`; `Cached` always false on our own response | ☐ |
| 4 | Extensions framework (Tasks, MCP Apps) | **Not implemented** — the deliverable requires at least one | ☐ |
| 5 | Header name and casing | Assumed `Mcp-Method` / `Mcp-Protocol-Version` | ☐ |

## Why this matters for the headline claim

If delta 2 or 5 is wrong, the stateless core still works but is *not* conformant,
and the benchmark compares our own invention rather than the protocol. That would
make the number real but the claim false — the failure mode worth guarding
against, because it is invisible in the benchmark output.

Rows 1 and 3 are the load-bearing ones: they are what make a plain round-robin
load balancer sufficient, which is the property the benchmark is trying to
demonstrate.

## Open questions to resolve against the spec

1. Is the method header required, or optional with body fallback? (Affects whether
   the fallback path is conformant or merely convenient.)
2. Does `ttlMs` apply per-tool, or to the listing as a whole?
3. What exactly must `initialize` return in a stateless world — and may a server
   omit it entirely if the header is present?
