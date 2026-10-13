package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/epequeno/mcp-stateless/internal/protocol"
)

// post issues one JSON-RPC frame, optionally with the stateless routing header.
func post(t *testing.T, h http.Handler, header, body string) (*httptest.ResponseRecorder, protocol.Response) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	if header != "" {
		req.Header.Set(protocol.HeaderMethod, header)
	}

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

// The defining property of the stateless core: it hands back no session.
func TestStatelessCoreIssuesNoSession(t *testing.T) {
	s := New(Config{Stateless: true})

	rec, _ := post(t, s, "initialize", `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
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

func TestListCarriesTTLAndIsNotCached(t *testing.T) {
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
	if lr.Cached {
		t.Fatal("a server must not advertise its own response as cached")
	}
	if len(lr.Tools) == 0 {
		t.Fatal("expected at least one tool")
	}
}

// The counterfactual mode really is stricter, or the benchmark is meaningless.
func TestPinnedModeRequiresSession(t *testing.T) {
	s := New(Config{Stateless: false})

	rec, _ := post(t, s, "initialize", `{"jsonrpc":"2.0","id":5,"method":"initialize"}`)
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
