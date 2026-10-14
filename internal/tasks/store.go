// Package tasks holds the store behind the Tasks extension surface.
//
// Per SEP-2663, a task is created server-directed: the server returns a
// CreateTaskResult from tools/call, not via a separate tasks/create method.
// The client polls tasks/get, may send input via tasks/update, and cancels
// via tasks/cancel. The store is the seam for where task state lives.
//
// The in-memory MemoryStore is per-process: two server instances behind one
// round-robin load balancer do NOT share it, so a get routed to the "other"
// instance misses. The stateless core's round-robin claim therefore does not
// hold for tasks/* unless a shared store is plugged in. That is an
// intentional, documented limitation — see docs/DECISIONS.md — not a bug to
// paper over.
package tasks

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/epequeno/mcp-stateless/internal/protocol"
)

// ErrNotFound is returned for a task id the store has no record of.
var ErrNotFound = errors.New("task not found")

// Run executes a tool's body and returns its result. The server supplies this
// by resolving the named tool from its registry, so the store stays decoupled
// from the tool layer.
type Run func() (any, error)

// TaskState is the full internal state of a task, including the result or
// error when terminal. The handler uses this to build wire responses.
type TaskState struct {
	protocol.Task
	Result any             // set when Status == TaskCompleted
	Error  *protocol.Error // set when Status == TaskFailed
}

// Store is where task state lives. The interface is the seam: swap MemoryStore
// for a Redis- or database-backed implementation and every instance behind the
// load balancer can answer any poll.
type Store interface {
	// Create records a task, starts run in the background, and returns the
	// initial task state (status: working).
	Create(run Run) (TaskState, error)

	// Get returns the current state of one task.
	Get(taskID string) (TaskState, error)

	// Cancel signals intent to cancel. Cooperative: the task MAY still
	// complete. Returns the current state. A task already terminal is
	// returned as-is (cancel is a no-op on terminal tasks per spec).
	Cancel(taskID string) (TaskState, error)

	// Count returns the number of tasks held. Observability only.
	Count() int
}

// MemoryStore holds tasks in process memory. Safe for concurrent use.
//
// It is deliberately not a shared store: see the package doc.
type MemoryStore struct {
	mu    sync.Mutex
	tasks map[string]*taskSlot
}

type taskSlot struct {
	state TaskState
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tasks: make(map[string]*taskSlot)}
}

// Create implements Store.
func (s *MemoryStore) Create(run Run) (TaskState, error) {
	id := newID()
	now := time.Now()
	slot := &taskSlot{
		state: TaskState{
			Task: protocol.Task{
				TaskID:         id,
				Status:         protocol.TaskWorking,
				CreatedAt:      now,
				LastUpdatedAt:  now,
				TTLMs:          3600000, // 1 hour
				PollIntervalMs: 100,
			},
		},
	}

	s.mu.Lock()
	s.tasks[id] = slot
	s.mu.Unlock()

	if run != nil {
		go s.run(slot, run)
	}
	return slot.state, nil
}

func (s *MemoryStore) run(slot *taskSlot, run Run) {
	result, err := run()

	s.mu.Lock()
	defer s.mu.Unlock()
	// A cancel that landed while we were running wins: stay cancelled and
	// discard the result.
	if slot.state.Status == protocol.TaskCancelled {
		return
	}
	if err != nil {
		slot.state.Status = protocol.TaskFailed
		slot.state.Error = &protocol.Error{
			Code:    protocol.CodeInternal,
			Message: err.Error(),
		}
	} else {
		slot.state.Status = protocol.TaskCompleted
		slot.state.Result = result
	}
	slot.state.LastUpdatedAt = time.Now()
}

// Get implements Store.
func (s *MemoryStore) Get(taskID string) (TaskState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot, ok := s.tasks[taskID]
	if !ok {
		return TaskState{}, ErrNotFound
	}
	return slot.state, nil
}

// Cancel implements Store.
func (s *MemoryStore) Cancel(taskID string) (TaskState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot, ok := s.tasks[taskID]
	if !ok {
		return TaskState{}, ErrNotFound
	}
	// Cooperative: if already terminal, return as-is. The spec says
	// cancellation MAY ultimately reach a terminal status other than
	// cancelled if the work finished first.
	if slot.state.Status == protocol.TaskCompleted ||
		slot.state.Status == protocol.TaskFailed ||
		slot.state.Status == protocol.TaskCancelled {
		return slot.state, nil
	}
	slot.state.Status = protocol.TaskCancelled
	slot.state.LastUpdatedAt = time.Now()
	return slot.state, nil
}

// Count implements Store.
func (s *MemoryStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tasks)
}

// newID returns a short random hex id. Random rather than a counter so a task
// handle carries no information about the instance that created it.
func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
