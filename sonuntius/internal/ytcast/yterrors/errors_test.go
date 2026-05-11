// Maps to: N/A — Go-only tests for the Errors port.
package yterrors

import (
	"errors"
	"testing"
)

func TestSentinelMatching(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		sentinel error
	}{
		{"connection", NewConnectionError("oops", "https://example", nil), ErrConnection},
		{"abort", NewAbortError("oops", "https://example"), ErrAbort},
		{"badResponse", NewBadResponseError("oops", "https://example", HTTPResponse{Status: 500}), ErrBadResponse},
		{"data", NewDataError("oops", nil, nil), ErrData},
		{"incomplete", NewIncompleteAPIDataError("oops", []string{"x"}), ErrIncompleteAPIData},
		{"session", NewSessionError("oops", nil), ErrSession},
		{"app", NewAppError("oops", nil), ErrApp},
		{"dial", NewDialServerError("oops", nil), ErrDialServer},
		{"sender", NewSenderConnectionError("oops", nil, ""), ErrSenderConnection},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !errors.Is(c.err, c.sentinel) {
				t.Fatalf("expected errors.Is(_, %v) to match", c.sentinel)
			}
			if !errors.Is(c.err, ErrYouTubeCastReceiver) {
				t.Fatalf("expected errors.Is(_, ErrYouTubeCastReceiver) to match")
			}
		})
	}
}

func TestGetCausesWalksOnlyBaseErrors(t *testing.T) {
	root := errors.New("root")
	mid := NewSessionError("mid", root)
	top := NewAppError("top", mid)

	causes := top.GetCauses()
	if len(causes) != 2 {
		t.Fatalf("expected 2 causes, got %d (%v)", len(causes), causes)
	}
	if !errors.Is(causes[0], ErrSession) {
		t.Fatalf("first cause should be SessionError, got %v", causes[0])
	}
	if causes[1] != root {
		t.Fatalf("second cause should be root, got %v", causes[1])
	}
}

func TestSenderConnectionDefaultAction(t *testing.T) {
	e := NewSenderConnectionError("x", nil, "")
	if e.Info["action"] != "unknown" {
		t.Fatalf("expected default action 'unknown', got %v", e.Info["action"])
	}
}

func TestUnwrapWorks(t *testing.T) {
	root := errors.New("root")
	wrapped := NewSessionError("wrap", root)
	if !errors.Is(wrapped, root) {
		t.Fatal("errors.Is should walk through Unwrap to the root cause")
	}
}
