package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/epequeno/mcp-stateless/internal/protocol"
	"github.com/epequeno/mcp-stateless/internal/tools"
)

// post issues one JSON-RPC frame with the stateless routing headers and _meta.
// For stateless mode, the Mcp-Method and MCP-Protocol-Version headers are
// always set, and _meta is included in the body.
func post(t *testing.T, h http.Handler, header, body string) (*httptest.ResponseRecorder, protocol.Response) {
	t.Helper()

	// Wrap the body to inject _meta if it doesn't already have it.
	body = withMeta(t, body)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if header != "" {
		req.Header.Set(protocol.HeaderMethod, header)
	}
	req.Header.Set(protocol.HeaderProtocolVersion, protocol.Revision)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp protocol.Response
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v (body %q)", err, rec.Body.String())
		}
	}
	return rec, resp
}

// postRaw is like post but without the protocol headers — for testing the
// pinned mode or header-missing error paths.
func postRaw(t *testing.T, h http.Handler, body string) (*httptest.ResponseRecorder, protocol.Response) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp protocol.Response
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v (body %q)", err, rec.Body.String())
		}
	}
	return rec, resp
}

// withMeta injects _meta into the params of a JSON-RPC body if it has params
// but no _meta. This is the minimum a conformant stateless client sends.
func withMeta(t *testing.T, body string) string {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("withMeta: invalid body %q: %v", body, err)
	}
	params, ok := raw["params"]
	if !ok {
		// No params — add a minimal params with _meta.
		raw["params"] = map[string]any{
			"_meta": map[string]any{
				"io.modelcontextprotocol/protocolVersion":    protocol.Revision,
				"io.modelcontextprotocol/clientCapabilities": map[string]any{},
			},
		}
		b, _ := json.Marshal(raw)
		return string(b)
	}
	// params exists — check for _meta inside it.
	pmap, ok := params.(map[string]any)
	if !ok {
		return body
	}
	if _, hasMeta := pmap["_meta"]; hasMeta {
		return body
	}
	pmap["_meta"] = map[string]any{
		"io.modelcontextprotocol/protocolVersion":    protocol.Revision,
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}
	raw["params"] = pmap
	b, _ := json.Marshal(raw)
	return string(b)
}

// The defining property of the stateless core: it hands back no session.
func TestStatelessCoreIssuesNoSession(t *testing.T) {
	s := New(Config{Stateless: true})

	rec, _ := post(t, s, "server/discover", `{"jsonrpc":"2.0","id":1,"method":"server/discover"}`)
	if got := rec.Header().Get(headerSessionID); got != "" {
		t.Fatalf("stateless core must not issue %s, got %q", headerSessionID, got)
	}
}

// A stateless peer calls straight in, with no handshake first.
func TestCallWithoutHandshake(t *testing.T) {
	s := New(Config{Stateless: true})

	rec, resp := post(t, s, "tools/call",
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"a":1}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

// The routing header alone must be enough to reach a handler.
func TestMethodRoutedFromHeaderOnly(t *testing.T) {
	s := New(Config{Stateless: true})

	rec, resp := post(t, s, "tools/list", `{"jsonrpc":"2.0","id":3}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

func TestListCarriesTTLAndCacheScope(t *testing.T) {
	s := New(Config{Stateless: true})

	_, resp := post(t, s, "tools/list", `{"jsonrpc":"2.0","id":4,"method":"tools/list"}`)

	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var lr protocol.ListResult
	if err := json.Unmarshal(raw, &lr); err != nil {
		t.Fatalf("unmarshal list result: %v", err)
	}
	if lr.TTLMs <= 0 {
		t.Fatalf("ttlMs = %d, want > 0", lr.TTLMs)
	}
	if lr.CacheScope == "" {
		t.Fatal("cacheScope must not be empty")
	}
	if lr.ResultType != protocol.ResultComplete {
		t.Fatalf("resultType = %q, want %q", lr.ResultType, protocol.ResultComplete)
	}
	if len(lr.Tools) == 0 {
		t.Fatal("expected at least one tool")
	}
}

// The counterfactual mode really is stricter, or the benchmark is meaningless.
func TestPinnedModeRequiresSession(t *testing.T) {
	s := New(Config{Stateless: false})

	rec, _ := postRaw(t, s, `{"jsonrpc":"2.0","id":5,"method":"server/discover"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("pinned mode without a session: status = %d, want 400", rec.Code)
	}
}

func TestUnknownMethod(t *testing.T) {
	s := New(Config{Stateless: true})

	_, resp := post(t, s, "nope/nope", `{"jsonrpc":"2.0","id":6,"method":"nope/nope"}`)
	if resp.Error == nil {
		t.Fatal("expected an error for an unknown method")
	}
	if resp.Error.Code != protocol.CodeMethodNotFound {
		t.Fatalf("code = %d, want %d", resp.Error.Code, protocol.CodeMethodNotFound)
	}
}

// server/discover replaces initialize and returns capabilities.
func TestServerDiscover(t *testing.T) {
	s := New(Config{Stateless: true})

	_, resp := post(t, s, "server/discover", `{"jsonrpc":"2.0","id":1,"method":"server/discover"}`)
	if resp.Error != nil {
		t.Fatalf("discover error: %+v", resp.Error)
	}

	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dr protocol.DiscoverResult
	if err := json.Unmarshal(raw, &dr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dr.ResultType != protocol.ResultComplete {
		t.Fatalf("resultType = %q, want %q", dr.ResultType, protocol.ResultComplete)
	}
	if len(dr.SupportedVersions) == 0 {
		t.Fatal("expected supportedVersions")
	}
	if _, ok := dr.Capabilities["tools"]; !ok {
		t.Fatal("discover must advertise tools capability")
	}
	exts, ok := dr.Capabilities["extensions"].(map[string]any)
	if !ok {
		t.Fatal("discover must advertise extensions")
	}
	if _, ok := exts["io.modelcontextprotocol/tasks"]; !ok {
		t.Fatal("discover must advertise tasks extension")
	}
}

// The stateless core must reject a request missing the Mcp-Method header.
func TestStatelessRejectsMissingMethodHeader(t *testing.T) {
	s := New(Config{Stateless: true})

	// Send without the Mcp-Method header.
	body := withMeta(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(protocol.HeaderProtocolVersion, protocol.Revision)

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	var resp protocol.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for missing Mcp-Method header")
	}
	if resp.Error.Code != protocol.CodeHeaderMismatch {
		t.Fatalf("code = %d, want %d", resp.Error.Code, protocol.CodeHeaderMismatch)
	}
}

// The stateless core must reject a request missing the MCP-Protocol-Version header.
func TestStatelessRejectsMissingProtocolVersionHeader(t *testing.T) {
	s := New(Config{Stateless: true})

	body := withMeta(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(protocol.HeaderMethod, "tools/list")
	// Intentionally omit MCP-Protocol-Version.

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	var resp protocol.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for missing MCP-Protocol-Version header")
	}
	if resp.Error.Code != protocol.CodeHeaderMismatch {
		t.Fatalf("code = %d, want %d", resp.Error.Code, protocol.CodeHeaderMismatch)
	}
}

// All results must carry resultType.
func TestCallResultHasResultType(t *testing.T) {
	s := New(Config{Stateless: true})

	_, resp := post(t, s, "tools/call",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"x":1}}}`)
	if resp.Error != nil {
		t.Fatalf("call error: %+v", resp.Error)
	}

	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var cr protocol.CallResult
	if err := json.Unmarshal(raw, &cr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cr.ResultType != protocol.ResultComplete {
		t.Fatalf("resultType = %q, want %q", cr.ResultType, protocol.ResultComplete)
	}
}

// tools/list with a cursor returns the next page and a nextCursor if more
// tools remain.
func TestListPagination(t *testing.T) {
	s := New(Config{Stateless: true})

	// The default registry has one tool (echo). Add enough to exceed
	// listPageSize (100), so pagination kicks in.
	reg := s.cfg.Registry
	for i := range 105 {
		reg.Add(tools.Tool{
			Name:        fmt.Sprintf("tool-%03d", i),
			Description: "test tool",
			Call:        func(args map[string]any) (any, error) { return nil, nil },
		})
	}

	// First page: no cursor. Should get 100 tools + nextCursor.
	_, resp := post(t, s, "tools/list", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	raw, _ := json.Marshal(resp.Result)
	var lr protocol.ListResult
	_ = json.Unmarshal(raw, &lr)
	if len(lr.Tools) != 100 {
		t.Fatalf("page 1: got %d tools, want 100", len(lr.Tools))
	}
	if lr.NextCursor == "" {
		t.Fatal("page 1: expected nextCursor")
	}

	// Second page: pass the cursor. Should get 6 tools, no nextCursor.
	_, resp = post(t, s, "tools/list",
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{"cursor":"`+lr.NextCursor+`"}}`)
	raw, _ = json.Marshal(resp.Result)
	lr = protocol.ListResult{} // reset — Unmarshal doesn't clear absent fields
	_ = json.Unmarshal(raw, &lr)
	if len(lr.Tools) != 6 {
		t.Fatalf("page 2: got %d tools, want 6", len(lr.Tools))
	}
	if lr.NextCursor != "" {
		t.Fatalf("page 2: expected empty nextCursor, got %q", lr.NextCursor)
	}
}

// tools/list includes title and icons when set on the tool.
func TestListIncludesTitleAndIcons(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Add(tools.Tool{
		Name:        "weather",
		Title:       "Weather",
		Description: "Get weather info",
		Icons: []protocol.Icon{
			{Src: "https://example.com/icon.png", MimeType: "image/png", Sizes: []string{"48x48"}},
		},
		Call: func(args map[string]any) (any, error) { return "ok", nil },
	})
	s := New(Config{Stateless: true, Registry: reg})

	_, resp := post(t, s, "tools/list", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	raw, _ := json.Marshal(resp.Result)
	var lr protocol.ListResult
	_ = json.Unmarshal(raw, &lr)
	if len(lr.Tools) != 2 {
		t.Fatalf("expected 2 tools (echo + weather), got %d", len(lr.Tools))
	}
	// Find the weather tool (the other is echo).
	var weather protocol.Tool
	for _, tool := range lr.Tools {
		if tool.Name == "weather" {
			weather = tool
			break
		}
	}
	if weather.Name == "" {
		t.Fatal("weather tool not found in listing")
	}
	if weather.Title != "Weather" {
		t.Fatalf("title = %q, want %q", weather.Title, "Weather")
	}
	if len(weather.Icons) != 1 {
		t.Fatalf("expected 1 icon, got %d", len(weather.Icons))
	}
	if weather.Icons[0].Src != "https://example.com/icon.png" {
		t.Fatalf("icon src = %q", weather.Icons[0].Src)
	}
}
