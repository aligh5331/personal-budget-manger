package extract_test

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

	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/extract"
)

var now = time.Date(2026, 10, 6, 12, 0, 0, 0, clock.Tehran())

// recorded is a Metis chat completion reply as the OpenAI route sends it.
const recordedContent = `{"is_transaction":true,"note":"تنقلات","transactions":[{"amount":1240000,"amount_unit":"rial","direction":"out","date":"1405/07/04","time":"12:05","bank_label":"#برداشت_با_POS","confidence":0.95}]}`

func completion(content string) string {
	b, _ := json.Marshal(map[string]any{
		"id":      "chatcmpl-1",
		"object":  "chat.completion",
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 600, "completion_tokens": 80},
	})
	return string(b)
}

// server replies with the scripted handlers in turn and records requests.
type server struct {
	mu       sync.Mutex
	requests []map[string]any
	headers  []http.Header
	replies  []func(w http.ResponseWriter)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	_ = json.Unmarshal(body, &req)
	s.mu.Lock()
	n := len(s.requests)
	s.requests = append(s.requests, req)
	s.headers = append(s.headers, r.Header.Clone())
	reply := s.replies[min(n, len(s.replies)-1)]
	s.mu.Unlock()
	if r.URL.Path != "/openai/v1/chat/completions" {
		http.NotFound(w, r)
		return
	}
	reply(w)
}

func ok(content string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, completion(content))
	}
}

func status(code int, body string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}
}

// dropped closes the connection without a response.
func dropped(w http.ResponseWriter) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err == nil {
		_ = conn.Close()
	}
}

func newClient(t *testing.T, s *server) *extract.Metis {
	t.Helper()
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	m := extract.NewMetis("test-key")
	m.BaseURL = ts.URL
	return m
}

func TestMetisRequestShapeAndParsing(t *testing.T) {
	s := &server{replies: []func(http.ResponseWriter){ok(recordedContent)}}
	m := newClient(t, s)

	input := "مبلغ: *۱,۲۴۰,۰۰۰-* ریال\nتنقلات"
	res, err := m.Extract(context.Background(), input, now)
	if err != nil {
		t.Fatal(err)
	}
	want := extract.Result{IsTransaction: true, Note: "تنقلات", Transactions: []extract.Item{{
		Amount: 1240000, AmountUnit: "rial", Direction: "out", Date: "1405/07/04", Time: "12:05",
		BankLabel: "#برداشت_با_POS", Confidence: 0.95,
	}}}
	if len(res.Transactions) != 1 || res.Transactions[0] != want.Transactions[0] || res.Note != want.Note || !res.IsTransaction {
		t.Errorf("result = %+v\nwant     %+v", res, want)
	}

	if len(s.requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(s.requests))
	}
	if got := s.headers[0].Get("Authorization"); got != "Bearer test-key" {
		t.Errorf("Authorization = %q", got)
	}
	req := s.requests[0]
	if req["model"] != "gpt-4.1-mini" {
		t.Errorf("model = %v", req["model"])
	}
	if temp, ok := req["temperature"]; !ok || temp != float64(0) {
		t.Errorf("temperature = %v (present=%v), want 0", temp, ok)
	}
	rf, _ := req["response_format"].(map[string]any)
	js, _ := rf["json_schema"].(map[string]any)
	if rf["type"] != "json_schema" || js["strict"] != true || js["name"] != "input_extraction" {
		t.Errorf("response_format = %v", rf)
	}
	schema, _ := js["schema"].(map[string]any)
	items := schema["properties"].(map[string]any)["transactions"].(map[string]any)["items"].(map[string]any)
	if _, ok := items["properties"].(map[string]any)["bank_label"]; !ok {
		t.Errorf("schema has no bank_label: %v", items)
	}
	msgs, _ := req["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages = %v", msgs)
	}
	sys := msgs[0].(map[string]any)
	user := msgs[1].(map[string]any)
	if sys["role"] != "system" || !strings.Contains(sys["content"].(string), "Today is 1405/07/14 (Jalali, Asia/Tehran)") {
		t.Errorf("system message = %v", sys)
	}
	if user["role"] != "user" || user["content"] != input {
		t.Errorf("user message = %v, want the Input verbatim", user)
	}
}

func TestMetisRetriesOnceOnTheFallbackModel(t *testing.T) {
	tests := []struct {
		name  string
		first func(http.ResponseWriter)
	}{
		{"5xx", status(http.StatusServiceUnavailable, "upstream connect error")},
		{"dropped connection", dropped},
		{"invalid JSON content", ok(`{"is_transaction": tru`)},
		{"no choices", func(w http.ResponseWriter) { _, _ = io.WriteString(w, `{"choices":[]}`) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &server{replies: []func(http.ResponseWriter){tc.first, ok(recordedContent)}}
			m := newClient(t, s)
			res, err := m.Extract(context.Background(), "x", now)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			if !res.IsTransaction || len(res.Transactions) != 1 {
				t.Errorf("result = %+v", res)
			}
			if len(s.requests) != 2 {
				t.Fatalf("got %d requests, want 2", len(s.requests))
			}
			retry := s.requests[1]
			if retry["model"] != "gpt-5-mini" {
				t.Errorf("retry model = %v, want gpt-5-mini", retry["model"])
			}
			if _, has := retry["temperature"]; has {
				t.Errorf("gpt-5-mini rejects temperature, but the retry sent it")
			}
		})
	}
}

func TestMetisFailsAfterTwoAttempts(t *testing.T) {
	s := &server{replies: []func(http.ResponseWriter){status(http.StatusServiceUnavailable, "no healthy upstream")}}
	m := newClient(t, s)
	if _, err := m.Extract(context.Background(), "x", now); err == nil {
		t.Fatal("want an error")
	}
	if len(s.requests) != 2 {
		t.Errorf("got %d requests, want exactly 2", len(s.requests))
	}
}

func TestMetisDoesNotRetryAClientError(t *testing.T) {
	s := &server{replies: []func(http.ResponseWriter){status(http.StatusUnauthorized, `{"error":"no key"}`)}}
	m := newClient(t, s)
	if _, err := m.Extract(context.Background(), "x", now); err == nil {
		t.Fatal("want an error")
	}
	if len(s.requests) != 1 {
		t.Errorf("got %d requests, want 1", len(s.requests))
	}
}

func TestMetisTimeoutIsRetried(t *testing.T) {
	slow := func(w http.ResponseWriter) { time.Sleep(300 * time.Millisecond); ok(recordedContent)(w) }
	s := &server{replies: []func(http.ResponseWriter){slow, ok(recordedContent)}}
	m := newClient(t, s)
	m.Timeout = 50 * time.Millisecond
	if _, err := m.Extract(context.Background(), "x", now); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(s.requests) != 2 {
		t.Errorf("got %d requests, want 2", len(s.requests))
	}
}
