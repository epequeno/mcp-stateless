package protocol

import "strings"

// MethodFrom resolves the method name for a request.
//
// A stateless peer puts the method in HeaderMethod precisely so the server can
// route without a session, without a prior initialize, and without having to
// buffer the body first. The body remains the fallback so a session-pinned
// client still works — which is what makes the A/B in bench/ a fair comparison
// rather than a comparison of two different protocols.
func MethodFrom(headerValue string, body *Request) string {
	if h := strings.TrimSpace(headerValue); h != "" {
		return h
	}
	if body != nil {
		return strings.TrimSpace(body.Method)
	}
	return ""
}

// StatelessMethods are the methods the core answers without a handshake.
var StatelessMethods = map[string]bool{
	"initialize":              true,
	"tools/list":              true,
	"tools/call":              true,
	"notifications/cancelled": true,
}

// IsStateless reports whether a method is answerable without a session.
func IsStateless(method string) bool { return StatelessMethods[method] }
