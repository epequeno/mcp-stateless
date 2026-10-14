package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/epequeno/mcp-stateless/internal/protocol"
	"github.com/epequeno/mcp-stateless/internal/tasks"
	"github.com/epequeno/mcp-stateless/internal/tools"
)

// postWithTasks is like post but includes the tasks extension capability in
// _meta.clientCapabilities, so the server may return a CreateTaskResult.
func postWithTasks(t *testing.T, h http.Handler, header, body string) (*httptest.ResponseRecorder, protocol.Response) {
	t.Helper()
	body = withMetaTasks(t, body)
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

// withMetaTasks injects _meta with the tasks extension capability into the
// body. Unlike withMeta, it always overwrites _meta to ensure the tasks
// extension is present.
func withMetaTasks(t *testing.T, body string) string {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("withMetaTasks: invalid body %q: %v", body, err)
	}
	meta := map[string]any{
		"io.modelcontextprotocol/protocolVersion": protocol.Revision,
		"io.modelcontextprotocol/clientCapabilities": map[string]any{
			"extensions": map[string]any{
				"io.modelcontextprotocol/tasks": map[string]any{},
			},
		},
	}
	params, ok := raw["params"]
	if !ok {
		raw["params"] = map[string]any{"_meta": meta}
	} else {
		pmap, ok := params.(map[string]any)
		if !ok {
			t.Fatalf("withMetaTasks: params is not an object")
		}
		pmap["_meta"] = meta
		raw["params"] = pmap
	}
	b, _ := json.Marshal(raw)
	return string(b)
}

// TestTaskCreateViaToolsCall verifies that tools/call returns a CreateTaskResult
// (resultType: "task") when the tool is async and the client declares the
// tasks extension.
func TestTaskCreateViaToolsCall(t *testing.T) {
	registry := tools.NewRegistry()
	done := make(chan struct{})
	registry.Add(tools.Tool{
		Name:  "slow",
		Async: true,
		Call: func(args map[string]any) (any, error) {
			<-done
			return "done-value", nil
		},
	})
	store := tasks.NewMemoryStore()
	s := New(Config{Stateless: true, Registry: registry, Tasks: store})
	defer close(done)

	// Call tools/call with the tasks extension capability.
	_, resp := postWithTasks(t, s, "tools/call",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"slow","arguments":{}}}`)
	if resp.Error != nil {
		t.Fatalf("create error: %+v", resp.Error)
	}

	// The result should be a CreateTaskResult with resultType: "task".
	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var ctr protocol.CreateTaskResult
	if err := json.Unmarshal(raw, &ctr); err != nil {
		t.Fatalf("unmarshal CreateTaskResult: %v (raw: %s)", err, raw)
	}
	if ctr.ResultType != protocol.ResultTask {
		t.Fatalf("resultType = %q, want %q", ctr.ResultType, protocol.ResultTask)
	}
	if ctr.TaskID == "" {
		t.Fatal("task has no taskId")
	}
	if ctr.Status != protocol.TaskWorking {
		t.Fatalf("initial status = %q, want %q", ctr.Status, protocol.TaskWorking)
	}
}

// TestTaskCreateAndGet exercises the full lifecycle: create returns a handle,
// and polling eventually sees the completed result.
func TestTaskCreateAndGet(t *testing.T) {
	registry := tools.NewRegistry()
	done := make(chan struct{})
	registry.Add(tools.Tool{
		Name:  "slow",
		Async: true,
		Call: func(args map[string]any) (any, error) {
			<-done
			return "done-value", nil
		},
	})
	store := tasks.NewMemoryStore()
	s := New(Config{Stateless: true, Registry: registry, Tasks: store})

	// Create the task via tools/call.
	_, resp := postWithTasks(t, s, "tools/call",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"slow","arguments":{}}}`)
	if resp.Error != nil {
		t.Fatalf("create error: %+v", resp.Error)
	}

	raw, _ := json.Marshal(resp.Result)
	var ctr protocol.CreateTaskResult
	_ = json.Unmarshal(raw, &ctr)

	// While the tool is blocked, get should show working status.
	_, resp = post(t, s, "tasks/get",
		`{"jsonrpc":"2.0","id":2,"method":"tasks/get","params":{"taskId":"`+ctr.TaskID+`"}}`)
	if resp.Error != nil {
		t.Fatalf("get error: %+v", resp.Error)
	}

	result := mustMap(t, resp.Result)
	if result["status"] == protocol.TaskCompleted {
		t.Fatal("task completed before the tool returned — race in the test")
	}

	// Release the tool and wait for completion.
	close(done)
	result = waitForStatus(t, s, ctr.TaskID, protocol.TaskCompleted, 2*time.Second)
	if result["result"] != "done-value" {
		t.Fatalf("result = %v, want done-value", result["result"])
	}
}

// TestTaskGetMissing verifies a get for an unknown taskId is an error.
func TestTaskGetMissing(t *testing.T) {
	s := New(Config{Stateless: true})

	_, resp := post(t, s, "tasks/get",
		`{"jsonrpc":"2.0","id":1,"method":"tasks/get","params":{"taskId":"nonexistent"}}`)
	if resp.Error == nil {
		t.Fatal("expected error for missing task")
	}
}

// TestTaskCancel exercises cancelling a running task.
func TestTaskCancel(t *testing.T) {
	registry := tools.NewRegistry()
	done := make(chan struct{})
	registry.Add(tools.Tool{
		Name:  "blocker",
		Async: true,
		Call: func(args map[string]any) (any, error) {
			<-done
			return nil, nil
		},
	})
	store := tasks.NewMemoryStore()
	s := New(Config{Stateless: true, Registry: registry, Tasks: store})
	defer close(done)

	_, resp := postWithTasks(t, s, "tools/call",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"blocker","arguments":{}}}`)
	raw, _ := json.Marshal(resp.Result)
	var ctr protocol.CreateTaskResult
	_ = json.Unmarshal(raw, &ctr)

	// Cancel the task.
	_, resp = post(t, s, "tasks/cancel",
		`{"jsonrpc":"2.0","id":2,"method":"tasks/cancel","params":{"taskId":"`+ctr.TaskID+`"}}`)
	if resp.Error != nil {
		t.Fatalf("cancel error: %+v", resp.Error)
	}

	// The cancel result is an empty result with resultType: complete.
	result := mustMap(t, resp.Result)
	if result["resultType"] != protocol.ResultComplete {
		t.Fatalf("resultType = %v, want %q", result["resultType"], protocol.ResultComplete)
	}

	// Verify the task is cancelled.
	result = waitForStatus(t, s, ctr.TaskID, protocol.TaskCancelled, time.Second)
	if result["status"] != protocol.TaskCancelled {
		t.Fatalf("status = %v, want %q", result["status"], protocol.TaskCancelled)
	}
}

// TestTaskCancelAlreadyTerminal verifies cancelling a completed task returns
// an empty ack (cooperative — the task stays as it was).
func TestTaskCancelAlreadyTerminal(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Add(tools.Tool{
		Name:  "fast",
		Async: true,
		Call:  func(args map[string]any) (any, error) { return 42, nil },
	})
	store := tasks.NewMemoryStore()
	s := New(Config{Stateless: true, Registry: registry, Tasks: store})

	_, resp := postWithTasks(t, s, "tools/call",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"fast","arguments":{}}}`)
	raw, _ := json.Marshal(resp.Result)
	var ctr protocol.CreateTaskResult
	_ = json.Unmarshal(raw, &ctr)
	waitForStatus(t, s, ctr.TaskID, protocol.TaskCompleted, time.Second)

	_, resp = post(t, s, "tasks/cancel",
		`{"jsonrpc":"2.0","id":2,"method":"tasks/cancel","params":{"taskId":"`+ctr.TaskID+`"}}`)
	if resp.Error != nil {
		t.Fatalf("cancel of terminal task should not error: %+v", resp.Error)
	}

	// Verify the task is still completed (cancel doesn't mutate terminal state).
	result := waitForStatus(t, s, ctr.TaskID, protocol.TaskCompleted, time.Second)
	if result["status"] != protocol.TaskCompleted {
		t.Fatalf("status = %v, want completed (cancel must not mutate terminal state)", result["status"])
	}
}

// TestTaskSharedStoreCrossInstance tests the thesis: two server instances
// sharing one store. A task created on instance A is visible to a get on
// instance B.
func TestTaskSharedStoreCrossInstance(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Add(tools.Tool{
		Name:  "fast",
		Async: true,
		Call:  func(args map[string]any) (any, error) { return "ok", nil },
	})
	store := tasks.NewMemoryStore()

	instanceA := New(Config{Stateless: true, Registry: registry, Tasks: store})
	instanceB := New(Config{Stateless: true, Registry: registry, Tasks: store})

	// Create on A.
	_, resp := postWithTasks(t, instanceA, "tools/call",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"fast","arguments":{}}}`)
	if resp.Error != nil {
		t.Fatalf("create error: %+v", resp.Error)
	}
	raw, _ := json.Marshal(resp.Result)
	var ctr protocol.CreateTaskResult
	_ = json.Unmarshal(raw, &ctr)

	// Poll on B — same store, different instance, no session.
	result := waitForStatus(t, instanceB, ctr.TaskID, protocol.TaskCompleted, time.Second)
	if result["result"] != "ok" {
		t.Fatalf("result from B = %v, want ok", result["result"])
	}
}

// TestTaskUpdate verifies tasks/update acknowledges input.
func TestTaskUpdate(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Add(tools.Tool{
		Name:  "fast",
		Async: true,
		Call:  func(args map[string]any) (any, error) { return "ok", nil },
	})
	store := tasks.NewMemoryStore()
	s := New(Config{Stateless: true, Registry: registry, Tasks: store})

	_, resp := postWithTasks(t, s, "tools/call",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"fast","arguments":{}}}`)
	raw, _ := json.Marshal(resp.Result)
	var ctr protocol.CreateTaskResult
	_ = json.Unmarshal(raw, &ctr)

	_, resp = post(t, s, "tasks/update",
		`{"jsonrpc":"2.0","id":2,"method":"tasks/update","params":{"taskId":"`+ctr.TaskID+`"}}`)
	if resp.Error != nil {
		t.Fatalf("update error: %+v", resp.Error)
	}
	result := mustMap(t, resp.Result)
	if result["resultType"] != protocol.ResultComplete {
		t.Fatalf("resultType = %v, want %q", result["resultType"], protocol.ResultComplete)
	}
}

// TestDiscoverAdvertisesTasks verifies the tasks extension is advertised.
func TestDiscoverAdvertisesTasks(t *testing.T) {
	s := New(Config{Stateless: true})

	_, resp := post(t, s, "server/discover", `{"jsonrpc":"2.0","id":1,"method":"server/discover"}`)
	raw := mustJSON(t, resp.Result)
	var init struct {
		Capabilities map[string]any `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &init); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	exts, ok := init.Capabilities["extensions"].(map[string]any)
	if !ok {
		t.Fatal("no extensions in capabilities")
	}
	if _, ok := exts["io.modelcontextprotocol/tasks"]; !ok {
		t.Fatal("discover does not advertise tasks extension")
	}
}

// TestSyncCallWithoutTasksExtension verifies that a tool marked Async still
// calls synchronously when the client does NOT declare the tasks extension.
func TestSyncCallWithoutTasksExtension(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Add(tools.Tool{
		Name:  "slow",
		Async: true,
		Call:  func(args map[string]any) (any, error) { return "sync-result", nil },
	})
	s := New(Config{Stateless: true, Registry: registry})

	// Call without tasks extension — post() uses empty clientCapabilities.
	_, resp := post(t, s, "tools/call",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"slow","arguments":{}}}`)
	if resp.Error != nil {
		t.Fatalf("call error: %+v", resp.Error)
	}

	// Should get a synchronous CallResult, not a task.
	raw, _ := json.Marshal(resp.Result)
	var cr protocol.CallResult
	if err := json.Unmarshal(raw, &cr); err != nil {
		// Check if it's a task result instead.
		var ctr protocol.CreateTaskResult
		if err2 := json.Unmarshal(raw, &ctr); err2 == nil && ctr.ResultType == protocol.ResultTask {
			t.Fatal("server returned a task for a client without tasks extension")
		}
		t.Fatalf("unmarshal: %v (raw: %s)", err, raw)
	}
	if cr.ResultType != protocol.ResultComplete {
		t.Fatalf("resultType = %q, want %q", cr.ResultType, protocol.ResultComplete)
	}
}

// mustMap marshals and unmarshals a result into a map for field access.
func mustMap(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v (raw: %s)", err, raw)
	}
	return m
}

// mustJSON marshals a result to bytes.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	return b
}

// waitForStatus polls tasks/get until the task reaches the wanted status or
// the deadline expires. Returns the final result map.
func waitForStatus(t *testing.T, s *Server, taskID, want string, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, resp := post(t, s, "tasks/get",
			`{"jsonrpc":"2.0","id":0,"method":"tasks/get","params":{"taskId":"`+taskID+`"}}`)
		if resp.Error != nil {
			t.Fatalf("get error during poll: %+v", resp.Error)
		}
		result := mustMap(t, resp.Result)
		if result["status"] == want {
			return result
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("task %s did not reach %s within %s", taskID, want, timeout)
	return nil
}
