// Maps to: N/A — Go-only tests for the AsyncTaskQueue port.
package asyncq

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueueRunsTasksInOrder(t *testing.T) {
	q := New()
	q.SetAutoStart(false)

	var mu sync.Mutex
	var got []int
	for i := range 5 {
		q.Push(Task{
			Run: func(_ context.Context) error {
				mu.Lock()
				got = append(got, i)
				mu.Unlock()
				return nil
			},
		})
	}

	done := q.Start()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("queue did not drain in time")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 5 {
		t.Fatalf("expected 5 tasks, got %d (%v)", len(got), got)
	}
	for i, v := range got {
		if v != i {
			t.Fatalf("expected order [0..4], got %v", got)
		}
	}
}

func TestQueueOnErrorAndStop(t *testing.T) {
	q := New()
	q.SetAutoStart(false)

	bad := errors.New("boom")
	var caught error
	var ranAfter atomic.Bool

	q.Push(Task{Run: func(_ context.Context) error { return bad },
		OnError: func(err error) { caught = err }})
	q.Push(Task{Run: func(_ context.Context) error { ranAfter.Store(true); return nil }})

	done := q.Start()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("queue did not exit after error")
	}

	if !errors.Is(caught, bad) {
		t.Fatalf("expected onError to receive %v, got %v", bad, caught)
	}
	if ranAfter.Load() {
		t.Fatal("queue should stop after a failing task")
	}
}

func TestQueueClearCancelsPending(t *testing.T) {
	q := New()
	q.SetAutoStart(false)

	var cancelled atomic.Int32
	for range 3 {
		q.Push(Task{
			Run:    func(_ context.Context) error { time.Sleep(time.Hour); return nil },
			Cancel: func() { cancelled.Add(1) },
		})
	}
	q.Clear()
	if got := cancelled.Load(); got != 3 {
		t.Fatalf("expected 3 cancellations, got %d", got)
	}
	if q.Len() != 0 {
		t.Fatalf("expected empty queue after clear, got %d", q.Len())
	}
}

func TestQueueAutostartTriggersOnPush(t *testing.T) {
	q := New() // autostart = true
	ran := make(chan struct{})
	q.Push(Task{Run: func(_ context.Context) error { close(ran); return nil }})
	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("autostart did not run pushed task")
	}
}
