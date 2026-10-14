// Package protocol holds the wire types and routing rules for the
// 2026-07-28 MCP revision, verified against the published specification.
//
// Nothing in this package assumes a session: no session id is issued, stored,
// or required. See docs/SPEC-NOTES.md for the verification record.
package protocol

import "encoding/json"

// JSONRPCVersion is carried by every frame.
const JSONRPCVersion = "2.0"

// Revision is the protocol revision this server targets.
const Revision = "2026-07-28"

// Request headers required by the Streamable HTTP transport so intermediaries
// can route and inspect requests without parsing the body.
// Spec: Streamable HTTP §Request Metadata.
const (
	HeaderMethod          = "Mcp-Method"
	HeaderName            = "Mcp-Name"
	HeaderProtocolVersion = "MCP-Protocol-Version"
)

// ResultType values carried by every result.
// Spec: basic §Messages › Result Responses › ResultType.
const (
	ResultComplete      = "complete"
	ResultInputRequired = "input_required"
	ResultTask          = "task"
)

// JSON-RPC error codes.
// Standard codes from JSON-RPC 2.0; MCP-specific codes from
// basic §Error Codes.
const (
	CodeParse          = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603

	// MCP-specific codes (-32020 to -32099, spec-reserved).
	CodeHeaderMismatch             = -32020
	CodeMissingClientCapability    = -32021
	CodeUnsupportedProtocolVersion = -32022
)

// CacheScope values for cacheable results.
// Spec: Caching §Cache Scope Field.
const (
	CacheScopePublic  = "public"
	CacheScopePrivate = "private"
)

// Meta is the _meta field on a request, carrying per-request protocol
// metadata. Spec: basic §General Fields › _meta.
type Meta struct {
	ProtocolVersion    string          `json:"io.modelcontextprotocol/protocolVersion"`
	ClientInfo         *Implementation `json:"io.modelcontextprotocol/clientInfo,omitempty"`
	ClientCapabilities json.RawMessage `json:"io.modelcontextprotocol/clientCapabilities"`
}

// Implementation identifies a client or server.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Request is one JSON-RPC frame from a client.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RequestParams holds the parsed params including _meta. The _meta field
// can appear at the top level of params.
type RequestParams struct {
	Meta  *Meta          `json:"_meta,omitempty"`
	Name  string         `json:"name,omitempty"`
	Args  map[string]any `json:"arguments,omitempty"`
	Other map[string]any `json:"-"`
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
	Data    any    `json:"data,omitempty"`
}

// Tool is one entry in a tools/list result.
// Spec: Tools §Data Types.
type Tool struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"inputSchema,omitempty"`
	Icons       []Icon `json:"icons,omitempty"`
}

// Icon is a visual identifier for a tool, resource, or prompt.
// Spec: basic §General Fields › icons.
type Icon struct {
	Src      string   `json:"src"`
	MimeType string   `json:"mimeType,omitempty"`
	Sizes    []string `json:"sizes,omitempty"`
	Theme    string   `json:"theme,omitempty"`
}

// ListResult is the tools/list payload.
//
// TTLMs is the freshness hint (milliseconds); CacheScope controls who may
// cache. Spec: Caching §Cacheable Results.
type ListResult struct {
	ResultType string `json:"resultType"`
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
	TTLMs      int    `json:"ttlMs"`
	CacheScope string `json:"cacheScope"`
}

// DiscoverResult is the server/discover payload.
// Spec: server/discover §Response.
type DiscoverResult struct {
	ResultType        string         `json:"resultType"`
	SupportedVersions []string       `json:"supportedVersions"`
	Capabilities      map[string]any `json:"capabilities"`
	Instructions      string         `json:"instructions,omitempty"`
	TTLMs             int            `json:"ttlMs"`
	CacheScope        string         `json:"cacheScope"`
}

// ListParams is the tools/list payload.
// Spec: Tools §Listing Tools (supports pagination via cursor).
type ListParams struct {
	Cursor string `json:"cursor,omitempty"`
}

// CallParams is the tools/call payload.
type CallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// CallResult is the tools/call result.
type CallResult struct {
	ResultType string    `json:"resultType"`
	Content    []Content `json:"content"`
	IsError    bool      `json:"isError,omitempty"`
}

// Content is one content block in a call result.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}
