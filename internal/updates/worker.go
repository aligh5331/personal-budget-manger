package updates

import (
	"context"
	"log/slog"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
)

// Worker is the single in-order consumer of updates from every source
// (polling and webhook). Sources call Enqueue; Run feeds Handle one update
// at a time.
type Worker struct {
	Handle func(ctx context.Context, u bale.Update) error
	Log    *slog.Logger
	ch     chan bale.Update
}

// NewWorker returns a Worker with a queue of the given size.
func NewWorker(handle func(ctx context.Context, u bale.Update) error, log *slog.Logger, queue int) *Worker {
	return &Worker{Handle: handle, Log: log, ch: make(chan bale.Update, queue)}
}

// Enqueue adds u to the queue, blocking while it is full.
func (w *Worker) Enqueue(ctx context.Context, u bale.Update) error {
	select {
	case w.ch <- u:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run processes queued updates until ctx ends. Handler errors are logged and
// do not stop the worker.
func (w *Worker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case u := <-w.ch:
			if err := w.Handle(ctx, u); err != nil {
				w.Log.Error("update failed", "update_id", u.UpdateID, "err", err)
			}
		}
	}
}
