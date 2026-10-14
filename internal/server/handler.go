// Package server wires the 2026-07-28 MCP core to net/http.
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/epequeno/mcp-stateless/internal/protocol"
	"github.com/epequeno/mcp-stateless/internal/tasks"
	"github.com/epequeno/mcp-stateless/internal/tools"
)

const (
	version = "0.2.0"

	headerSessionID = "Mcp-Session-Id"

	listTTL = 5 * time.Minute
)

// Config configures a Server.
type Config struct {
	Stateless bool
	Registry  *tools.Registry
	Tasks     tasks.Store
	Logger    *slog.Logger
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
	if cfg.Tasks == nil {
		cfg.Tasks = tasks.NewMemoryStore()
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

	// Stateless mode: the spec requires Mcp-Method and MCP-Protocol-Version
	// headers. The pinned mode (counterfactual) does not enforce them.
	if s.cfg.Stateless {
		if !protocol.HeaderPresent(r.Header.Get(protocol.HeaderMethod)) {
			s.write(w, protocol.Response{
				JSONRPC: protocol.JSONRPCVersion,
				ID:      req.ID,
				Error: &protocol.Error{
					Code:    protocol.CodeHeaderMismatch,
					Message: "Mcp-Method header required",
				},
			})
			return
		}
		if r.Header.Get(protocol.HeaderProtocolVersion) == "" {
			s.write(w, protocol.Response{
				JSONRPC: protocol.JSONRPCVersion,
				ID:      req.ID,
				Error: &protocol.Error{
					Code:    protocol.CodeHeaderMismatch,
					Message: "MCP-Protocol-Version header required",
				},
			})
			return
		}
	}

	// Parse _meta from params. Every request MUST carry it in the stateless
	// core (spec: basic §_meta).
	meta := s.parseMeta(req.Params)

	if s.cfg.Stateless && meta != nil {
		hdrVer := r.Header.Get(protocol.HeaderProtocolVersion)
		if hdrVer != "" && meta.ProtocolVersion != "" && hdrVer != meta.ProtocolVersion {
			s.write(w, protocol.Response{
				JSONRPC: protocol.JSONRPCVersion,
				ID:      req.ID,
				Error: &protocol.Error{
					Code:    protocol.CodeHeaderMismatch,
					Message: "protocol version mismatch: header=" + hdrVer + " meta=" + meta.ProtocolVersion,
				},
			})
			return
		}
		if meta.ProtocolVersion == "" {
			s.write(w, protocol.Response{
				JSONRPC: protocol.JSONRPCVersion,
				ID:      req.ID,
				Error: &protocol.Error{
					Code:    protocol.CodeInvalidParams,
					Message: "io.modelcontextprotocol/protocolVersion required in _meta",
				},
			})
			return
		}
	}

	switch method {
	case "server/discover":
		s.write(w, protocol.Response{
			JSONRPC: protocol.JSONRPCVersion,
			ID:      req.ID,
			Result:  s.discover(),
		})
	case "tools/list":
		s.write(w, protocol.Response{
			JSONRPC: protocol.JSONRPCVersion,
			ID:      req.ID,
		Result:  s.list(req.Params),
		})
	case "tools/call":
		result, rpcErr := s.call(req.Params, meta)
		s.write(w, protocol.Response{
			JSONRPC: protocol.JSONRPCVersion,
			ID:      req.ID,
			Result:  result,
			Error:   rpcErr,
		})
	case "tasks/get":
		result, rpcErr := s.getTask(req.Params)
		s.write(w, protocol.Response{
			JSONRPC: protocol.JSONRPCVersion,
			ID:      req.ID,
			Result:  result,
			Error:   rpcErr,
		})
	case "tasks/update":
		result, rpcErr := s.updateTask(req.Params)
		s.write(w, protocol.Response{
			JSONRPC: protocol.JSONRPCVersion,
			ID:      req.ID,
			Result:  result,
			Error:   rpcErr,
		})
	case "tasks/cancel":
		result, rpcErr := s.cancelTask(req.Params)
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

// metaHolder is used to extract _meta from the raw params.
type metaHolder struct {
	Meta *protocol.Meta `json:"_meta,omitempty"`
}

// parseMeta extracts _meta from the request params.
func (s *Server) parseMeta(raw json.RawMessage) *protocol.Meta {
	if len(raw) == 0 {
		return nil
	}
	var mh metaHolder
	if err := json.Unmarshal(raw, &mh); err != nil {
		return nil
	}
	return mh.Meta
}

// clientCapabilities is used to check for the tasks extension.
type clientCapabilities struct {
	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
}

// hasTasksExtension checks whether the client declared the tasks extension.
func hasTasksExtension(meta *protocol.Meta) bool {
	if meta == nil || len(meta.ClientCapabilities) == 0 {
		return false
	}
	var caps clientCapabilities
	if err := json.Unmarshal(meta.ClientCapabilities, &caps); err != nil {
		return false
	}
	_, ok := caps.Extensions["io.modelcontextprotocol/tasks"]
	return ok
}

// discover returns the server/discover result.
// Spec: server/discover §Response.
func (s *Server) discover() protocol.DiscoverResult {
	return protocol.DiscoverResult{
		ResultType:        protocol.ResultComplete,
		SupportedVersions: []string{protocol.Revision},
		Capabilities: map[string]any{
			"tools": map[string]any{},
			"extensions": map[string]any{
				"io.modelcontextprotocol/tasks": map[string]any{},
			},
		},
		TTLMs:      int(listTTL.Milliseconds()),
		CacheScope: protocol.CacheScopePublic,
	}
}

// listPageSize is the max tools returned per page. A registry with more
// tools than this returns a nextCursor for the next page.
const listPageSize = 100

func (s *Server) list(raw json.RawMessage) protocol.ListResult {
	var params protocol.ListParams
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &params) // cursor is optional; ignore error
	}

	names := s.cfg.Registry.Names()

	// Resolve the cursor into a start index. The cursor is the index of
	// the next tool to return; an empty cursor means start from 0.
	start := 0
	if params.Cursor != "" {
		// Find the cursor position: the cursor is a tool name; we start
		// from the tool after it.
		for i, name := range names {
			if name == params.Cursor {
				start = i + 1
				break
			}
		}
		// If the cursor doesn't match any tool, start from 0 (stale cursor).
	}

	end := start + listPageSize
	if end > len(names) {
		end = len(names)
	}

	out := make([]protocol.Tool, 0, end-start)
	for _, name := range names[start:end] {
		t, ok := s.cfg.Registry.Get(name)
		if !ok {
			continue
		}
		out = append(out, protocol.Tool{
			Name:        t.Name,
			Title:       t.Title,
			Description: t.Description,
			Icons:       t.Icons,
		})
	}

	result := protocol.ListResult{
		ResultType: protocol.ResultComplete,
		Tools:      out,
		TTLMs:      int(listTTL.Milliseconds()),
		CacheScope: protocol.CacheScopePublic,
	}

	// If there are more tools, set nextCursor to the last tool name in
	// this page. The client passes it back as cursor on the next call.
	if end < len(names) {
		result.NextCursor = names[end-1]
	}

	return result
}

// call handles tools/call. If the tool is marked Async and the client declared
// the tasks extension, it returns a CreateTaskResult (resultType: "task").
// Otherwise it calls the tool synchronously and returns a CallResult.
func (s *Server) call(raw json.RawMessage, meta *protocol.Meta) (any, *protocol.Error) {
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

	// If the tool is async and the client supports tasks, return a task
	// handle per SEP-2663. The server is the sole decider.
	if tool.Async && hasTasksExtension(meta) {
		args := params.Arguments
		state, err := s.cfg.Tasks.Create(tasks.Run(func() (any, error) {
			return tool.Call(args)
		}))
		if err != nil {
			return nil, &protocol.Error{Code: protocol.CodeInternal, Message: err.Error()}
		}
		return protocol.CreateTaskResult{
			ResultType: protocol.ResultTask,
			Task:       state.Task,
		}, nil
	}

	// Synchronous call.
	v, err := tool.Call(params.Arguments)
	if err != nil {
		return nil, &protocol.Error{Code: protocol.CodeInternal, Message: err.Error()}
	}

	return protocol.CallResult{
		ResultType: protocol.ResultComplete,
		Content:    []protocol.Content{{Type: "text", Text: toText(v)}},
	}, nil
}

// getTask returns the current state of one task.
// Spec: Tasks §Task Polling.
func (s *Server) getTask(raw json.RawMessage) (any, *protocol.Error) {
	var params protocol.GetTaskParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, &protocol.Error{Code: protocol.CodeInvalidParams, Message: "invalid params"}
		}
	}
	if params.TaskID == "" {
		return nil, &protocol.Error{Code: protocol.CodeInvalidParams, Message: "taskId is required"}
	}

	state, err := s.cfg.Tasks.Get(params.TaskID)
	if err != nil {
		return nil, &protocol.Error{Code: protocol.CodeInvalidParams, Message: err.Error()}
	}

	// Build the response based on status. The result includes the task
	// fields plus result/error for terminal states.
	result := map[string]any{
		"resultType": protocol.ResultComplete,
		"taskId":     state.TaskID,
		"status":     state.Status,
		"createdAt":  state.CreatedAt,
		"lastUpdatedAt": state.LastUpdatedAt,
		"ttlMs":      state.TTLMs,
	}
	if state.PollIntervalMs > 0 {
		result["pollIntervalMs"] = state.PollIntervalMs
	}
	if state.Status == protocol.TaskCompleted {
		result["result"] = state.Result
	}
	if state.Status == protocol.TaskFailed && state.Error != nil {
		result["error"] = state.Error
	}
	return result, nil
}

// updateTask acknowledges client input for a task.
// Spec: Tasks §Task Update Requests.
func (s *Server) updateTask(raw json.RawMessage) (any, *protocol.Error) {
	var params protocol.UpdateTaskParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, &protocol.Error{Code: protocol.CodeInvalidParams, Message: "invalid params"}
		}
	}
	if params.TaskID == "" {
		return nil, &protocol.Error{Code: protocol.CodeInvalidParams, Message: "taskId is required"}
	}

	// Verify the task exists.
	_, err := s.cfg.Tasks.Get(params.TaskID)
	if err != nil {
		return nil, &protocol.Error{Code: protocol.CodeInvalidParams, Message: err.Error()}
	}

	// Acknowledge — the spec says the ack is an empty result.
	return map[string]any{
		"resultType": protocol.ResultComplete,
	}, nil
}

// cancelTask signals intent to cancel. Cooperative: the task may still
// complete. The spec says the ack is an empty result.
func (s *Server) cancelTask(raw json.RawMessage) (any, *protocol.Error) {
	var params protocol.CancelTaskParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, &protocol.Error{Code: protocol.CodeInvalidParams, Message: "invalid params"}
		}
	}
	if params.TaskID == "" {
		return nil, &protocol.Error{Code: protocol.CodeInvalidParams, Message: "taskId is required"}
	}

	_, err := s.cfg.Tasks.Cancel(params.TaskID)
	if err != nil {
		return nil, &protocol.Error{Code: protocol.CodeInvalidParams, Message: err.Error()}
	}

	return map[string]any{
		"resultType": protocol.ResultComplete,
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
