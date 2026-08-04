package main

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Both the OpenAI and Ollama chat schemas carry a top-level "messages" array of
// {role, content}, which is why one code path serves both. They differ only in
// where the assistant's reply lands in the response.

// contentText flattens a message's content field to plain text.
//
// Content is a string in the common case, but multimodal requests send an array
// of typed parts. Non-text parts (images, audio) are skipped: they are not
// useful as a memory key and would bloat the store.
func contentText(v any) string {
	switch c := v.(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, p := range c {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := pm["type"].(string); t != "" && t != "text" {
				continue
			}
			if s, ok := pm["text"].(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

func messagesOf(payload map[string]any) ([]any, bool) {
	msgs, ok := payload["messages"].([]any)
	return msgs, ok
}

// lastUserText returns the most recent user turn, which is both the recall
// query and the half of the exchange we store.
//
// Home Assistant resends the whole rolling window every turn, so keying off the
// last user message (rather than the transcript) keeps recall focused on what
// was actually just asked.
func lastUserText(payload map[string]any) string {
	msgs, ok := messagesOf(payload)
	if !ok {
		return ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		m, ok := msgs[i].(map[string]any)
		if !ok {
			continue
		}
		if role, _ := m["role"].(string); role != "user" {
			continue
		}
		if txt := strings.TrimSpace(contentText(m["content"])); txt != "" {
			return txt
		}
	}
	return ""
}

// injectSystem adds the recalled-memory block to the conversation's system
// prompt, appending to an existing one rather than replacing it — Home
// Assistant's system prompt carries the exposed-entity list and tool
// instructions, and clobbering it would break Assist entirely.
//
// Reports whether the payload was modified.
func injectSystem(payload map[string]any, block string) bool {
	msgs, ok := messagesOf(payload)
	if !ok {
		return false
	}
	for i, raw := range msgs {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if role, _ := m["role"].(string); role != "system" {
			continue
		}
		existing := contentText(m["content"])
		m["content"] = strings.TrimRight(existing, "\n") + "\n\n" + block
		msgs[i] = m
		payload["messages"] = msgs
		return true
	}

	// No system message: prepend one.
	sys := map[string]any{"role": "system", "content": block}
	payload["messages"] = append([]any{sys}, msgs...)
	return true
}

var sseData = []byte("data:")

// extractDelta pulls the incremental assistant text out of one streamed line.
// Unparseable lines yield "" — capture is best-effort and must never disturb
// the bytes being relayed to the client.
func extractDelta(line []byte, flavour string) string {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return ""
	}

	if flavour == "openai" {
		if !bytes.HasPrefix(trimmed, sseData) {
			return ""
		}
		payload := bytes.TrimSpace(trimmed[len(sseData):])
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			return ""
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(payload, &chunk); err != nil || len(chunk.Choices) == 0 {
			return ""
		}
		return chunk.Choices[0].Delta.Content
	}

	// Ollama streams newline-delimited JSON objects.
	var chunk struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(trimmed, &chunk); err != nil {
		return ""
	}
	return chunk.Message.Content
}

// extractFull pulls the assistant text out of a complete, non-streamed response.
func extractFull(raw []byte, flavour string) string {
	if flavour == "openai" {
		var resp struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil || len(resp.Choices) == 0 {
			return ""
		}
		return resp.Choices[0].Message.Content
	}

	var resp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return ""
	}
	return resp.Message.Content
}
