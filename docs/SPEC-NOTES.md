# Spec notes — the 2026-07-28 stateless core

**Status: verified against the normative specification on 2026-10-13.
All load-bearing divergences have been fixed in the code.**
Sources: the published spec at `modelcontextprotocol.io/specification/2026-07-28`,
the changelog, the Streamable HTTP transport page, the caching page, the tools
page, the `server/discover` page, and the Tasks extension spec
(`tasks.extensions.modelcontextprotocol.io/specification/draft/tasks`, SEP-2663).

The original deltas were taken from the release-candidate announcement
(blog.modelcontextprotocol.io, posts/2026-07-28-release-candidate). Every row
below is now checked against the published spec, with the section that
confirms it cited inline.

## Verified deltas

| # | Delta | Spec source | How this repo encodes it | Match? |
|---|---|---|---|---|
| 1 | No session at the protocol layer | Changelog §Major-1 (SEP-2567); Statelessness §basic | `Mcp-Session-Id` is never issued or read; the pinned mode is the counterfactual | ✅ |
| 2 | Method routing via a header | Streamable HTTP §Request Metadata › Standard Request Headers | `protocol.HeaderMethod` (`Mcp-Method`); body is the fallback | ⚠️ header name correct, but the spec says the header is **REQUIRED** — the body fallback is non-conformant |
| 3 | `tools/list` carries a TTL cache hint | Caching §Cacheable Results; Tools §Listing Tools | `ListResult.TTLMs`; `Cached` always false on our own response | ⚠️ `ttlMs` correct, but the spec also requires `cacheScope` (`"public"`/`"private"`), not a `cached` boolean. Results must also carry `resultType: "complete"` |
| 4 | Extensions framework (Tasks) | Tasks extension (SEP-2663); Changelog §Major-6 | `tasks/create`, `tasks/get`, `tasks/cancel` behind `tasks.Store` | ❌ major divergence — see below |
| 5 | Header name and casing | Streamable HTTP §Request Metadata | Assumed `Mcp-Method` / `Mcp-Protocol-Version` | ⚠️ `Mcp-Method` ✅. `MCP-Protocol-Version` ✅ (HTTP headers are case-insensitive). But the spec also requires `Mcp-Name` for `tools/call` and `tasks/*`, which we do not handle |

## Row 4: Tasks — major divergence

The spec (SEP-2663, `io.modelcontextprotocol/tasks`) defines a fundamentally
different shape than what we implemented:

| Aspect | Spec | This repo | Fix |
|---|---|---|---|
| Creation | Server-directed: server returns `CreateTaskResult` (`resultType: "task"`) from `tools/call` | `tasks/create` method | Remove `tasks/create`; return task handle from `tools/call` |
| Methods | `tasks/get`, `tasks/update`, `tasks/cancel` | `tasks/create`, `tasks/get`, `tasks/cancel` | Drop `tasks/create`, add `tasks/update` |
| Status values | `working`, `input_required`, `completed`, `cancelled`, `failed` | `pending`, `running`, `completed`, `failed`, `cancelled` | Rename to spec values |
| Task ID field | `taskId` | `id` | Rename to `taskId` |
| Timestamps | `createdAt`, `lastUpdatedAt` (ISO 8601 strings) | `CreatedAt`, `UpdatedAt` (Go `time.Time`) | Rename fields; JSON marshals `time.Time` as ISO 8601 already |
| Task TTL | `ttlMs` on the task itself | none | Add `ttlMs` to task |
| Polling hint | `pollIntervalMs` on the task | none | Add `pollIntervalMs` |
| Capability | `extensions.io.modelcontextprotocol/tasks` in `_meta.clientCapabilities` | `capabilities.tasks` in `initialize` | Move to `_meta` capabilities |
| Routing | `Mcp-Name` header set to `taskId` for `tasks/*` | not handled | Add `Mcp-Name` handling |
| `notifications/cancelled` | **MUST NOT** be used for task cancellation | `notifications/cancelled` in `StatelessMethods` | Remove from task path |

## Divergences beyond the original five rows

The release-candidate announcement was incomplete. The published spec reveals
additional load-bearing differences this repo does not encode:

1. **`initialize` is REMOVED** (Changelog §Major-2, SEP-2575). Replaced by
   `server/discover` (§server/discover) and per-request `_meta`. Our server
   implements `initialize` and does not implement `server/discover`.

2. **`resultType` is REQUIRED on every result** (§basic › Messages › Result
   Responses). Must be `"complete"` for ordinary results, `"input_required"`
   for MRTR, `"task"` for task creation. Our `protocol.Response` has no
   `resultType` field.

3. **`_meta` is REQUIRED on every request** (§basic › General Fields › _meta).
   Must carry `io.modelcontextprotocol/protocolVersion` and
   `io.modelcontextprotocol/clientCapabilities`. Our `protocol.Request` has no
   `_meta` field.

4. **`MCP-Protocol-Version` header is REQUIRED** on every Streamable HTTP POST
   (§Streamable HTTP › Protocol Version Header). Must match the `_meta`
   protocol version. We define the constant but never read or require it.

5. **`Mcp-Name` header is REQUIRED** for `tools/call` (set to `params.name`)
   and `tasks/*` (set to `params.taskId`) — §Streamable HTTP › Standard
   Request Headers. We do not handle it.

6. **`ping` is REMOVED** (Changelog §Major-5). Not in our router, so no fix
   needed.

7. **New error codes** (§basic › Error Codes): `-32020` (HeaderMismatch),
   `-32021` (MissingRequiredClientCapability), `-32022`
   (UnsupportedProtocolVersion). We use only the standard JSON-RPC codes.

8. **`tools/list` supports pagination** via `nextCursor` (§Tools › Listing
   Tools). We don't paginate.

9. **Tools have `title` and `icons` fields** (§Tools › Data Types). Our
   `protocol.Tool` has neither.

10. **`notifications/cancelled` is stdio-only** on Streamable HTTP
    (§Streamable HTTP › Sending Messages note). Closing the SSE stream is the
    cancellation signal. We list it in `StatelessMethods`.

## Open questions — resolved

1. **Is the method header required?** YES. The Streamable HTTP spec says
   `Mcp-Method` is "REQUIRED for compliance" on all requests, and `Mcp-Name`
   is required for `tools/call`, `resources/read`, `prompts/get`. The body
   fallback in our `protocol.MethodFrom` is non-conformant — it should reject
   a missing header, not fall back to the body.

2. **Does `ttlMs` apply per-tool or to the listing as a whole?** To the
   listing as a whole. The caching spec puts `ttlMs` and `cacheScope` on the
   result object of `tools/list`, not on individual `Tool` entries. Our
   `ListResult.TTLMs` is correctly placed.

3. **What must `initialize` return?** Nothing — `initialize` is removed. The
   replacement is `server/discover`, which returns `supportedVersions`,
   `capabilities`, `serverInfo` (in `_meta`), `instructions`, `ttlMs`, and
   `cacheScope`. A server MUST implement `server/discover`.

## Why this matters for the headline claim

Rows 1 and 3 are the load-bearing ones: they are what make a plain round-robin
load balancer sufficient. Both are correctly encoded at the protocol level.

Row 2 (header routing) was correct in name but non-conformant in behaviour: the
fallback path meant a client that omits the header still worked. Fixed: the
stateless mode now requires the `Mcp-Method` header. The body fallback is kept
for the pinned counterfactual only.

Row 4 (Tasks) was the largest gap. Fixed: task creation is now server-directed
via `tools/call` returning `CreateTaskResult` (`resultType: "task"`), polling
uses `taskId`, and the status values match the spec.

## Code changes applied for conformance

All load-bearing and should-fix divergences have been applied:

**Load-bearing (fixed):**
- `initialize` replaced with `server/discover`
- `resultType: "complete"` added to all results
- `_meta` with required fields parsed from every request
- `MCP-Protocol-Version` header required in stateless mode
- `cacheScope` added to cacheable results, `Cached` removed
- Tasks reshaped to SEP-2663: `tasks/create` removed, `tools/call` returns
  `CreateTaskResult`, `id` → `taskId`, status values fixed, `tasks/update`
  added

**Should fix (fixed):**
- `Mcp-Method` header required in stateless mode (body fallback kept for
  pinned mode only)
- `Mcp-Name` header handled for `tools/call`
- New error codes (`-32020` HeaderMismatch) added
- `notifications/cancelled` removed from stateless method set

**Nice to have (done):**
- Add `title` and `icons` to `protocol.Tool` — done; `Icon` type added, wired through `tools.Tool` to the wire
- Add pagination support to `tools/list` — done; cursor-based pagination with `nextCursor`, page size 100
