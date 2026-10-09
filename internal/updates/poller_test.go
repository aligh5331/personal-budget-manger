package updates_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/updates"
)

// fakeBale serves a scripted sequence of getUpdates replies. After the script
// runs out it cancels the test context so Poller.Run returns.
type fakeBale struct {
	t       *testing.T
	cancel  context.CancelFunc
	mu      sync.Mutex
	replies []string // JSON bodies for getUpdates, in order
	calls   []string // "getUpdates offset=N" / "deleteWebhook"
}

func (f *fakeBale) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	if method == "deleteWebhook" {
		f.calls = append(f.calls, "deleteWebhook")
		_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
		return
	}
	var p bale.GetUpdatesParams
	_ = json.NewDecoder(r.Body).Decode(&p)
	if p.Timeout != 25 || p.Limit != 100 {
		f.t.Errorf("getUpdates timeout=%d limit=%d, want 25 and 100", p.Timeout, p.Limit)
	}
	f.calls = append(f.calls, "getUpdates offset="+itoa(p.Offset))
	if len(f.replies) == 0 {
		f.cancel()
		_, _ = io.WriteString(w, `{"ok":true,"result":[]}`)
		return
	}
	body := f.replies[0]
	f.replies = f.replies[1:]
	if strings.HasPrefix(body, "HTTP500") {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = io.WriteString(w, body)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

type run struct {
	slept   []time.Duration
	handled []int64
	calls   []string
	poller  *updates.Poller
}

func runPoller(t *testing.T, startOffset int64, replies ...string) run {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fb := &fakeBale{t: t, cancel: cancel, replies: replies}
	srv := httptest.NewServer(fb)
	defer srv.Close()

	var r run
	p := &updates.Poller{
		Bale:   bale.NewHTTPClient(srv.URL, "TOKEN"),
		Offset: startOffset,
		Handle: func(_ context.Context, u bale.Update) error {
			r.handled = append(r.handled, u.UpdateID)
			return nil
		},
		Sleep: func(ctx context.Context, d time.Duration) error {
			r.slept = append(r.slept, d)
			return ctx.Err()
		},
		Jitter: func() float64 { return 0.5 }, // no jitter: factor 1.0
		Clock:  clock.NewFake(time.Unix(0, 0)),
		Log:    slog.New(slog.DiscardHandler),
	}
	_ = p.Run(ctx)
	r.calls = fb.calls
	r.poller = p
	return r
}

func TestPollerHandsOverUpdatesAndAdvancesOffset(t *testing.T) {
	r := runPoller(t, 0,
		`{"ok":true,"result":[{"update_id":3},{"update_id":4}]}`,
		`{"ok":true,"result":[{"update_id":5}]}`,
	)
	if got, want := r.handled, []int64{3, 4, 5}; !slices.Equal(got, want) {
		t.Fatalf("handled %v, want %v", got, want)
	}
	if want := []string{"getUpdates offset=0", "getUpdates offset=5", "getUpdates offset=6"}; !slices.Equal(r.calls, want) {
		t.Fatalf("calls %v, want %v", r.calls, want)
	}
	if len(r.slept) != 0 {
		t.Fatalf("slept %v after successes, want re-poll at once", r.slept)
	}
}

func TestPollerStartsAfterPersistedOffset(t *testing.T) {
	r := runPoller(t, 42)
	if r.calls[0] != "getUpdates offset=42" {
		t.Fatalf("first call %q", r.calls[0])
	}
}

func TestPollerBacksOffExponentiallyAndResets(t *testing.T) {
	fail := "HTTP500"
	r := runPoller(t, 0, fail, fail, fail, fail, fail, fail, fail, fail,
		`{"ok":true,"result":[]}`, fail)
	want := []time.Duration{1, 2, 4, 8, 16, 32, 60, 60, 1}
	for i := range want {
		want[i] *= time.Second
	}
	if !slices.Equal(r.slept, want) {
		t.Fatalf("slept %v, want %v", r.slept, want)
	}
}

func TestPollerWaitsRetryAfterOn429(t *testing.T) {
	r := runPoller(t, 0,
		`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":7}}`)
	if !slices.Equal(r.slept, []time.Duration{7 * time.Second}) {
		t.Fatalf("slept %v, want [7s]", r.slept)
	}
}

func TestPollerDeletesWebhookOnceAndRetries(t *testing.T) {
	webhookSet := `{"ok":false,"error_code":409,"description":"Conflict: can't use getUpdates method while webhook is active"}`
	r := runPoller(t, 0, webhookSet, `{"ok":true,"result":[{"update_id":1}]}`)
	want := []string{"getUpdates offset=0", "deleteWebhook", "getUpdates offset=0", "getUpdates offset=2"}
	if !slices.Equal(r.calls, want) {
		t.Fatalf("calls %v, want %v", r.calls, want)
	}
	if len(r.slept) != 0 {
		t.Fatalf("slept %v, want an immediate retry", r.slept)
	}
}

func TestPollerHealth(t *testing.T) {
	r := runPoller(t, 0)
	clk := r.poller.Clock.(*clock.Fake)
	if err := r.poller.Healthy(); err != nil {
		t.Fatalf("healthy right after a cycle: %v", err)
	}
	clk.Advance(51 * time.Second)
	if err := r.poller.Healthy(); err == nil {
		t.Fatal("want unhealthy when no cycle finished within twice the poll timeout")
	}
}
