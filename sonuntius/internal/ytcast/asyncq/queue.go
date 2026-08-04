// Maps to: src/lib/utils/AsyncTaskQueue.ts
//
// Package asyncq ports the upstream `AsyncTaskQueue` — a serial executor of
// async tasks with the same lifecycle controls (autostart toggle, push,
// unshift, clear, stop) the lounge layer relies on.
//
// Concurrency model:
//   - Tasks run one at a time, in FIFO order (push) or LIFO at head (unshift).
//   - The queue starts a background goroutine on Start(); push() will start
//     it automatically when autostart is on (the upstream default).
//   - The goroutine exits when the queue is drained or Stop() / Clear() is
//     called.
//   - All public methods are safe to call from any goroutine; internal state
//     is protected by a single mutex.
package asyncq

import (
	"context"
	"sync"
)

// Task ports the upstream `Task` interface. Run is the unit of work; Cancel
// is invoked on tasks that get dropped via Clear(); OnError fires when Run
// returns an error (the queue then stops, matching upstream behaviour).
type Task struct {
	// Run executes the task. The caller-provided context is propagated so
	// long-running tasks can react to cancellation.
	Run func(ctx context.Context) error
	// Cancel is invoked when the task is removed from the queue without
	// running (e.g. because Clear was called). Optional — leave nil if no
	// cleanup is needed.
	Cancel func()
	// OnError is invoked when Run returns a non-nil error. Optional. The
	// upstream `onError` signature is `(task, error) => any`; we don't pass
	// the task because callers can capture it in the closure if they want.
	OnError func(err error)
}

// Queue ports the upstream `AsyncTaskQueue` class.
type Queue struct {
	mu        sync.Mutex
	tasks     []Task
	stopped   bool
	autostart bool

	// runDone is closed when the running goroutine exits. nil when no
	// goroutine is in flight.
	runDone chan struct{}

	// rootCtx is the context the worker passes to each task; reset on Start.
	rootCtx    context.Context
	rootCancel context.CancelFunc
}

// New constructs a Queue with autostart on (matching the upstream default).
func New() *Queue {
	return &Queue{
		stopped:   true,
		autostart: true,
	}
}

// SetAutoStart ports `setAutoStart`. Toggling autostart on while tasks are
// pending kicks off the worker (parity with upstream).
func (q *Queue) SetAutoStart(value bool) {
	q.mu.Lock()
	q.autostart = value
	shouldStart := value && len(q.tasks) > 0 && q.stopped
	q.mu.Unlock()
	if shouldStart {
		q.Start()
	}
}

// Push ports `push` — append a task to the tail and start the worker if
// autostart is on.
func (q *Queue) Push(task Task) {
	q.mu.Lock()
	q.tasks = append(q.tasks, task)
	shouldStart := q.stopped && q.autostart
	q.mu.Unlock()
	if shouldStart {
		q.Start()
	}
}

// Unshift ports `unshift` — prepend a task to the head. Does NOT start the
// worker (upstream behaviour: only push triggers autostart).
func (q *Queue) Unshift(task Task) {
	q.mu.Lock()
	q.tasks = append([]Task{task}, q.tasks...)
	q.mu.Unlock()
}

// Start ports `start`. It is idempotent: calling Start while the worker is
// already running is a no-op. Returns a channel that is closed when the
// worker exits (drains or is stopped).
func (q *Queue) Start() <-chan struct{} {
	q.mu.Lock()
	if !q.stopped {
		ch := q.runDone
		q.mu.Unlock()
		return ch
	}
	q.stopped = false
	done := make(chan struct{})
	q.runDone = done
	ctx, cancel := context.WithCancel(context.Background())
	q.rootCtx = ctx
	q.rootCancel = cancel
	q.mu.Unlock()

	go q.run(ctx, done)
	return done
}

// run is the worker loop. It pulls one task at a time and bails out as soon
// as Stop / Clear flips `stopped`.
func (q *Queue) run(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	var failed *failedTask
	for {
		q.mu.Lock()
		if q.stopped || len(q.tasks) == 0 {
			q.stopped = true
			q.runDone = nil
			if q.rootCancel != nil {
				q.rootCancel()
				q.rootCancel = nil
				q.rootCtx = nil
			}
			q.mu.Unlock()
			break
		}
		task := q.tasks[0]
		q.tasks = q.tasks[1:]
		q.mu.Unlock()

		if task.Run == nil {
			continue
		}
		if err := task.Run(ctx); err != nil {
			failed = &failedTask{task: task, err: err}
			q.mu.Lock()
			q.stopped = true
			q.runDone = nil
			if q.rootCancel != nil {
				q.rootCancel()
				q.rootCancel = nil
				q.rootCtx = nil
			}
			q.mu.Unlock()
			break
		}
	}
	if failed != nil && failed.task.OnError != nil {
		failed.task.OnError(failed.err)
	}
}

// failedTask captures the (task, error) pair upstream attaches to onError.
type failedTask struct {
	task Task
	err  error
}

// Clear ports `clear` — stop the worker, cancel every pending task, drop the
// queue. Tasks already executing are not interrupted (they observe the
// cancelled context); upstream has the same semantics because JS tasks are
// cooperative.
func (q *Queue) Clear() {
	q.mu.Lock()
	q.stopped = true
	if q.rootCancel != nil {
		q.rootCancel()
		q.rootCancel = nil
		q.rootCtx = nil
	}
	pending := q.tasks
	q.tasks = nil
	q.mu.Unlock()
	for _, t := range pending {
		if t.Cancel != nil {
			t.Cancel()
		}
	}
}

// Stop ports `stop` — flip `stopped` so the worker exits after the current
// task. Pending tasks remain in the queue.
func (q *Queue) Stop() {
	q.mu.Lock()
	q.stopped = true
	if q.rootCancel != nil {
		q.rootCancel()
		q.rootCancel = nil
		q.rootCtx = nil
	}
	q.mu.Unlock()
}

// ForEach ports `forEach` — invoke fn for each pending (not currently
// executing) task in order. The slice passed to fn is a snapshot copy; it is
// safe for fn to mutate it.
func (q *Queue) ForEach(fn func(idx int, task Task)) {
	q.mu.Lock()
	snapshot := make([]Task, len(q.tasks))
	copy(snapshot, q.tasks)
	q.mu.Unlock()
	for i, t := range snapshot {
		fn(i, t)
	}
}

// Len returns the number of pending tasks (excluding the one currently
// executing, if any).
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.tasks)
}

// Stopped reports whether the worker is idle.
func (q *Queue) Stopped() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.stopped
}
