//go:build live

// Live evaluation (#47). Opt-in and never part of CI or a plain `go test`:
//
//	LLM_API_KEY=... go test -tags live -run TestLiveEvaluation -v -timeout 20m ./internal/liveeval/
//
// It sends the 18 cases of the gitignored samples/forwarded-messages.txt
// through the real Metis Extractor, the Input rules and the Jev Categorizer
// and prints per-field scores, cost and latency. It costs real money
// (about $0.01 per run of 18 cases).
//
// Environment:
//
//	LLM_API_KEY    required (skips when empty)
//	LIVE_SAMPLES   samples file; default: samples/forwarded-messages.txt found
//	               by walking up from here (works from a worktree too)
//	LIVE_RUNS      repeat the whole set N times (default 1)
package liveeval_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/categorize"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/extract"
	"github.com/aligh5331/personal-budget-manger/internal/inputrules"
	"github.com/aligh5331/personal-budget-manger/internal/jalali"
	"github.com/aligh5331/personal-budget-manger/internal/liveeval"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
	"github.com/aligh5331/personal-budget-manger/internal/storage/sqlite"
)

// List prices from the research (USD per token, incl. Metis's +10%).
const (
	extractInPrice  = 0.44 / 1e6 // gpt-4.1-mini; a gpt-5-mini retry is priced the same (approximate)
	extractOutPrice = 1.76 / 1e6
	jevInPrice      = 0.0462 / 1e6
)

// meter is an http.RoundTripper that adds up token cost from reply bodies.
type meter struct {
	next http.RoundTripper
	mu   sync.Mutex
	cost float64
}

func (m *meter) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := m.next.RoundTrip(r)
	if err != nil {
		return resp, err
	}
	raw, rerr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	if rerr != nil || resp.StatusCode != http.StatusOK {
		return resp, nil
	}
	var u struct {
		Usage struct {
			PromptTokens     float64 `json:"prompt_tokens"`
			CompletionTokens float64 `json:"completion_tokens"`
			InputTokens      float64 `json:"input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &u) == nil {
		c := u.Usage.InputTokens * jevInPrice
		if strings.Contains(r.URL.Path, "/chat/completions") {
			c = u.Usage.PromptTokens*extractInPrice + u.Usage.CompletionTokens*extractOutPrice
		}
		m.mu.Lock()
		m.cost += c
		m.mu.Unlock()
	}
	return resp, nil
}

func (m *meter) total() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cost
}

// samplesPath finds the samples file, or "".
func samplesPath() string {
	if p := os.Getenv("LIVE_SAMPLES"); p != "" {
		return p
	}
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		p := filepath.Join(dir, "samples", "forwarded-messages.txt")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// categoryKey turns a Category name into the harness's key.
func categoryKey(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), " ", "_")
}

type tally struct{ amount, direction, date, category, description, all int }

func TestLiveEvaluation(t *testing.T) {
	key := os.Getenv("LLM_API_KEY")
	if key == "" {
		t.Skip("LLM_API_KEY not set")
	}
	path := samplesPath()
	if path == "" {
		t.Skip("samples/forwarded-messages.txt not found (set LIVE_SAMPLES)")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("samples file unreadable: %v", err)
	}
	cases := liveeval.ParseSamples(string(raw))
	if len(cases) == 0 {
		t.Fatalf("no cases found in %s", path)
	}
	runs := 1
	if s := os.Getenv("LIVE_RUNS"); s != "" {
		if runs, err = strconv.Atoi(s); err != nil || runs < 1 {
			t.Fatalf("LIVE_RUNS=%q", s)
		}
	}

	ctx := context.Background()
	st, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "eval.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	un, err := st.Uncategorized(ctx)
	if err != nil {
		t.Fatal(err)
	}
	optionsFor := map[string][]categorize.Option{}
	for _, kind := range []string{storage.KindExpense, storage.KindIncome} {
		cs, err := st.ActiveCategories(ctx, kind)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			optionsFor[kind] = append(optionsFor[kind], categorize.Option{ID: c.ID, Name: c.Name, Hint: c.Hint})
		}
		optionsFor[kind] = append(optionsFor[kind], categorize.Option{ID: un.ID, Name: un.Name, Hint: un.Hint, None: true})
	}
	names := map[int64]string{un.ID: categoryKey(un.Name)}
	for _, opts := range optionsFor {
		for _, o := range opts {
			names[o.ID] = categoryKey(o.Name)
		}
	}

	m := &meter{next: http.DefaultTransport}
	hc := &http.Client{Transport: m}
	ex := extract.NewMetis(key)
	ex.HTTP = hc
	jev := categorize.NewJev(key)
	jev.HTTP = hc

	var today jalali.Date
	parts := strings.Split(liveeval.Today, "/")
	today.Year, _ = strconv.Atoi(parts[0])
	today.Month, _ = strconv.Atoi(parts[1])
	today.Day, _ = strconv.Atoi(parts[2])
	now := today.At(12, 0, clock.Tehran())

	var tot tally
	var latencies []time.Duration
	n := 0
	for run := 1; run <= runs; run++ {
		var rt tally
		for _, c := range cases {
			want, err := liveeval.ParseWant(c)
			if err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			got, notes := runPipeline(ctx, t, ex, jev, optionsFor, names, c.Input, now)
			lat := time.Since(start)
			latencies = append(latencies, lat)
			v := liveeval.Score(want, got)
			n++
			rt.add(v)
			status := "ok"
			if !v.OK() {
				status = "FAIL"
				t.Errorf("run %d case %02d: %s", run, c.N, strings.Join(v.Fails, "; "))
			}
			t.Logf("run %d case %02d %-4s amount=%s dir=%s date=%s cat=%s desc=%s  %.1fs  %s",
				run, c.N, status, mark(v.Amount), mark(v.Direction), mark(v.Date), mark(v.Category), mark(v.Description),
				lat.Seconds(), notes)
			if !v.Description {
				t.Logf("    description miss: %s", strings.Join(v.Fails, "; "))
			}
		}
		t.Logf("run %d: amount %d/%d  direction %d/%d  date %d/%d  category %d/%d  description %d/%d  all-four %d/%d",
			run, rt.amount, len(cases), rt.direction, len(cases), rt.date, len(cases), rt.category, len(cases),
			rt.description, len(cases), rt.all, len(cases))
		tot.amount += rt.amount
		tot.direction += rt.direction
		tot.date += rt.date
		tot.category += rt.category
		tot.description += rt.description
		tot.all += rt.all
	}
	t.Logf("TOTAL over %d cases: amount %d/%d  direction %d/%d  date %d/%d  category %d/%d  description %d/%d",
		n, tot.amount, n, tot.direction, n, tot.date, n, tot.category, n, tot.description, n)
	t.Logf("latency per case (extract + category): p50 %.1fs  p95 %.1fs  max %.1fs",
		pct(latencies, 50).Seconds(), pct(latencies, 95).Seconds(), pct(latencies, 100).Seconds())
	t.Logf("cost ~$%.5f", m.total())
}

func (r *tally) add(v liveeval.Verdict) {
	for _, p := range []struct {
		n *int
		b bool
	}{{&r.amount, v.Amount}, {&r.direction, v.Direction}, {&r.date, v.Date}, {&r.category, v.Category}, {&r.description, v.Description}} {
		if p.b {
			*p.n++
		}
	}
	if v.OK() {
		r.all++
	}
}

func mark(ok bool) string {
	if ok {
		return "ok"
	}
	return "MISS"
}

// runPipeline is the production flow minus storage: extract, Input rules,
// then the Category step for each draft (the bot's categorizeTransaction
// logic: kind from Direction, Jev over the whole Input, below
// bot.MinCategoryConfidence becomes Uncategorized).
func runPipeline(ctx context.Context, t *testing.T, ex extract.Extractor, jev categorize.Categorizer,
	options map[string][]categorize.Option, names map[int64]string, input string, now time.Time) (liveeval.Got, string) {
	t.Helper()
	res, err := ex.Extract(ctx, input, now)
	if err != nil {
		return liveeval.Got{}, "extract error: " + err.Error()
	}
	out := inputrules.Apply(inputrules.Input{Text: input, Extraction: res, Now: now})
	got := liveeval.Got{IsTransaction: out.IsTransaction}
	var notes []string
	for _, d := range out.Drafts {
		if d.HasAmount {
			a := d.AmountToman
			got.AmountsToman = append(got.AmountsToman, &a)
		} else {
			got.AmountsToman = append(got.AmountsToman, nil)
		}
		got.Directions = append(got.Directions, string(d.Direction))
		got.Dates = append(got.Dates, jalali.FromTime(d.OccurredAt.In(clock.Tehran())).String())
		got.Descriptions = append(got.Descriptions, d.Description)

		cat := ""
		if d.Direction != inputrules.Internal {
			kind := storage.KindExpense // ambiguous reads as expense, like the research harness
			if d.Direction == inputrules.In {
				kind = storage.KindIncome
			}
			opts := options[kind]
			cat = "uncategorized"
			pick, err := jev.Categorize(ctx, input, opts)
			switch {
			case err != nil:
				notes = append(notes, "jev error: "+err.Error())
			case pick.Confidence >= 0.7:
				cat = names[pick.ID]
				notes = append(notes, cat+" "+strconv.FormatFloat(pick.Confidence, 'f', 2, 64))
			default:
				notes = append(notes, "below gate: "+names[pick.ID]+" "+strconv.FormatFloat(pick.Confidence, 'f', 2, 64))
			}
		}
		got.Categories = append(got.Categories, cat)
	}
	return got, strings.Join(notes, ", ")
}

func pct(d []time.Duration, p int) time.Duration {
	if len(d) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), d...)
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	i := (len(s)*p + 99) / 100
	if i < 1 {
		i = 1
	}
	return s[i-1]
}
