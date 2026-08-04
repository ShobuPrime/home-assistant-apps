package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Memory is a MuninnDB client covering the two calls this proxy needs:
// ACTIVATE (recall) and store-engram.
//
// Every method is safe for concurrent use and fails OPEN: if MuninnDB is slow,
// down, or not installed at all, inference must continue unaffected. A voice
// assistant that stops answering because its memory store is unreachable is a
// worse outcome than one that briefly forgets.
type Memory struct {
	baseURL string
	vault   string
	apiKey  string
	client  *http.Client

	recallCount  int
	recallBudget int // characters, not tokens (see budgetChars)

	// Circuit breaker. Without one, an unreachable MuninnDB would make every
	// single turn pay the full dial timeout.
	mu           sync.Mutex
	failures     int
	disabledTill time.Time

	// Bounds in-flight background writes so a burst of traffic can't spawn
	// unbounded goroutines.
	writeSem chan struct{}

	// Rate-limited warning rather than sync.Once. Logging only the FIRST
	// failure means a memory backend that breaks later, or breaks
	// intermittently, produces no further evidence at all — which is how a
	// recall path that silently returned nothing went unnoticed.
	warnMu   sync.Mutex
	lastWarn time.Time

	storeOK   atomic.Int64
	storeFail atomic.Int64
	recallOK  atomic.Int64
	recallErr atomic.Int64
}

const (
	// Consecutive failures before the breaker opens.
	breakerThreshold = 3
	// How long to stay open before probing again.
	breakerCooldown = 60 * time.Second
	// Rough chars-per-token for budgeting injected context.
	charsPerToken = 4
	// Minimum gap between repeated memory warnings.
	warnInterval = 5 * time.Minute
)

func NewMemory(baseURL, vault, apiKey string, recallCount, recallTokenBudget int, timeout time.Duration) *Memory {
	return &Memory{
		baseURL: strings.TrimRight(baseURL, "/"),
		vault:   vault,
		apiKey:  apiKey,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        16,
				MaxIdleConnsPerHost: 8,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		recallCount:  recallCount,
		recallBudget: recallTokenBudget * charsPerToken,
		writeSem:     make(chan struct{}, 4),
	}
}

func (m *Memory) available() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return time.Now().After(m.disabledTill)
}

func (m *Memory) recordResult(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil {
		m.failures = 0
		return
	}
	m.failures++
	if m.failures >= breakerThreshold {
		m.disabledTill = time.Now().Add(breakerCooldown)
		m.failures = 0
		logf("memory: %d consecutive failures, pausing memory for %s (inference continues normally)",
			breakerThreshold, breakerCooldown)
	}
}

// warn logs at most once per warnInterval. Failures are expected to be
// transient and per-request, so logging every one would flood the app log —
// but logging only the first (the previous behaviour) hides everything after
// it, including a backend that stops working entirely.
func (m *Memory) warn(format string, args ...any) {
	m.warnMu.Lock()
	defer m.warnMu.Unlock()
	if time.Since(m.lastWarn) < warnInterval {
		return
	}
	m.lastWarn = time.Now()
	logf(format, args...)
}

func (m *Memory) do(ctx context.Context, method, path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s%s?vault=%s", m.baseURL, path, urlQueryEscape(m.vault))
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.apiKey)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Cap the read: a malformed/hostile response must not exhaust memory.
	out, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("muninndb %s %s: HTTP %d: %s",
			method, path, resp.StatusCode, truncate(string(out), 200))
	}
	return out, nil
}

// engram is the subset of MuninnDB's response we use. The API returns more
// (confidence, relevance, associations); we only need something to show the
// model.
type engram struct {
	Concept string `json:"concept"`
	Content string `json:"content"`
}

// activateResponse mirrors MuninnDB's real /api/activate body. The top-level
// key is "activations" — verified against a running 0.9.0 server, whose
// response keys are: activations, brief, latency_ms, query_id, total_found.
//
// This previously listed only "engrams" and "results", which do not exist in
// the API at all. encoding/json silently ignores unknown keys, so Recall
// always decoded zero results and returned "" without error: memory appeared
// healthy — engrams were being stored, activate was returning matches, nothing
// logged — while nothing was ever injected into a prompt. The unit tests
// passed because the fake server returned the same invented shape.
//
// "engrams"/"results" are kept only as a fallback in case the key is renamed
// again; "activations" is the one that matters.
type activateResponse struct {
	Activations []engram `json:"activations"`
	Engrams     []engram `json:"engrams"`
	Results     []engram `json:"results"`
}

// Recall returns memories relevant to the given context, formatted as a block
// suitable for prepending to a system prompt. Returns "" when memory is
// unavailable, disabled, or simply has nothing relevant.
func (m *Memory) Recall(ctx context.Context, contextText string) string {
	if m == nil || !m.available() || strings.TrimSpace(contextText) == "" {
		return ""
	}

	raw, err := m.do(ctx, http.MethodPost, "/api/activate", map[string]any{
		"context":     []string{contextText},
		"max_results": m.recallCount,
	})
	m.recordResult(err)
	if err != nil {
		m.recallErr.Add(1)
		m.warn("memory: recall failed (%v) — continuing without memory. "+
			"Check memory_url/memory_vault, and that the MuninnDB app is running.", err)
		return ""
	}

	var parsed activateResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		m.recallErr.Add(1)
		return ""
	}
	found := parsed.Activations
	if len(found) == 0 {
		found = parsed.Engrams
	}
	if len(found) == 0 {
		found = parsed.Results
	}
	if len(found) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Relevant notes from earlier conversations:\n")
	used := 0
	kept := 0
	for _, e := range found {
		line := strings.TrimSpace(e.Content)
		if line == "" {
			continue
		}
		if c := strings.TrimSpace(e.Concept); c != "" {
			line = c + ": " + line
		}
		line = collapseWhitespace(line)
		// Budget is a hard cap: on a 230M model with a 4096-token window,
		// unbounded injection would crowd out the actual conversation.
		if used+len(line) > m.recallBudget {
			if kept == 0 && m.recallBudget > 16 {
				line = line[:m.recallBudget-1] // keep at least one, truncated
			} else {
				break
			}
		}
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteString("\n")
		used += len(line)
		kept++
	}
	if kept == 0 {
		return ""
	}
	m.recallOK.Add(1)
	return b.String()
}

// Store writes one exchange to MuninnDB in the background. It never blocks the
// response path; if the write queue is full the exchange is dropped rather
// than delaying the caller.
func (m *Memory) Store(userText, assistantText, model string) {
	if m == nil || !m.available() {
		return
	}
	userText = strings.TrimSpace(userText)
	assistantText = strings.TrimSpace(assistantText)
	if userText == "" || assistantText == "" {
		return
	}

	select {
	case m.writeSem <- struct{}{}:
	default:
		m.storeFail.Add(1)
		return // queue full; drop rather than block inference
	}

	go func() {
		defer func() { <-m.writeSem }()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		concept := collapseWhitespace(userText)
		if len(concept) > 80 {
			concept = concept[:80]
		}
		content := fmt.Sprintf("User asked: %s\nAssistant answered: %s",
			collapseWhitespace(userText), collapseWhitespace(assistantText))

		_, err := m.do(ctx, http.MethodPost, "/api/engrams", map[string]any{
			"concept":    concept,
			"content":    content,
			"tags":       []string{"home-assistant", "lemonade", model},
			"confidence": 0.7,
		})
		m.recordResult(err)
		if err != nil {
			m.storeFail.Add(1)
			m.warn("memory: store failed (%v) — continuing without memory.", err)
			return
		}
		m.storeOK.Add(1)
	}()
}

func (m *Memory) Stats() string {
	return fmt.Sprintf("recall ok=%d err=%d, store ok=%d dropped=%d",
		m.recallOK.Load(), m.recallErr.Load(), m.storeOK.Load(), m.storeFail.Load())
}
