package updates

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
)

// Webhook settings from the spec.
const (
	// MinSecretLen is the shortest WEBHOOK_SECRET_PATH webhook mode accepts.
	MinSecretLen = 32
	// MaxWebhookBody caps one webhook request body.
	MaxWebhookBody = 1 << 20
	// WebhookPathPrefix is where the webhook is served: WebhookPathPrefix + secret.
	WebhookPathPrefix = "/bale/"
)

// CheckSecret returns an error when secret is too short for webhook mode. The
// error never contains the secret.
func CheckSecret(secret string) error {
	if len(secret) < MinSecretLen {
		return fmt.Errorf("WEBHOOK_SECRET_PATH must be at least %d characters (it has %d)", MinSecretLen, len(secret))
	}
	return nil
}

// NewWebhookHandler returns the handler for Bale webhook deliveries, to be
// mounted at WebhookPathPrefix. Only POSTs to WebhookPathPrefix+secret are
// accepted; every well-formed update is handed to enqueue (normally
// Worker.Enqueue, the same in-order worker polling uses) and acked with 200
// at once. Owner checks and dedupe happen in the bot core.
//
// Responses: 404 (empty body) for any other path, 405 for non-POST, 413 for
// bodies over MaxWebhookBody, 400 for undecodable JSON.
func NewWebhookHandler(secret string, enqueue func(ctx context.Context, u bale.Update) error, log *slog.Logger) (http.Handler, error) {
	if err := CheckSecret(secret); err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	want := sha256.Sum256([]byte(WebhookPathPrefix + secret))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hashing first makes the comparison constant-time in the length too.
		got := sha256.Sum256([]byte(r.URL.Path))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxWebhookBody))
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		u, err := decodeUpdate(body)
		if err != nil {
			log.Warn("webhook: undecodable update", "err", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := enqueue(r.Context(), u); err != nil {
			// Shutting down: let Bale deliver it again later.
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}), nil
}

// decodeUpdate parses one Update and insists on an update_id, which the
// core's dedupe needs.
func decodeUpdate(body []byte) (bale.Update, error) {
	var probe struct {
		UpdateID *int64 `json:"update_id"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return bale.Update{}, err
	}
	if probe.UpdateID == nil {
		return bale.Update{}, errors.New("no update_id")
	}
	var u bale.Update
	if err := json.Unmarshal(body, &u); err != nil {
		return bale.Update{}, err
	}
	return u, nil
}
