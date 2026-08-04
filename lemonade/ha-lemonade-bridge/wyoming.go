package main

// Wyoming protocol server, exposing Lemonade's speech-to-text to Home
// Assistant as a first-class Voice Assistant engine.
//
// Home Assistant discovers speech services over the Wyoming protocol, not over
// HTTP. Lemonade speaks OpenAI-compatible HTTP and nothing else, so without a
// translator its Moonshine models cannot appear in the Speech-to-text dropdown
// no matter how well they work. This is that translator:
//
//	HA  ──Wyoming/TCP──►  bridge  ──POST /api/v1/audio/transcriptions──►  lemond
//
// Discovery is automatic: config.yaml declares `discovery: - wyoming`, the
// Supervisor tells Home Assistant an add-on offers a Wyoming service, HA
// connects and sends `describe`, and the `info` reply below is what registers
// the engine. No IP addresses, no manual setup — the same path Piper takes.
//
// # Protocol
//
// Newline-delimited JSON. Each event is one header line, optionally followed by
// a JSON data block and then a binary payload:
//
//	{"type":"audio-chunk","data_length":D,"payload_length":P}\n<D bytes JSON><P bytes audio>
//
// `data` may be inline in the header OR length-prefixed. Both occur in
// practice — Piper uses the length-prefixed form — so the reader handles both
// and the writer uses the inline form, which every implementation accepts.

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// Bounds a single utterance. Home Assistant sends audio only while the user is
// speaking, so this is a safety valve against a client that never sends
// audio-stop rather than a real limit — 10 minutes of 16 kHz mono.
const maxUtteranceBytes = 16000 * 2 * 600

type wyomingConfig struct {
	listen   string // e.g. ":10600"
	upstream string // lemond base URL, e.g. http://127.0.0.1:13306
	model    string // Moonshine model name
	apiKey   string
	timeout  time.Duration
}

type wyomingServer struct {
	cfg wyomingConfig
	hc  *http.Client

	transcribed atomic.Int64
	failed      atomic.Int64
}

// --- wire format ------------------------------------------------------------

type wyomingEvent struct {
	Type          string          `json:"type"`
	Data          json.RawMessage `json:"data,omitempty"`
	DataLength    int             `json:"data_length,omitempty"`
	PayloadLength int             `json:"payload_length,omitempty"`
}

func writeEvent(w io.Writer, typ string, data any, payload []byte) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	ev := map[string]any{"type": typ, "data": json.RawMessage(raw)}
	if len(payload) > 0 {
		ev["payload_length"] = len(payload)
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(line, '\n')); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// readEvent returns the event header (with data resolved from either form) and
// its payload.
func readEvent(r *bufio.Reader) (*wyomingEvent, []byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, nil, err
	}
	line = trimSpaceBytes(line)
	if len(line) == 0 {
		return nil, nil, errEmptyLine
	}
	var ev wyomingEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil, nil, fmt.Errorf("bad event header: %w", err)
	}
	// Length-prefixed data block, if the sender used that form.
	if ev.DataLength > 0 {
		buf := make([]byte, ev.DataLength)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, nil, err
		}
		ev.Data = buf
	}
	var payload []byte
	if ev.PayloadLength > 0 {
		if ev.PayloadLength > maxUtteranceBytes {
			return nil, nil, fmt.Errorf("payload_length %d exceeds cap", ev.PayloadLength)
		}
		payload = make([]byte, ev.PayloadLength)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, nil, err
		}
	}
	return &ev, payload, nil
}

var errEmptyLine = fmt.Errorf("empty event line")

func trimSpaceBytes(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return b
}

// --- describe / info --------------------------------------------------------

type wyomingAttribution struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type wyomingModel struct {
	Name        string             `json:"name"`
	Attribution wyomingAttribution `json:"attribution"`
	Installed   bool               `json:"installed"`
	Description string             `json:"description"`
	Version     string             `json:"version"`
	Languages   []string           `json:"languages"`
}

type wyomingProgram struct {
	Name        string             `json:"name"`
	Attribution wyomingAttribution `json:"attribution"`
	Installed   bool               `json:"installed"`
	Description string             `json:"description"`
	Version     string             `json:"version"`
	Models      []wyomingModel     `json:"models"`
}

// info mirrors the shape faster-whisper advertises, captured from a running
// instance. Home Assistant registers the engine from this reply, so the field
// set matters more than the values.
//
// Only `asr` is advertised. Lemonade cannot do text-to-speech on this hardware
// — its Kokoro backend is x86-64 only, and running Kokoro's ONNX model directly
// measured ~2x slower than real time on a CM5 against Piper's ~0.23x. Claiming
// a `tts` capability we cannot serve would put a broken engine in the user's
// dropdown, so it is deliberately absent.
func (s *wyomingServer) info() map[string]any {
	return map[string]any{
		"asr": []wyomingProgram{{
			Name: "lemonade",
			Attribution: wyomingAttribution{
				Name: "Lemonade SDK",
				URL:  "https://lemonade-server.ai/",
			},
			Installed:   true,
			Description: "Speech-to-text via Lemonade (" + s.cfg.model + ")",
			Version:     bridgeVersion,
			Models: []wyomingModel{{
				Name:        s.cfg.model,
				Attribution: wyomingAttribution{Name: "Useful Sensors", URL: "https://github.com/usefulsensors/moonshine"},
				Installed:   true,
				Description: s.cfg.model,
				Version:     bridgeVersion,
				// Moonshine is English-only. Advertising more would make Home
				// Assistant offer languages that silently transcribe wrongly.
				Languages: []string{"en"},
			}},
		}},
	}
}

const bridgeVersion = "1.0.0"

// --- audio ------------------------------------------------------------------

// wavFromPCM wraps raw PCM in a RIFF/WAVE container, which is what Lemonade's
// transcription endpoint expects. Home Assistant streams headerless PCM.
func wavFromPCM(pcm []byte, rate, width, channels int) []byte {
	if rate <= 0 {
		rate = 16000
	}
	if width <= 0 {
		width = 2
	}
	if channels <= 0 {
		channels = 1
	}
	byteRate := rate * channels * width
	blockAlign := channels * width

	hdr := make([]byte, 44)
	copy(hdr[0:], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:], uint32(36+len(pcm)))
	copy(hdr[8:], "WAVE")
	copy(hdr[12:], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:], 16)
	binary.LittleEndian.PutUint16(hdr[20:], 1) // PCM
	binary.LittleEndian.PutUint16(hdr[22:], uint16(channels))
	binary.LittleEndian.PutUint32(hdr[24:], uint32(rate))
	binary.LittleEndian.PutUint32(hdr[28:], uint32(byteRate))
	binary.LittleEndian.PutUint16(hdr[32:], uint16(blockAlign))
	binary.LittleEndian.PutUint16(hdr[34:], uint16(width*8))
	copy(hdr[36:], "data")
	binary.LittleEndian.PutUint32(hdr[40:], uint32(len(pcm)))
	return append(hdr, pcm...)
}

// transcribe POSTs one utterance to Lemonade and returns the recognised text.
func (s *wyomingServer) transcribe(wav []byte) (string, error) {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		defer pw.Close()
		defer mw.Close()
		_ = mw.WriteField("model", s.cfg.model)
		part, err := mw.CreateFormFile("file", "utterance.wav")
		if err != nil {
			return
		}
		_, _ = part.Write(wav)
	}()

	req, err := http.NewRequest(http.MethodPost,
		strings.TrimRight(s.cfg.upstream, "/")+"/api/v1/audio/transcriptions", pr)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if s.cfg.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.apiKey)
	}

	resp, err := s.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("lemonade HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decoding transcription: %w", err)
	}
	return out.Text, nil
}

// --- connection handling ----------------------------------------------------

func (s *wyomingServer) handleConn(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReaderSize(conn, 16<<10)

	var (
		pcm      []byte
		rate     = 16000
		width    = 2
		channels = 1
		started  time.Time
	)

	for {
		ev, payload, err := readEvent(r)
		if err != nil {
			if err != io.EOF && err != errEmptyLine {
				logf("wyoming: read: %v", err)
			}
			return
		}

		switch ev.Type {
		case "describe":
			if err := writeEvent(conn, "info", s.info(), nil); err != nil {
				logf("wyoming: writing info: %v", err)
				return
			}

		case "transcribe":
			// Start of a new utterance. HA may send a language hint here; it is
			// ignored because Moonshine is English-only and the info reply says so.
			pcm = pcm[:0]
			started = time.Now()

		case "audio-start":
			var d struct {
				Rate     int `json:"rate"`
				Width    int `json:"width"`
				Channels int `json:"channels"`
			}
			if len(ev.Data) > 0 {
				_ = json.Unmarshal(ev.Data, &d)
			}
			if d.Rate > 0 {
				rate = d.Rate
			}
			if d.Width > 0 {
				width = d.Width
			}
			if d.Channels > 0 {
				channels = d.Channels
			}
			pcm = pcm[:0]
			if started.IsZero() {
				started = time.Now()
			}

		case "audio-chunk":
			if len(pcm)+len(payload) > maxUtteranceBytes {
				logf("wyoming: utterance exceeded %d bytes, dropping", maxUtteranceBytes)
				pcm = pcm[:0]
				continue
			}
			pcm = append(pcm, payload...)

		case "audio-stop":
			if len(pcm) == 0 {
				// Nothing captured: still answer, or Home Assistant waits for a
				// transcript that never comes and the pipeline stalls.
				_ = writeEvent(conn, "transcript", map[string]any{"text": ""}, nil)
				continue
			}
			wav := wavFromPCM(pcm, rate, width, channels)
			text, err := s.transcribe(wav)
			if err != nil {
				s.failed.Add(1)
				logf("wyoming: transcription failed (%v) — returning empty transcript", err)
				// An empty transcript is a clean "I didn't catch that" in the
				// voice pipeline. Dropping the connection instead would surface
				// as a generic pipeline error with no useful detail.
				_ = writeEvent(conn, "transcript", map[string]any{"text": ""}, nil)
				pcm = pcm[:0]
				continue
			}
			s.transcribed.Add(1)
			audioSec := float64(len(pcm)) / float64(rate*width*channels)
			logf("wyoming: transcribed %.2fs of audio in %.2fs: %q",
				audioSec, time.Since(started).Seconds(), truncate(text, 120))
			if err := writeEvent(conn, "transcript", map[string]any{"text": text}, nil); err != nil {
				logf("wyoming: writing transcript: %v", err)
				return
			}
			pcm = pcm[:0]
			started = time.Time{}
		}
	}
}

func (s *wyomingServer) serve(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			logf("wyoming: accept: %v", err)
			return
		}
		go s.handleConn(conn)
	}
}

// startWyoming brings up the STT bridge. Failing to listen is logged and
// otherwise ignored: speech-to-text is an addition, and losing it must never
// stop Lemonade serving chat completions.
func startWyoming(cfg wyomingConfig) *wyomingServer {
	s := &wyomingServer{
		cfg: cfg,
		hc:  &http.Client{Timeout: cfg.timeout},
	}
	ln, err := net.Listen("tcp", cfg.listen)
	if err != nil {
		logf("wyoming: cannot listen on %s (%v) — speech-to-text disabled, chat unaffected", cfg.listen, err)
		return nil
	}
	logf("wyoming: speech-to-text listening on %s (model %q, upstream %s)",
		cfg.listen, cfg.model, cfg.upstream)
	go s.serve(ln)
	return s
}
