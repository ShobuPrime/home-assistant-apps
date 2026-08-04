package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The reader must accept BOTH forms of `data`. Piper sends the length-prefixed
// form; a reader that only handles inline data dies on the first audio-start,
// which is exactly how the first version of this failed.
func TestReadEventAcceptsInlineAndLengthPrefixedData(t *testing.T) {
	inline := []byte(`{"type":"audio-start","data":{"rate":16000,"width":2,"channels":1}}` + "\n")

	dataBlock := `{"rate":22050,"width":2,"channels":1}`
	prefixed := fmt.Appendf(nil, `{"type":"audio-start","data_length":%d}`+"\n"+`%s`,
		len(dataBlock), dataBlock)

	for name, raw := range map[string][]byte{"inline": inline, "length-prefixed": prefixed} {
		t.Run(name, func(t *testing.T) {
			ev, _, err := readEvent(bufio.NewReader(bytes.NewReader(raw)))
			if err != nil {
				t.Fatalf("readEvent: %v", err)
			}
			var d struct {
				Rate     int `json:"rate"`
				Width    int `json:"width"`
				Channels int `json:"channels"`
			}
			if err := json.Unmarshal(ev.Data, &d); err != nil {
				t.Fatalf("data did not decode: %v", err)
			}
			if d.Rate == 0 || d.Width != 2 || d.Channels != 1 {
				t.Errorf("decoded %+v; rate/width/channels not recovered", d)
			}
		})
	}
}

func TestReadEventReadsPayloadAfterDataBlock(t *testing.T) {
	dataBlock := `{"rate":16000,"width":2,"channels":1}`
	audio := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	raw := fmt.Appendf(nil, `{"type":"audio-chunk","data_length":%d,"payload_length":%d}`+"\n"+`%s`,
		len(dataBlock), len(audio), dataBlock)
	raw = append(raw, audio...)

	ev, payload, err := readEvent(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatalf("readEvent: %v", err)
	}
	if ev.Type != "audio-chunk" {
		t.Errorf("type = %q", ev.Type)
	}
	if !bytes.Equal(payload, audio) {
		t.Errorf("payload = %v, want %v — data block and payload must not be confused", payload, audio)
	}
}

func TestReadEventRejectsOversizedPayload(t *testing.T) {
	raw := fmt.Appendf(nil, `{"type":"audio-chunk","payload_length":%d}`+"\n", maxUtteranceBytes+1)
	if _, _, err := readEvent(bufio.NewReader(bytes.NewReader(raw))); err == nil {
		t.Fatal("expected an oversized payload_length to be refused")
	}
}

func TestWavHeaderIsValid(t *testing.T) {
	pcm := make([]byte, 3200) // 0.1s of 16kHz mono 16-bit
	wav := wavFromPCM(pcm, 16000, 2, 1)

	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		t.Fatalf("not a RIFF/WAVE file: %q", wav[:12])
	}
	if got := binary.LittleEndian.Uint32(wav[4:]); got != uint32(36+len(pcm)) {
		t.Errorf("RIFF size = %d, want %d", got, 36+len(pcm))
	}
	if got := binary.LittleEndian.Uint16(wav[22:]); got != 1 {
		t.Errorf("channels = %d, want 1", got)
	}
	if got := binary.LittleEndian.Uint32(wav[24:]); got != 16000 {
		t.Errorf("sample rate = %d, want 16000", got)
	}
	if got := binary.LittleEndian.Uint16(wav[34:]); got != 16 {
		t.Errorf("bits per sample = %d, want 16", got)
	}
	if got := binary.LittleEndian.Uint32(wav[40:]); got != uint32(len(pcm)) {
		t.Errorf("data chunk size = %d, want %d", got, len(pcm))
	}
	if len(wav) != 44+len(pcm) {
		t.Errorf("total length = %d, want %d", len(wav), 44+len(pcm))
	}
}

func TestWavDefaultsAreSaneWhenHomeAssistantOmitsFormat(t *testing.T) {
	wav := wavFromPCM(make([]byte, 100), 0, 0, 0)
	if got := binary.LittleEndian.Uint32(wav[24:]); got != 16000 {
		t.Errorf("default sample rate = %d, want 16000", got)
	}
	if got := binary.LittleEndian.Uint16(wav[22:]); got != 1 {
		t.Errorf("default channels = %d, want 1", got)
	}
}

// info() is what registers the engine in Home Assistant, so its shape is a
// contract. Modelled on faster-whisper's actual reply.
func TestInfoAdvertisesAsrOnlyAndNeverTts(t *testing.T) {
	s := &wyomingServer{cfg: wyomingConfig{model: "Moonshine-Small-Streaming"}}
	raw, err := json.Marshal(s.info())
	if err != nil {
		t.Fatalf("info did not marshal: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(raw, &got)

	if _, ok := got["asr"]; !ok {
		t.Fatal("info must advertise asr or Home Assistant registers nothing")
	}
	// Lemonade cannot do TTS on this hardware; advertising it would put a
	// broken engine in the user's dropdown.
	if _, ok := got["tts"]; ok {
		t.Error("info must NOT advertise tts — no usable text-to-speech backend exists here")
	}

	progs := got["asr"].([]any)
	p := progs[0].(map[string]any)
	for _, k := range []string{"name", "attribution", "installed", "description", "version", "models"} {
		if _, ok := p[k]; !ok {
			t.Errorf("asr program missing required field %q", k)
		}
	}
	m := p["models"].([]any)[0].(map[string]any)
	for _, k := range []string{"name", "attribution", "installed", "description", "version", "languages"} {
		if _, ok := m[k]; !ok {
			t.Errorf("asr model missing required field %q", k)
		}
	}
	if m["name"] != "Moonshine-Small-Streaming" {
		t.Errorf("model name = %v, want the configured model", m["name"])
	}
}

// End-to-end over a real socket, against a stub standing in for lemond.
func TestFullTranscriptionExchange(t *testing.T) {
	var gotModel, gotContentType string
	var gotWav []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("upstream could not parse multipart: %v", err)
		}
		gotModel = r.FormValue("model")
		f, _, err := r.FormFile("file")
		if err == nil {
			gotWav, _ = io.ReadAll(f)
		}
		_, _ = io.WriteString(w, `{"text":"turn on the living room plug"}`)
	}))
	defer upstream.Close()

	srv := &wyomingServer{
		cfg: wyomingConfig{upstream: upstream.URL, model: "Moonshine-Small-Streaming", timeout: 30 * time.Second},
		hc:  &http.Client{Timeout: 30 * time.Second},
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go srv.serve(ln)

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	r := bufio.NewReader(conn)

	// describe -> info
	if err := writeEvent(conn, "describe", map[string]any{}, nil); err != nil {
		t.Fatal(err)
	}
	ev, _, err := readEvent(r)
	if err != nil || ev.Type != "info" {
		t.Fatalf("expected info, got %v (err %v)", ev, err)
	}

	// A full utterance.
	pcm := make([]byte, 6400)
	if err := writeEvent(conn, "transcribe", map[string]any{}, nil); err != nil {
		t.Fatal(err)
	}
	_ = writeEvent(conn, "audio-start", map[string]any{"rate": 16000, "width": 2, "channels": 1}, nil)
	_ = writeEvent(conn, "audio-chunk", map[string]any{"rate": 16000, "width": 2, "channels": 1}, pcm[:3200])
	_ = writeEvent(conn, "audio-chunk", map[string]any{"rate": 16000, "width": 2, "channels": 1}, pcm[3200:])
	_ = writeEvent(conn, "audio-stop", map[string]any{}, nil)

	ev, _, err = readEvent(r)
	if err != nil {
		t.Fatalf("reading transcript: %v", err)
	}
	if ev.Type != "transcript" {
		t.Fatalf("expected transcript, got %q", ev.Type)
	}
	var d struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(ev.Data, &d)
	if d.Text != "turn on the living room plug" {
		t.Errorf("transcript = %q", d.Text)
	}
	if gotModel != "Moonshine-Small-Streaming" {
		t.Errorf("upstream received model %q", gotModel)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Errorf("upstream Content-Type = %q", gotContentType)
	}
	if len(gotWav) != 44+len(pcm) {
		t.Errorf("upstream got %d bytes, want a %d-byte WAV", len(gotWav), 44+len(pcm))
	}
	if string(gotWav[0:4]) != "RIFF" {
		t.Error("upstream did not receive a RIFF/WAVE file")
	}
}

// A failing upstream must still produce a transcript event. Returning nothing
// leaves Home Assistant's pipeline waiting forever.
func TestUpstreamFailureStillAnswers(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not loaded", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	srv := &wyomingServer{
		cfg: wyomingConfig{upstream: upstream.URL, model: "m", timeout: 10 * time.Second},
		hc:  &http.Client{Timeout: 10 * time.Second},
	}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go srv.serve(ln)

	conn, _ := net.Dial("tcp", ln.Addr().String())
	defer conn.Close()
	r := bufio.NewReader(conn)

	_ = writeEvent(conn, "transcribe", map[string]any{}, nil)
	_ = writeEvent(conn, "audio-start", map[string]any{"rate": 16000, "width": 2, "channels": 1}, nil)
	_ = writeEvent(conn, "audio-chunk", map[string]any{"rate": 16000, "width": 2, "channels": 1}, make([]byte, 320))
	_ = writeEvent(conn, "audio-stop", map[string]any{}, nil)

	ev, _, err := readEvent(r)
	if err != nil {
		t.Fatalf("no reply after upstream failure: %v", err)
	}
	if ev.Type != "transcript" {
		t.Fatalf("expected an (empty) transcript, got %q", ev.Type)
	}
	if srv.failed.Load() != 1 {
		t.Errorf("failure counter = %d, want 1", srv.failed.Load())
	}
}

// Silence must be answered too, for the same reason.
func TestEmptyUtteranceAnswersImmediately(t *testing.T) {
	srv := &wyomingServer{cfg: wyomingConfig{model: "m"}, hc: &http.Client{}}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go srv.serve(ln)

	conn, _ := net.Dial("tcp", ln.Addr().String())
	defer conn.Close()
	r := bufio.NewReader(conn)

	_ = writeEvent(conn, "transcribe", map[string]any{}, nil)
	_ = writeEvent(conn, "audio-start", map[string]any{"rate": 16000, "width": 2, "channels": 1}, nil)
	_ = writeEvent(conn, "audio-stop", map[string]any{}, nil)

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	ev, _, err := readEvent(r)
	if err != nil {
		t.Fatalf("silence produced no reply: %v", err)
	}
	if ev.Type != "transcript" {
		t.Errorf("expected transcript, got %q", ev.Type)
	}
}
