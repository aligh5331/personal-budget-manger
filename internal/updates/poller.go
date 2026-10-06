// Package updates holds the update sources (long polling now, the webhook
// later) and the single in-order worker that feeds the bot core.
package updates

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
)

// Poll settings from the spec.
const (
	PollTimeout = 25 // seconds, sent as getUpdates timeout
	PollLimit   = 100
	minBackoff  = time.Second
	maxBackoff  = 60 * time.Second
)

// PollClient is the part of bale.Client the poller needs.
type PollClient interface {
	GetUpdates(ctx context.Context, p bale.GetUpdatesParams) ([]bale.Update, error)
	DeleteWebhook(ctx context.Context) error
}

// Poller long-polls Bale and hands every update to Handle in order.
type Poller struct {
	Bale PollClient
	// Handle receives each update (normally Worker.Enqueue).
	Handle func(ctx context.Context, u bale.Update) error
	// Offset is the first update_id to ask for (highest processed + 1).
	Offset int64
	// Wait, if set, is called after each non-empty batch and blocks until
	// that batch is processed (normally Worker.WaitIdle), so the next
	// getUpdates never confirms an update the worker has not handled.
	Wait func(ctx context.Context) error

	Clock clock.Clock
	Log   *slog.Logger
	// Sleep waits d or until ctx ends. Nil means a real timer.
	Sleep func(ctx context.Context, d time.Duration) error
	// Jitter returns a number in [0,1). Nil means math/rand.
	Jitter func() float64

	mu          sync.Mutex
	started     time.Time
	lastSuccess time.Time
}

// Run polls until ctx ends. It returns ctx's error.
func (p *Poller) Run(ctx context.Context) error {
	return p.RunUntil(ctx, nil)
}

// RunUntil polls until ctx ends or stop is closed. A stop lets the in-flight
// getUpdates finish and hands over its updates, then commits the offset (a
// getUpdates with timeout 0, repeated while it returns more updates, which
// are handed over too) so Bale does not deliver them again, and returns nil.
// A failed commit returns its error; nothing fetched is lost either way.
func (p *Poller) RunUntil(ctx context.Context, stop <-chan struct{}) error {
	p.mu.Lock()
	p.started = p.Clock.Now()
	p.mu.Unlock()

	// waitCtx also ends on stop: sleeps and idle waits are cut short, the
	// in-flight getUpdates is not.
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if stop != nil {
		go func() {
			select {
			case <-stop:
				cancel()
			case <-waitCtx.Done():
			}
		}()
	}
	stopped := func() bool {
		select {
		case <-stop:
			return true
		default:
			return false
		}
	}

	failures := 0
	triedDeleteWebhook := false
	for ctx.Err() == nil {
		if stopped() {
			return p.commit(ctx)
		}
		us, err := p.Bale.GetUpdates(ctx, bale.GetUpdatesParams{Offset: p.Offset, Limit: PollLimit, Timeout: PollTimeout})
		if err == nil {
			failures = 0
			triedDeleteWebhook = false
			p.mu.Lock()
			p.lastSuccess = p.Clock.Now()
			p.mu.Unlock()
			if err := p.handOver(ctx, us); err != nil {
				return err
			}
			if len(us) > 0 && p.Wait != nil {
				_ = p.Wait(waitCtx) // cut short by stop or ctx; the loop checks both
			}
			continue
		}
		if ctx.Err() != nil {
			break
		}

		if bale.IsWebhookActive(err) && !triedDeleteWebhook {
			triedDeleteWebhook = true
			p.Log.Warn("getUpdates refused because a webhook is set; deleting it", "err", err)
			if derr := p.Bale.DeleteWebhook(ctx); derr == nil {
				continue
			} else {
				p.Log.Warn("deleteWebhook failed", "err", derr)
			}
		}

		var ae *bale.APIError
		if errors.As(err, &ae) && ae.RetryAfter > 0 {
			p.Log.Warn("getUpdates rate limited", "retry_after", ae.RetryAfter)
			_ = p.sleep(waitCtx, ae.RetryAfter)
			continue
		}

		failures++
		d := p.backoff(failures)
		p.Log.Warn("getUpdates failed", "err", err, "failures", failures, "retry_in", d)
		_ = p.sleep(waitCtx, d)
	}
	return ctx.Err()
}

// handOver passes us to Handle in order and advances the offset past each.
func (p *Poller) handOver(ctx context.Context, us []bale.Update) error {
	for _, u := range us {
		if err := p.Handle(ctx, u); err != nil {
			return err
		}
		if u.UpdateID >= p.Offset {
			p.Offset = u.UpdateID + 1
		}
	}
	return nil
}

// commit confirms every handed-over update to Bale.
func (p *Poller) commit(ctx context.Context) error {
	for {
		us, err := p.Bale.GetUpdates(ctx, bale.GetUpdatesParams{Offset: p.Offset, Limit: PollLimit})
		if err != nil {
			return fmt.Errorf("stop polling: commit offset: %w", err)
		}
		if len(us) == 0 {
			return nil
		}
		if err := p.handOver(ctx, us); err != nil {
			return err
		}
	}
}

// backoff is 1s doubled per consecutive failure, capped at 60s, with ±20%
// jitter, clamped to [1s, 60s].
func (p *Poller) backoff(failures int) time.Duration {
	d := minBackoff
	for i := 1; i < failures && d < maxBackoff; i++ {
		d *= 2
	}
	j := rand.Float64
	if p.Jitter != nil {
		j = p.Jitter
	}
	d = time.Duration(float64(d) * (0.8 + 0.4*j()))
	return min(max(d, minBackoff), maxBackoff)
}

func (p *Poller) sleep(ctx context.Context, d time.Duration) error {
	if p.Sleep != nil {
		return p.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Healthy returns nil when a poll cycle succeeded within twice the poll
// timeout (or the poller started less than that ago).
func (p *Poller) Healthy() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	limit := 2 * PollTimeout * time.Second
	last := p.lastSuccess
	if last.IsZero() {
		if p.started.IsZero() {
			return errors.New("polling not started")
		}
		last = p.started
	}
	if age := p.Clock.Now().Sub(last); age > limit {
		return fmt.Errorf("no successful poll for %s", age.Round(time.Second))
	}
	return nil
}
