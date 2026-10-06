package updates_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/updates"
)

const secret = "0123456789abcdef0123456789abcdef" // 32 characters

func newWebhook(t *testing.T) (http.Handler, *[]int64) {
	t.Helper()
	var got []int64
	h, err := updates.NewWebhookHandler(secret, func(_ context.Context, u bale.Update) error {
		got = append(got, u.UpdateID)
		return nil
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewWebhookHandler: %v", err)
	}
	return h, &got
}

func serve(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestWebhookAcceptsUpdateOnSecretPath(t *testing.T) {
	h, got := newWebhook(t)
	rec := serve(h, http.MethodPost, "/bale/"+secret, `{"update_id":7,"message":{"message_id":1,"chat":{"id":5,"type":"private"},"text":"hi"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d, want 200", rec.Code)
	}
	if !slices.Equal(*got, []int64{7}) {
		t.Fatalf("handed over %v, want [7]", *got)
	}
}

func TestWebhookRejectsWrongPathWithBare404(t *testing.T) {
	h, got := newWebhook(t)
	for _, path := range []string{
		"/bale/" + secret[:31],
		"/bale/" + secret + "x",
		"/bale/" + strings.ToUpper(secret),
		"/bale/",
		"/bale/" + secret + "/",
		"/" + secret,
	} {
		for _, method := range []string{http.MethodPost, http.MethodGet} {
			rec := serve(h, method, path, `{"update_id":1}`)
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s: code %d, want 404", method, path, rec.Code)
			}
			if rec.Body.Len() != 0 {
				t.Errorf("%s %s: body %q, want a bare 404", method, path, rec.Body.String())
			}
		}
	}
	if len(*got) != 0 {
		t.Fatalf("handed over %v from wrong paths", *got)
	}
}

func TestWebhookRejectsNonPost(t *testing.T) {
	h, got := newWebhook(t)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodHead, http.MethodDelete} {
		rec := serve(h, method, "/bale/"+secret, `{"update_id":1}`)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: code %d, want 405", method, rec.Code)
		}
	}
	if len(*got) != 0 {
		t.Fatalf("handed over %v from non-POST requests", *got)
	}
}

func TestWebhookRejectsBodiesOverOneMegabyte(t *testing.T) {
	h, got := newWebhook(t)
	// Valid JSON followed by padding: only the size makes it bad.
	body := `{"update_id":1}` + strings.Repeat(" ", 1<<20)
	rec := serve(h, http.MethodPost, "/bale/"+secret, body)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("code %d, want 413", rec.Code)
	}
	if len(*got) != 0 {
		t.Fatalf("handed over %v from an oversized body", *got)
	}

	// Just under the cap is fine.
	body = `{"update_id":2}` + strings.Repeat(" ", 1<<20-len(`{"update_id":2}`))
	if rec := serve(h, http.MethodPost, "/bale/"+secret, body); rec.Code != http.StatusOK {
		t.Fatalf("1 MB body: code %d, want 200", rec.Code)
	}
}

func TestWebhookRejectsUndecodableJSON(t *testing.T) {
	h, got := newWebhook(t)
	for _, body := range []string{"", "not json", `{"update_id":`, `[1,2]`, `{"message":{}}`, `{"update_id":"x"}`} {
		rec := serve(h, http.MethodPost, "/bale/"+secret, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: code %d, want 400", body, rec.Code)
		}
	}
	if len(*got) != 0 {
		t.Fatalf("handed over %v from bad bodies", *got)
	}
}

func TestWebhookHandsOverNonOwnerUpdatesForTheCoreToDrop(t *testing.T) {
	// The Owner check lives in the bot core (shared with polling); the
	// webhook acks every well-formed update with 200 so Bale does not retry.
	h, got := newWebhook(t)
	rec := serve(h, http.MethodPost, "/bale/"+secret, `{"update_id":9,"message":{"message_id":1,"from":{"id":777},"chat":{"id":-5,"type":"group"},"text":"spam"}}`)
	if rec.Code != http.StatusOK || !slices.Equal(*got, []int64{9}) {
		t.Fatalf("code %d, handed over %v", rec.Code, *got)
	}
}

func TestWebhookRefusesShortSecret(t *testing.T) {
	for _, s := range []string{"", "short", secret[:31]} {
		if _, err := updates.NewWebhookHandler(s, func(context.Context, bale.Update) error { return nil }, nil); err == nil {
			t.Errorf("secret of %d characters accepted", len(s))
		} else if s != "" && strings.Contains(err.Error(), s) {
			t.Errorf("error leaks the secret: %v", err)
		}
	}
}
