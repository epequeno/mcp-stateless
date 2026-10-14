// Package tools holds the server's tool registry.
//
// This is deliberately the only place a tool is described. The follow-on
// governance project classifies exactly these entries by risk and attaches
// provenance to them, so keep descriptions here complete rather than
// scattering them into handlers.
package tools

import (
	"sort"
	"sync"

	"github.com/epequeno/mcp-stateless/internal/protocol"
)

// Risk classes. The governance layer requires every reachable tool to carry
// exactly one; an empty Risk is a bug, not a default.
const (
	RiskRead         = "read"
	RiskWrite        = "write"
	RiskEgress       = "egress"
	RiskIrreversible = "irreversible"
)

// Tool is one callable unit. Call must be safe for concurrent use.
type Tool struct {
	Name        string
	Title       string
	Description string
	ReadOnly    bool
	// Async marks a tool for asynchronous task execution. When true and the
	// client declares the tasks extension capability, the server returns a
	// task handle instead of blocking on Call.
	Async bool
	// Risk is consumed by the governance layer (see the Risk* constants).
	Risk string
	// Icons is optional display metadata (spec: basic §icons).
	Icons []protocol.Icon
	Call  func(args map[string]any) (any, error)
}

// Registry is a concurrency-safe set of tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry returns a registry with the builtins registered.
func NewRegistry() *Registry {
	r := &Registry{tools: make(map[string]Tool)}
	r.Add(Tool{
		Name:        "echo",
		Description: "Return the arguments unchanged.",
		ReadOnly:    true,
		Risk:        RiskRead,
		Call: func(args map[string]any) (any, error) {
			if args == nil {
				return map[string]any{}, nil
			}
			return args, nil
		},
	})
	return r
}

// Add registers or replaces a tool.
func (r *Registry) Add(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name] = t
}

// Get looks up a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Names returns registered names in sorted order. Sorted because the listing
// feeds a cache hint: unstable order would defeat it.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Count returns how many tools are registered.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}
