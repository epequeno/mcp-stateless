package protocol

import "strings"

// MethodFrom resolves the method name for a request.
//
// The spec requires the Mcp-Method header on all Streamable HTTP POSTs
// (§Streamable HTTP › Request Metadata › Standard Request Headers). It mirrors
// the body's `method` field so intermediaries can route without parsing the
// body. This function prefers the header and falls back to the body so the
// pinned (counterfactual) mode — which represents pre-2026-07-28 servers that
// don't send headers — still works. The stateless handler enforces the header
// requirement separately.
func MethodFrom(headerValue string, body *Request) string {
	if h := strings.TrimSpace(headerValue); h != "" {
		return h
	}
	if body != nil {
		return strings.TrimSpace(body.Method)
	}
	return ""
}

// HeaderPresent reports whether the Mcp-Method header was provided.
func HeaderPresent(headerValue string) bool {
	return strings.TrimSpace(headerValue) != ""
}

// CoreMethods are the methods the 2026-07-28 core answers.
// initialize is removed; server/discover replaces it.
// notifications/cancelled is stdio-only on Streamable HTTP.
var CoreMethods = map[string]bool{
	"server/discover": true,
	"tools/list":      true,
	"tools/call":      true,
}

// TaskMethods are the methods introduced by the Tasks extension (SEP-2663).
var TaskMethods = map[string]bool{
	"tasks/get":    true,
	"tasks/update": true,
	"tasks/cancel": true,
}

// IsCore reports whether a method is part of the core protocol.
func IsCore(method string) bool { return CoreMethods[method] }

// IsTaskMethod reports whether a method belongs to the Tasks extension.
func IsTaskMethod(method string) bool { return TaskMethods[method] }
