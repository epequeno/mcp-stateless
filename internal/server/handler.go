// Package server wires the stateless MCP core to net/http.
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/epequeno/mcp-stateless/internal/protocol"
	"github.com/epequeno/mcp-stateless/internal/tools"
)

const (
	// version is this server's own version string.
	version = "0.1.0-scaffold"

	// headerSessionID is the session header. A stateless core neither reads
	// nor sets it; the pinned mode requires it. That difference is the whole
	// point of the comparison in bench/.
	headerSessionID = "Mcp-Session-Id"

	// listTTL is the caching hint advertised on tools/list. The registry is
	// fixed for the process lifetime, so a client may reuse a listing.
	listTTL = 5 * time.Minute
)

// Config configures a Server.
type Config struct {
	// Stateless selects the 2026-07-28 core. When false the server emulates
	// the session-pinned shape so the benchmark compares like with like.
	Stateless bool

	// Registry holds the callable tools. Nil gets a default registry.
	Registry *tools.Registry

	// Logger receives request logs. Nil gets slog.Default.
	Logger *slog.Logger
}

// Server serves MCP over HTTP.
type Server struct {
	cfg Config
}

// New returns a Server, filling in defaults for nil fields.
func New(cfg Config) *Server {
	if cfg.Registry == nil {
		cfg.Registry = tools.NewRegistry()
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Server{cfg: cfg}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Pinned mode is the counterfactual: it needs a session, and therefore
	// needs affinity at the load balancer. The stateless core needs neither.
	if !s.cfg.Stateless && r.Header.Get(headerSessionID) == "" {
		http.Error(w, "session required", http.StatusBadRequest)
		return
	}

	var req protocol.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.write(w, protocol.Response{
			JSONRPC: protocol.JSONRPCVersion,
			Error:   &protocol.Error{Code: protocol.CodeParse, Message: "parse error"},
		})
		return
	}

	method := protocol.MethodFrom(r.Header.Get(protocol.HeaderMethod), &req)
	s.cfg.Logger.Debug("request", "method", method, "stateless", s.cfg.Stateless)

	switch method {
	case "initialize":
		s.write(w, protocol.Response{
			JSONRPC: protocol.JSONRPCVersion,
			ID:      req.ID,
			Result:  s.initialize(),
		})
	case "tools/list":
		s.write(w, protocol.Response{
			JSONRPC: protocol.JSONRPCVersion,
			ID:      req.ID,
			Result:  s.list(),
		})
	case "tools/call":
		result, rpcErr := s.call(req.Params)
		s.write(w, protocol.Response{
			JSONRPC: protocol.JSONRPCVersion,
			ID:      req.ID,
			Result:  result,
			Error:   rpcErr,
		})
	default:
		s.write(w, protocol.Response{
			JSONRPC: protocol.JSONRPCVersion,
			ID:      req.ID,
			Error: &protocol.Error{
				Code:    protocol.CodeMethodNotFound,
				Message: "method not found: " + method,
			},
		})
	}
}

// initialize answers without creating anything. A stateless core has no session
// to hand back, and must not invent one to look stateful.
func (s *Server) initialize() map[string]any {
	return map[string]any{
		"protocolVersion": protocol.Revision,
		"serverInfo":      map[string]any{"name": "mcp-stateless", "version": version},
		"capabilities":    map[string]any{"tools": map[string]any{}},
	}
}

func (s *Server) list() protocol.ListResult {
	names := s.cfg.Registry.Names()
	out := make([]protocol.Tool, 0, len(names))
	for _, name := range names {
		t, ok := s.cfg.Registry.Get(name)
		if !ok {
			continue
		}
		out = append(out, protocol.Tool{
			Name:        t.Name,
			Description: t.Description,
			ReadOnly:    t.ReadOnly,
		})
	}
	return protocol.ListResult{
		Tools:  out,
		TTLMs:  int(listTTL.Milliseconds()),
		Cached: false,
	}
}

func (s *Server) call(raw json.RawMessage) (any, *protocol.Error) {
	var params protocol.CallParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, &protocol.Error{Code: protocol.CodeInvalidParams, Message: "invalid params"}
		}
	}

	tool, ok := s.cfg.Registry.Get(params.Name)
	if !ok {
		return nil, &protocol.Error{
			Code:    protocol.CodeInvalidParams,
			Message: "unknown tool: " + params.Name,
		}
	}

	v, err := tool.Call(params.Arguments)
	if err != nil {
		return nil, &protocol.Error{Code: protocol.CodeInternal, Message: err.Error()}
	}

	return protocol.CallResult{
		Content: []protocol.Content{{Type: "text", Text: toText(v)}},
	}, nil
}

func (s *Server) write(w http.ResponseWriter, resp protocol.Response) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.cfg.Logger.Error("encode failed", "err", err)
	}
}

func toText(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
