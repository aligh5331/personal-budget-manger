package categorize_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/categorize"
)

var options = []categorize.Option{
	{ID: 3, Name: "Food", Hint: "غذا، ناهار"},
	{ID: 4, Name: "Loan installment", Hint: "قسط، وام"},
	{ID: 9, Name: "Gym", Hint: "Gym"},
	{ID: 1, Name: "Uncategorized", Hint: "نامشخص", None: true},
}

// jevServer replays the given replies in order (the last one repeats) and
// records every request body.
type jevServer struct {
	t       *testing.T
	mu      sync.Mutex
	replies []reply
	bodies  []map[string]any
	headers []http.Header
	paths   []string
}

type reply struct {
	status int
	body   string
}

func newJev(t *testing.T, replies ...reply) (*jevServer, *categorize.Jev) {
	s := &jevServer{t: t, replies: replies}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	j := categorize.NewJev("test-key")
	j.BaseURL = srv.URL
	j.Timeout = 2 * time.Second
	return s, j
}

func (s *jevServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		s.t.Errorf("request body is not JSON: %s", raw)
	}
	s.bodies = append(s.bodies, body)
	s.headers = append(s.headers, r.Header.Clone())
	s.paths = append(s.paths, r.Method+" "+r.URL.Path)
	rep := s.replies[0]
	if len(s.replies) > 1 {
		s.replies = s.replies[1:]
	}
	w.WriteHeader(rep.status)
	_, _ = io.WriteString(w, rep.body)
}

func (s *jevServer) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

func answer(choice string, confidence float64) reply {
	b, _ := json.Marshal(map[string]any{
		"model": "jev-1.13.0",
		"answers": map[string]any{"category": map[string]any{
			"type": "choice", "choice": choice, "confidence": confidence,
			"probabilities": map[string]any{choice: confidence},
		}},
		"usage": map[string]any{"input_tokens": 318, "output_tokens": 34},
	})
	return reply{http.StatusOK, string(b)}
}

func TestJevSendsAChoiceQuestionAboutTheInput(t *testing.T) {
	s, j := newJev(t, answer("food", 0.91))

	pick, err := j.Categorize(context.Background(), "bank text\nناهار", options)
	if err != nil {
		t.Fatal(err)
	}
	if pick.ID != 3 || pick.Confidence != 0.91 {
		t.Errorf("pick = %+v, want Food (3) at 0.91", pick)
	}

	if s.paths[0] != "POST /typesafe/v1/systemone" {
		t.Errorf("request = %s", s.paths[0])
	}
	if got := s.headers[0].Get("Authorization"); got != "Bearer test-key" {
		t.Errorf("Authorization = %q", got)
	}
	if got := s.headers[0].Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	body := s.bodies[0]
	if body["model"] != "jev-latest" || body["state"] != "bank text\nناهار" {
		t.Errorf("model %v state %v", body["model"], body["state"])
	}
	q := body["questions"].(map[string]any)["category"].(map[string]any)
	if q["type"] != "choice" || !strings.Contains(q["instructions"].(string), "note") {
		t.Errorf("question = %v", q)
	}
	want := map[string]any{
		"food":             "Food. غذا، ناهار",
		"loan_installment": "Loan installment. قسط، وام",
		"gym":              "Gym",
		"uncategorized":    "Uncategorized. Nothing above fits or the text is not enough. نامشخص",
	}
	criteria := q["criteria"].(map[string]any)
	if len(criteria) != len(want) {
		t.Errorf("criteria = %v, want %v", criteria, want)
	}
	for k, v := range want {
		if criteria[k] != v {
			t.Errorf("criteria[%q] = %v, want %q", k, criteria[k], v)
		}
	}
}

func TestJevOptionKeysStayUniqueAndASCII(t *testing.T) {
	s, j := newJev(t, answer("c8", 0.8))
	opts := []categorize.Option{
		{ID: 7, Name: "Café", Hint: "کافه"},
		{ID: 8, Name: "café", Hint: "کافه"},
		{ID: 2, Name: "باشگاه", Hint: "باشگاه"},
	}

	pick, err := j.Categorize(context.Background(), "x", opts)
	if err != nil {
		t.Fatal(err)
	}
	if pick.ID != 8 {
		t.Errorf("pick = %+v, want id 8", pick)
	}
	criteria := s.bodies[0]["questions"].(map[string]any)["category"].(map[string]any)["criteria"].(map[string]any)
	for _, k := range []string{"caf", "c8", "c2"} {
		if _, ok := criteria[k]; !ok {
			t.Errorf("criteria keys = %v, want %q", criteria, k)
		}
	}
}

func TestJevReadsConfidenceFromProbabilitiesWhenMissing(t *testing.T) {
	_, j := newJev(t, reply{200, `{"answers":{"category":{"type":"choice","choice":"gym","probabilities":{"gym":0.66,"food":0.34}}}}`})

	pick, err := j.Categorize(context.Background(), "x", options)
	if err != nil {
		t.Fatal(err)
	}
	if pick.ID != 9 || pick.Confidence != 0.66 {
		t.Errorf("pick = %+v, want gym at 0.66", pick)
	}
}

func TestJevRetriesOnceOnAFailure(t *testing.T) {
	cases := map[string]reply{
		"503":           {503, `{"error":"no healthy upstream"}`},
		"malformed":     {200, `{"answers":`},
		"unknown key":   answer("pizza", 0.9),
		"empty answers": {200, `{"answers":{}}`},
	}
	for name, first := range cases {
		t.Run(name, func(t *testing.T) {
			s, j := newJev(t, first, answer("food", 0.8))

			pick, err := j.Categorize(context.Background(), "x", options)
			if err != nil {
				t.Fatal(err)
			}
			if pick.ID != 3 || s.calls() != 2 {
				t.Errorf("pick %+v after %d calls, want Food after 2", pick, s.calls())
			}
		})
	}
}

func TestJevGivesUpAfterTwoFailures(t *testing.T) {
	s, j := newJev(t, reply{503, "upstream connect error"})

	if _, err := j.Categorize(context.Background(), "x", options); err == nil {
		t.Fatal("want an error")
	}
	if s.calls() != 2 {
		t.Errorf("%d calls, want 2", s.calls())
	}
}

func TestJevDoesNotRetryARejectedRequest(t *testing.T) {
	s, j := newJev(t, reply{422, `{"detail":"bad"}`})

	if _, err := j.Categorize(context.Background(), "x", options); err == nil {
		t.Fatal("want an error")
	}
	if s.calls() != 1 {
		t.Errorf("%d calls, want 1", s.calls())
	}
}

func TestJevTimesOutAndRetries(t *testing.T) {
	var calls int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
			return
		}
		rep := answer("food", 0.9)
		_, _ = io.WriteString(w, rep.body)
	}))
	t.Cleanup(srv.Close)
	j := categorize.NewJev("k")
	j.BaseURL = srv.URL
	j.Timeout = 100 * time.Millisecond

	pick, err := j.Categorize(context.Background(), "x", options)
	if err != nil {
		t.Fatal(err)
	}
	if pick.ID != 3 {
		t.Errorf("pick = %+v", pick)
	}
}
