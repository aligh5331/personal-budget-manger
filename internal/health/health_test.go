package health_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/health"
)

func get(t *testing.T, h http.Handler) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type %q", ct)
	}
	return rec.Code, body
}

func TestHealthyWhenAllChecksPass(t *testing.T) {
	code, body := get(t, health.Handler(
		health.Check{Name: "db", Func: func() error { return nil }},
		health.Check{Name: "updates", Func: func() error { return nil }},
	))
	if code != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("got %d %v", code, body)
	}
}

func TestUnhealthyNamesFailingPart(t *testing.T) {
	code, body := get(t, health.Handler(
		health.Check{Name: "db", Func: func() error { return nil }},
		health.Check{Name: "updates", Func: func() error { return errors.New("no successful poll for 1m0s") }},
	))
	if code != http.StatusServiceUnavailable {
		t.Fatalf("code %d", code)
	}
	failing, _ := body["failing"].(map[string]any)
	if failing["updates"] != "no successful poll for 1m0s" || len(failing) != 1 {
		t.Fatalf("failing = %v", body["failing"])
	}
}
