// Package protocol holds the wire types and routing rules for the stateless
// core of the 2026-07-28 MCP revision.
//
// Nothing in this package assumes a session: no session id is issued, stored,
// or required. See docs/SPEC-NOTES.md for the deltas this encodes, and for what
// has not yet been checked against the normative specification.
package protocol

import "encoding/json"

// JSONRPCVersion is carried by every frame.
const JSONRPCVersion = "2.0"

// Revision is the protocol revision this server targets.
const Revision = "2026-07-28"

// Request headers a stateless peer uses to route a call without a prior
// initialize handshake and without a session. The body is the fallback.
//
// TODO(A1): these names come from the release-candidate announcement, not the
// normative spec. Confirm exact names and casing before relying on them.
const (
	HeaderMethod          = "Mcp-Method"
	HeaderProtocolVersion = "Mcp-Protocol-Version"
)

// JSON-RPC error codes used by this server.
const (
	CodeParse          = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603
)

// Request is one JSON-RPC frame from a client.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is one JSON-RPC frame back to a client.
type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

// Error is a JSON-RPC error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Tool is one entry in a tools/list result.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema,omitempty"`
	ReadOnly    bool   `json:"readOnlyHint,omitempty"`
}

// ListResult is the tools/list payload.
//
// TTLMs is the caching hint added in this revision so a client may reuse a
// listing across connections. A server must never advertise its own response
// as cached, so Cached is always false here.
type ListResult struct {
	Tools  []Tool `json:"tools"`
	TTLMs  int    `json:"ttlMs"`
	Cached bool   `json:"cached"`
}

// CallParams is the tools/call payload.
type CallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// CallResult is the tools/call result.
type CallResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content is one content block in a call result.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}
