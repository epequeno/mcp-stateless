// Command bench measures the stateless MCP core against the session-pinned
// counterfactual. One client, one workload, two modes — the same-client rule
// from bench/README.md.
//
// It does not start the servers. Run `just up`, `just up-pinned`, and a second
// stateless instance (for the cross-instance test) first, or let `just bench`
// do it.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/epequeno/mcp-stateless/internal/protocol"
)

const (
	jsonrpc    = protocol.JSONRPCVersion
	sessionHdr = "Mcp-Session-Id"
)

// frame builds one JSON-RPC request body with the required _meta fields.
// Every stateless request MUST carry protocolVersion and clientCapabilities
// in _meta (spec: basic §_meta).
func frame(id int, method string, params any) string {
	return buildBody(id, method, params)
}

// buildBody constructs a complete JSON-RPC body with _meta injected into params.
func buildBody(id int, method string, params any) string {
	meta := map[string]any{
		"io.modelcontextprotocol/protocolVersion":    protocol.Revision,
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}
	var paramsMap map[string]any
	if params != nil {
		raw, _ := json.Marshal(params)
		_ = json.Unmarshal(raw, &paramsMap)
	}
	if paramsMap == nil {
		paramsMap = map[string]any{}
	}
	paramsMap["_meta"] = meta

	body := map[string]any{
		"jsonrpc": protocol.JSONRPCVersion,
		"id":      id,
		"method":  method,
		"params":  paramsMap,
	}
	raw, _ := json.Marshal(body)
	return string(raw)
}

// request sends one JSON-RPC frame and returns the round-trip latency.
// The session header is sent only when pinned is true — that is the one
// behavioural difference the bench exercises. The stateless mode sends the
// required MCP-Protocol-Version and Mcp-Name headers.
func request(client *http.Client, addr, method string, body string, pinned bool) (time.Duration, error) {
	req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/", bytes.NewReader([]byte(body)))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(protocol.HeaderMethod, method)
	req.Header.Set(protocol.HeaderProtocolVersion, protocol.Revision)
	// Mcp-Name is required for tools/call — set to the tool name.
	if method == "tools/call" {
		req.Header.Set(protocol.HeaderName, "echo")
	}
	if pinned {
		req.Header.Set(sessionHdr, "bench-session")
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	elapsed := time.Since(start)

	if resp.StatusCode != http.StatusOK {
		return elapsed, fmt.Errorf("status %d", resp.StatusCode)
	}
	return elapsed, nil
}

// phase measures latency distribution for n sequential requests and returns
// p50, p99 (in milliseconds), and total elapsed.
func phase(name string, client *http.Client, addr, method string, body string, pinned bool, n int) stats {
	latencies := make([]time.Duration, 0, n)
	var firstErr error
	for range n {
		d, err := request(client, addr, method, body, pinned)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		latencies = append(latencies, d)
	}
	if firstErr != nil && len(latencies) == 0 {
		log.Printf("%s: all requests failed: %v", name, firstErr)
		return stats{}
	}
	return compute(name, latencies)
}

// throughputPhase sends n requests at concurrency c and reports req/s.
func throughputPhase(name string, client *http.Client, addr, method string, body string, pinned bool, n, c int) stats {
	var mu sync.Mutex
	latencies := make([]time.Duration, 0, n)
	errCount := 0

	jobs := make(chan int, n)
	var wg sync.WaitGroup
	for range c {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				d, err := request(client, addr, method, body, pinned)
				if err != nil {
					mu.Lock()
					errCount++
					mu.Unlock()
					continue
				}
				mu.Lock()
				latencies = append(latencies, d)
				mu.Unlock()
			}
		}()
	}

	start := time.Now()
	for i := range n {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	total := time.Since(start)

	s := compute(name, latencies)
	if errCount > 0 {
		s.errCount = errCount
	}
	s.throughput = float64(len(latencies)) / total.Seconds()
	s.totalElapsed = total
	return s
}

// crossInstancePhase alternates between two stateless instances and counts how
// many each served. This is the thesis test: any instance can serve any
// request, no session, no affinity.
func crossInstancePhase(client *http.Client, addr1, addr2 string, n int) (served1, served2, failed int) {
	for i := range n {
		addr := addr1
		if i%2 == 1 {
			addr = addr2
		}
		_, err := request(client, addr, "tools/call",
			frame(i, "tools/call", map[string]any{"name": "echo", "arguments": map[string]any{"i": i}}),
			false)
		if err != nil {
			failed++
			continue
		}
		if i%2 == 0 {
			served1++
		} else {
			served2++
		}
	}
	return
}

type stats struct {
	name         string
	count        int
	p50, p99     time.Duration
	min, max     time.Duration
	throughput   float64
	totalElapsed time.Duration
	errCount     int
}

func compute(name string, latencies []time.Duration) stats {
	if len(latencies) == 0 {
		return stats{name: name}
	}
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	return stats{
		name:  name,
		count: len(sorted),
		p50:   sorted[len(sorted)*50/100],
		p99:   sorted[len(sorted)*99/100],
		min:   sorted[0],
		max:   sorted[len(sorted)-1],
	}
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000.0 }

func main() {
	var (
		stateless  = flag.String("stateless", ":8080", "stateless instance 1 address")
		stateless2 = flag.String("stateless2", ":8082", "stateless instance 2 address (for cross-instance test)")
		pinned     = flag.String("pinned", ":8081", "session-pinned address")
		latN       = flag.Int("lat-n", 1000, "sequential requests for latency phase")
		thrN       = flag.Int("thr-n", 5000, "total requests for throughput phase")
		thrC       = flag.Int("thr-c", 10, "concurrency for throughput phase")
		crossN     = flag.Int("cross-n", 200, "requests for cross-instance phase")
	)
	flag.Parse()

	client := &http.Client{Timeout: 10 * time.Second}

	callBody := frame(0, "tools/call", map[string]any{"name": "echo", "arguments": map[string]any{"k": "v"}})
	listBody := frame(0, "tools/list", nil)

	// --- Phase 1: latency (single-writer, sequential) ---
	fmt.Println("## Phase 1: latency (single-writer, sequential)")
	fmt.Println()
	fmt.Println("| Mode | Method | n | p50 (ms) | p99 (ms) | min (ms) | max (ms) |")
	fmt.Println("|---|---|---|---|---|---|---|")

	sLat := phase("stateless/call", client, *stateless, "tools/call", callBody, false, *latN)
	fmt.Printf("| stateless | tools/call | %d | %.3f | %.3f | %.3f | %.3f |\n",
		sLat.count, ms(sLat.p50), ms(sLat.p99), ms(sLat.min), ms(sLat.max))

	sList := phase("stateless/list", client, *stateless, "tools/list", listBody, false, *latN)
	fmt.Printf("| stateless | tools/list | %d | %.3f | %.3f | %.3f | %.3f |\n",
		sList.count, ms(sList.p50), ms(sList.p99), ms(sList.min), ms(sList.max))

	pLat := phase("pinned/call", client, *pinned, "tools/call", callBody, true, *latN)
	fmt.Printf("| pinned | tools/call | %d | %.3f | %.3f | %.3f | %.3f |\n",
		pLat.count, ms(pLat.p50), ms(pLat.p99), ms(pLat.min), ms(pLat.max))

	pList := phase("pinned/list", client, *pinned, "tools/list", listBody, true, *latN)
	fmt.Printf("| pinned | tools/list | %d | %.3f | %.3f | %.3f | %.3f |\n",
		pList.count, ms(pList.p50), ms(pList.p99), ms(pList.min), ms(pList.max))

	// --- Phase 2: throughput at fixed concurrency ---
	fmt.Println()
	fmt.Printf("## Phase 2: throughput (%d requests at concurrency %d)\n", *thrN, *thrC)
	fmt.Println()
	fmt.Println("| Mode | Method | served | errors | throughput (req/s) | p50 (ms) | p99 (ms) |")
	fmt.Println("|---|---|---|---|---|---|---|")

	sThr := throughputPhase("stateless/call", client, *stateless, "tools/call", callBody, false, *thrN, *thrC)
	fmt.Printf("| stateless | tools/call | %d | %d | %.1f | %.3f | %.3f |\n",
		sThr.count, sThr.errCount, sThr.throughput, ms(sThr.p50), ms(sThr.p99))

	pThr := throughputPhase("pinned/call", client, *pinned, "tools/call", callBody, true, *thrN, *thrC)
	fmt.Printf("| pinned | tools/call | %d | %d | %.1f | %.3f | %.3f |\n",
		pThr.count, pThr.errCount, pThr.throughput, ms(pThr.p50), ms(pThr.p99))

	// --- Phase 3: cross-instance serving (the thesis test) ---
	fmt.Println()
	fmt.Printf("## Phase 3: cross-instance serving (%d requests, alternating two instances)\n", *crossN)
	fmt.Println()

	served1, served2, failed := crossInstancePhase(client, *stateless, *stateless2, *crossN)
	fmt.Printf("- instance 1 served: %d\n", served1)
	fmt.Printf("- instance 2 served: %d\n", served2)
	fmt.Printf("- failed: %d\n", failed)
	if failed > 0 {
		fmt.Println("- **FAIL**: some requests failed — the round-robin property does not hold")
		os.Exit(1)
	}
	if served1 == 0 || served2 == 0 {
		fmt.Println("- **WARN**: only one instance served requests — check that both are running")
	} else {
		fmt.Println("- **PASS**: both instances served requests without a session, no affinity")
	}

	// --- Phase 4: tools/list ttlMs cache effect ---
	fmt.Println()
	fmt.Println("## Phase 4: tools/list ttlMs cache hint")
	fmt.Println()

	// Measure a cold tools/list round trip.
	coldLat := phase("list-cold", client, *stateless, "tools/list", listBody, false, 1)
	fmt.Printf("- cold tools/list round trip: %.3f ms\n", ms(coldLat.p50))
	fmt.Printf("- server advertises ttlMs: %d (5 min)\n", int((5 * time.Minute).Milliseconds()))
	fmt.Println("- a client that caches the listing for ttlMs skips the round trip: ~0 ms")
	fmt.Println("- the hint is worth advertising if the listing is stable and the client honours it")

	fmt.Println()
	fmt.Println("## Recording")
	fmt.Println()
	fmt.Println("Paste the table above into RESULTS.md with the commit SHA and date.")
	fmt.Println("A number without its command is not evidence.")
}
