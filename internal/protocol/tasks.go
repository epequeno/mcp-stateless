package protocol

import (
	"encoding/json"
	"time"
)

// Task lifecycle statuses per SEP-2663 (io.modelcontextprotocol/tasks).
// A task moves working → one of completed, failed, or cancelled.
// The input_required status is used when the server needs client input
// via tasks/update.
const (
	TaskWorking       = "working"
	TaskInputRequired = "input_required"
	TaskCompleted     = "completed"
	TaskFailed        = "failed"
	TaskCancelled     = "cancelled"
)

// Task is the wire representation of one asynchronous tool invocation.
//
// Per SEP-2663, tasks are server-directed: a server returns a CreateTaskResult
// (resultType: "task") from tools/call instead of a separate creation method.
// The client polls tasks/get, may send input via tasks/update, and cancels
// via tasks/cancel.
type Task struct {
	TaskID         string    `json:"taskId"`
	Status         string    `json:"status"`
	StatusMessage  string    `json:"statusMessage,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	LastUpdatedAt  time.Time `json:"lastUpdatedAt"`
	TTLMs          int       `json:"ttlMs"`
	PollIntervalMs int       `json:"pollIntervalMs,omitempty"`
}

// CreateTaskResult is returned from tools/call when the server chooses to
// process asynchronously. resultType is "task".
// Spec: Tasks §Task Creation.
type CreateTaskResult struct {
	ResultType string `json:"resultType"`
	Task
}

// CompletedTask is a task that finished successfully; the result field
// carries the original request's result shape (e.g. CallToolResult).
type CompletedTask struct {
	Task
	Result any `json:"result"`
}

// FailedTask is a task that failed with a JSON-RPC error.
type FailedTask struct {
	Task
	Error *Error `json:"error"`
}

// GetTaskParams is the tasks/get payload.
type GetTaskParams struct {
	TaskID string `json:"taskId"`
}

// UpdateTaskParams is the tasks/update payload.
type UpdateTaskParams struct {
	TaskID         string          `json:"taskId"`
	InputResponses json.RawMessage `json:"inputResponses,omitempty"`
}

// CancelTaskParams is the tasks/cancel payload.
type CancelTaskParams struct {
	TaskID string `json:"taskId"`
}
