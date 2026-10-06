package updates

import (
	"context"
	"log/slog"
	"sync"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
)

// Worker is the single in-order consumer of updates from every source
// (polling and webhook). Sources call Enqueue; Run feeds Handle one update
// at a time.
//
// The queue is unbounded so Enqueue never blocks: the webhook must ack at
// once, and a mode switch runs inside Handle while the poller it stops may
// still be handing over its last batch. The poller keeps Bale from
// confirming updates the worker has not processed by calling WaitIdle before
// its next getUpdates (the offset in that call is the confirmation).
type Worker struct {
	Handle func(ctx context.Context, u bale.Update) error
	Log    *slog.Logger

	mu      sync.Mutex
	queue   []bale.Update
	pending int           // queued + in progress
	idle    chan struct{} // closed when pending drops to 0
	wake    chan struct{}
}

// NewWorker returns a Worker.
func NewWorker(handle func(ctx context.Context, u bale.Update) error, log *slog.Logger) *Worker {
	if log == nil {
		log = slog.Default()
	}
	idle := make(chan struct{})
	close(idle)
	return &Worker{Handle: handle, Log: log, idle: idle, wake: make(chan struct{}, 1)}
}

// Enqueue adds u to the queue. It never blocks.
func (w *Worker) Enqueue(_ context.Context, u bale.Update) error {
	w.mu.Lock()
	w.queue = append(w.queue, u)
	if w.pending == 0 {
		w.idle = make(chan struct{})
	}
	w.pending++
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
	return nil
}

// WaitIdle blocks until every enqueued update has been handled, or ctx ends.
func (w *Worker) WaitIdle(ctx context.Context) error {
	w.mu.Lock()
	idle := w.idle
	w.mu.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run processes queued updates until ctx ends. Handler errors are logged and
// do not stop the worker.
func (w *Worker) Run(ctx context.Context) {
	for {
		w.mu.Lock()
		if len(w.queue) == 0 {
			w.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-w.wake:
			}
			continue
		}
		u := w.queue[0]
		w.queue[0] = bale.Update{}
		w.queue = w.queue[1:]
		w.mu.Unlock()

		if ctx.Err() != nil {
			return
		}
		if err := w.Handle(ctx, u); err != nil {
			w.Log.Error("update failed", "update_id", u.UpdateID, "err", err)
		}

		w.mu.Lock()
		w.pending--
		if w.pending == 0 {
			close(w.idle)
		}
		w.mu.Unlock()
	}
}
