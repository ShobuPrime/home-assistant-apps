package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("bad json fixture: %v", err)
	}
	return m
}

func TestLastUserText(t *testing.T) {
	tests := []struct {
		name, payload, want string
	}{
		{
			name:    "picks the most recent user turn, not the first",
			payload: `{"messages":[{"role":"user","content":"first"},{"role":"assistant","content":"ok"},{"role":"user","content":"second"}]}`,
			want:    "second",
		},
		{
			name:    "ignores trailing assistant and tool messages",
			payload: `{"messages":[{"role":"user","content":"turn on the light"},{"role":"assistant","content":"done"}]}`,
			want:    "turn on the light",
		},
		{
			name:    "flattens multimodal content, skipping non-text parts",
			payload: `{"messages":[{"role":"user","content":[{"type":"text","text":"what is this"},{"type":"image_url","image_url":{"url":"data:..."}}]}]}`,
			want:    "what is this",
		},
		{
			name:    "no user message",
			payload: `{"messages":[{"role":"system","content":"you are a bot"}]}`,
			want:    "",
		},
		{
			name:    "no messages key at all",
			payload: `{"model":"x"}`,
			want:    "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lastUserText(decode(t, tc.payload)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Home Assistant's system prompt carries the exposed-entity list and tool
// instructions. Replacing it instead of appending would break Assist, so this
// is the single most important behaviour to pin down.
func TestInjectSystemPreservesExistingPrompt(t *testing.T) {
	payload := decode(t, `{"messages":[
		{"role":"system","content":"You are a voice assistant for Home Assistant. Exposed entities: light.kitchen"},
		{"role":"user","content":"turn on the kitchen light"}]}`)

	if !injectSystem(payload, "Relevant notes:\n- user prefers warm light\n") {
		t.Fatal("injectSystem reported no change")
	}

	msgs := payload["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("message count changed: got %d, want 2", len(msgs))
	}
	sys := msgs[0].(map[string]any)["content"].(string)
	if !strings.Contains(sys, "Exposed entities: light.kitchen") {
		t.Error("original system prompt was lost — this would break Assist tool calling")
	}
	if !strings.Contains(sys, "user prefers warm light") {
		t.Error("memory block was not injected")
	}
	if role := msgs[1].(map[string]any)["role"].(string); role != "user" {
		t.Errorf("user message displaced: got role %q", role)
	}
}

func TestInjectSystemAddsOneWhenAbsent(t *testing.T) {
	payload := decode(t, `{"messages":[{"role":"user","content":"hi"}]}`)
	if !injectSystem(payload, "memories") {
		t.Fatal("expected modification")
	}
	msgs := payload["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	first := msgs[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "memories" {
		t.Errorf("expected a leading system message, got %v", first)
	}
}

func TestExtractDelta(t *testing.T) {
	tests := []struct {
		name, flavour, line, want string
	}{
		{"openai sse chunk", "openai", `data: {"choices":[{"delta":{"content":"Hello"}}]}`, "Hello"},
		{"openai done sentinel", "openai", `data: [DONE]`, ""},
		{"openai keepalive blank", "openai", ``, ""},
		{"openai non-data line", "openai", `event: ping`, ""},
		{"openai role-only chunk", "openai", `data: {"choices":[{"delta":{"role":"assistant"}}]}`, ""},
		{"ollama ndjson chunk", "ollama", `{"message":{"content":"Hi"},"done":false}`, "Hi"},
		{"ollama final chunk", "ollama", `{"message":{"content":""},"done":true}`, ""},
		{"garbage never panics", "ollama", `{not json`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractDelta([]byte(tc.line), tc.flavour); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractFull(t *testing.T) {
	openai := `{"choices":[{"message":{"role":"assistant","content":"The light is on."}}]}`
	if got := extractFull([]byte(openai), "openai"); got != "The light is on." {
		t.Errorf("openai: got %q", got)
	}
	ollama := `{"message":{"role":"assistant","content":"Done."},"done":true}`
	if got := extractFull([]byte(ollama), "ollama"); got != "Done." {
		t.Errorf("ollama: got %q", got)
	}
	if got := extractFull([]byte(`{"error":"nope"}`), "openai"); got != "" {
		t.Errorf("error body should yield empty, got %q", got)
	}
}

// fakeMuninn stands in for MuninnDB, recording what got stored.
type fakeMuninn struct {
	*httptest.Server
	mu     sync.Mutex
	stored []map[string]any
	vaults []string
	fail   bool
	delay  time.Duration
}

func newFakeMuninn() *fakeMuninn {
	f := &fakeMuninn{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/activate", func(w http.ResponseWriter, r *http.Request) {
		if f.delay > 0 {
			time.Sleep(f.delay)
		}
		if f.fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		f.mu.Lock()
		f.vaults = append(f.vaults, r.URL.Query().Get("vault"))
		f.mu.Unlock()
		// MUST mirror MuninnDB's real /api/activate body. The top-level key is
		// "activations" (verified against 0.9.0, whose keys are activations,
		// brief, latency_ms, query_id, total_found). An earlier fixture here
		// invented `{"engrams":[...]}`, which does not exist in the API — so
		// every test passed while Recall silently decoded nothing in
		// production. A fake that agrees with your assumption instead of the
		// real service tests nothing.
		_, _ = io.WriteString(w, `{"activations":[{"id":"01ABC","concept":"lighting","content":"user prefers warm light","score":0.61,"confidence":0.7}],"total_found":1,"latency_ms":12,"query_id":"q1","brief":""}`)
	})
	mux.HandleFunc("/api/engrams", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.stored = append(f.stored, body)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"eng_1"}`)
	})
	f.Server = httptest.NewServer(mux)
	return f
}

func (f *fakeMuninn) storedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.stored)
}

func (f *fakeMuninn) waitForStore(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if f.storedCount() >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d stored engrams, got %d", n, f.storedCount())
}

// newTestServer wires the proxy to a caller-supplied upstream handler.
func newTestServer(t *testing.T, mem *Memory, upstream http.Handler) *server {
	t.Helper()
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	u, err := url.Parse(up.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &server{
		cfg:    config{timeout: time.Second, minChars: 1},
		target: u,
		proxy:  newSingleHostProxy(u),
		client: up.Client(),
		mem:    mem,
		// Empty allowlist, set explicitly rather than left nil: these tests
		// send no Origin header, which must pass. See origins_test.go for the
		// gate's own coverage.
		origins: newOriginGate(nil),
	}
}

func TestChatNonStreamingInjectsAndStores(t *testing.T) {
	fake := newFakeMuninn()
	defer fake.Close()

	var gotUpstreamBody map[string]any
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotUpstreamBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"Turning on the warm light."}}]}`)
	})

	mem := NewMemory(fake.URL, "home_assistant", "", 3, 300, time.Second)
	srv := newTestServer(t, mem, upstream)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"turn on the light"}]}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Turning on the warm light.") {
		t.Errorf("response not relayed verbatim: %s", rec.Body.String())
	}

	// The recalled memory must have reached the model.
	msgs, _ := gotUpstreamBody["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatal("upstream got no messages")
	}
	sys, _ := msgs[0].(map[string]any)
	if c, _ := sys["content"].(string); !strings.Contains(c, "user prefers warm light") {
		t.Errorf("memory not injected into upstream request, system=%q", c)
	}

	fake.waitForStore(t, 1)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	content, _ := fake.stored[0]["content"].(string)
	if !strings.Contains(content, "turn on the light") || !strings.Contains(content, "Turning on the warm light.") {
		t.Errorf("stored engram missing one side of the exchange: %q", content)
	}
	if fake.vaults[0] != "home_assistant" {
		t.Errorf("recall hit vault %q, want home_assistant", fake.vaults[0])
	}
}

func TestChatStreamingRelaysVerbatimAndCaptures(t *testing.T) {
	fake := newFakeMuninn()
	defer fake.Close()

	chunks := []string{
		`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
		`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
		`data: {"choices":[{"delta":{"content":" world"}}]}`,
		`data: [DONE]`,
	}
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, c := range chunks {
			_, _ = io.WriteString(w, c+"\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	})

	mem := NewMemory(fake.URL, "home_assistant", "", 3, 300, time.Second)
	srv := newTestServer(t, mem, upstream)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"m","stream":true,"messages":[{"role":"user","content":"say hello"}]}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, c := range chunks {
		if !strings.Contains(body, c) {
			t.Errorf("stream chunk not relayed verbatim: %q", c)
		}
	}

	fake.waitForStore(t, 1)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	content, _ := fake.stored[0]["content"].(string)
	if !strings.Contains(content, "Hello world") {
		t.Errorf("streamed deltas not reassembled, stored: %q", content)
	}
}

func TestOllamaFlavour(t *testing.T) {
	fake := newFakeMuninn()
	defer fake.Close()

	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"message":{"role":"assistant","content":"Kitchen light on."},"done":true}`)
	})
	mem := NewMemory(fake.URL, "home_assistant", "", 3, 300, time.Second)
	srv := newTestServer(t, mem, upstream)

	req := httptest.NewRequest(http.MethodPost, "/api/chat",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"kitchen light"}]}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	fake.waitForStore(t, 1)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if c, _ := fake.stored[0]["content"].(string); !strings.Contains(c, "Kitchen light on.") {
		t.Errorf("ollama response not captured: %q", c)
	}
}

// The whole point of failing open: MuninnDB being down must not degrade
// inference in any observable way.
func TestMemoryFailureFailsOpen(t *testing.T) {
	fake := newFakeMuninn()
	fake.fail = true
	defer fake.Close()

	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"still works"}}]}`)
	})
	mem := NewMemory(fake.URL, "home_assistant", "", 3, 300, 200*time.Millisecond)
	srv := newTestServer(t, mem, upstream)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hello"}]}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "still works") {
		t.Fatalf("inference broke when memory failed: %d %s", rec.Code, rec.Body.String())
	}
}

// A slow MuninnDB must not add unbounded latency to a voice turn.
func TestSlowMemoryIsBounded(t *testing.T) {
	fake := newFakeMuninn()
	fake.delay = 2 * time.Second
	defer fake.Close()

	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"fast"}}]}`)
	})
	mem := NewMemory(fake.URL, "home_assistant", "", 3, 300, 150*time.Millisecond)
	srv := newTestServer(t, mem, upstream)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hello"}]}`))
	rec := httptest.NewRecorder()

	start := time.Now()
	srv.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Errorf("slow memory added %v of latency; recall timeout should have capped it", elapsed)
	}
	if !strings.Contains(rec.Body.String(), "fast") {
		t.Error("response lost")
	}
}

// Non-chat traffic (web UI, /live, /api/tags, /mcp) must pass through untouched.
func TestNonChatPathsPassThrough(t *testing.T) {
	fake := newFakeMuninn()
	defer fake.Close()

	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "upstream:"+r.URL.Path)
	})
	mem := NewMemory(fake.URL, "home_assistant", "", 3, 300, time.Second)
	srv := newTestServer(t, mem, upstream)

	for _, path := range []string{"/live", "/api/tags", "/api/v1/models", "/", "/mcp"} {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Body.String(); got != "upstream:"+path {
			t.Errorf("%s: got %q, want passthrough", path, got)
		}
	}
	if fake.storedCount() != 0 {
		t.Error("non-chat traffic should never be stored")
	}
}

func TestRecallRespectsTokenBudget(t *testing.T) {
	mem := NewMemory("http://127.0.0.1:1", "v", "", 3, 5, 50*time.Millisecond) // 5 tokens => 20 chars
	if mem.recallBudget != 20 {
		t.Fatalf("budget = %d chars, want 20", mem.recallBudget)
	}
}

func TestMalformedRequestBodyStillProxies(t *testing.T) {
	fake := newFakeMuninn()
	defer fake.Close()
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_, _ = w.Write(b) // echo, to prove the body survived intact
	})
	mem := NewMemory(fake.URL, "home_assistant", "", 3, 300, time.Second)
	srv := newTestServer(t, mem, upstream)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`this is not json`)))

	if rec.Body.String() != "this is not json" {
		t.Errorf("non-JSON body was not proxied verbatim: %q", rec.Body.String())
	}
}

// A non-streamed reply delivered with chunked encoding and no Content-Length
// looks exactly like a stream if you only inspect response headers. Capture
// must key off the request's stream flag instead.
func TestNonStreamedChunkedReplyIsStillCaptured(t *testing.T) {
	fake := newFakeMuninn()
	defer fake.Close()

	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// No Content-Length: httptest will use chunked encoding.
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"chunked but whole"}}]}`)
	})
	mem := NewMemory(fake.URL, "home_assistant", "", 3, 300, time.Second)
	srv := newTestServer(t, mem, upstream)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"m","stream":false,"messages":[{"role":"user","content":"hello"}]}`)))

	fake.waitForStore(t, 1)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if c, _ := fake.stored[0]["content"].(string); !strings.Contains(c, "chunked but whole") {
		t.Errorf("non-streamed chunked reply was not captured: %q", c)
	}
}

func TestWantsStream(t *testing.T) {
	if !wantsStream(map[string]any{"stream": true}) {
		t.Error("stream:true not detected")
	}
	for _, p := range []map[string]any{{"stream": false}, {}, {"stream": "yes"}} {
		if wantsStream(p) {
			t.Errorf("false positive for %v", p)
		}
	}
}

// Guards the exact bug that shipped: MuninnDB returns its matches under
// "activations", and a struct that only knew "engrams"/"results" decoded zero
// results and returned "" with no error — memory silently never worked.
func TestRecallParsesRealMuninnDBSchema(t *testing.T) {
	real := `{"activations":[{"id":"01X","concept":"freezer","content":"the garage plug powers the chest freezer","score":0.61}],"brief":"","latency_ms":8,"query_id":"q","total_found":1}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, real)
	}))
	defer srv.Close()

	got := NewMemory(srv.URL, "home_assistant", "", 3, 300, time.Second).
		Recall(context.Background(), "what does the garage plug power?")
	if !strings.Contains(got, "chest freezer") {
		t.Fatalf("real MuninnDB schema not parsed; Recall returned %q", got)
	}
}

// The legacy keys stay as a fallback, so a rename upstream degrades rather
// than silently breaking again.
func TestRecallStillAcceptsLegacyKeys(t *testing.T) {
	for _, key := range []string{"engrams", "results"} {
		body := `{"` + key + `":[{"concept":"c","content":"legacy shape still parsed"}]}`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, body)
		}))
		got := NewMemory(srv.URL, "v", "", 3, 300, time.Second).
			Recall(context.Background(), "q")
		srv.Close()
		if !strings.Contains(got, "legacy shape still parsed") {
			t.Errorf("%s fallback broken: %q", key, got)
		}
	}
}
