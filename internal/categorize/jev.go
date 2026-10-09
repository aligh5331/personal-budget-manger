package categorize

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Jev defaults.
const (
	DefaultBaseURL = "https://api.metisai.ir"
	DefaultModel   = "jev-latest"
	systemOnePath  = "/typesafe/v1/systemone"
	// maxOptions is Jev's limit on choice options.
	maxOptions = 255
	questionID = "category"
)

const instructions = "Which category does this transaction belong to? The Owner's own note, if any, decides over the bank label."

// noneText follows the "none" option's name in its criteria text.
const noneText = "Nothing above fits or the text is not enough"

// Jev is the production Categorizer: Jev on Metis (TypeSafe systemone) with
// one Choice question. A dropped call, HTTP 5xx, a timeout or an unreadable
// reply is retried once, immediately.
type Jev struct {
	BaseURL string
	APIKey  string
	Model   string
	// Timeout bounds each attempt.
	Timeout time.Duration
	HTTP    *http.Client
}

var _ Categorizer = (*Jev)(nil)

// NewJev returns a Jev Categorizer with the spec's model.
func NewJev(apiKey string) *Jev {
	return &Jev{
		BaseURL: DefaultBaseURL,
		APIKey:  apiKey,
		Model:   DefaultModel,
		Timeout: 15 * time.Second,
		HTTP:    &http.Client{},
	}
}

// retryableError marks a failure worth one retry.
type retryableError struct{ err error }

func (e retryableError) Error() string { return e.err.Error() }
func (e retryableError) Unwrap() error { return e.err }

// Categorize implements Categorizer.
func (j *Jev) Categorize(ctx context.Context, text string, options []Option) (Pick, error) {
	if len(options) == 0 || len(options) > maxOptions {
		return Pick{}, fmt.Errorf("jev: %d options, want 1-%d", len(options), maxOptions)
	}
	keys := optionKeys(options)
	body, err := json.Marshal(request{
		Model: j.Model,
		State: text,
		Questions: map[string]question{questionID: {
			Type:         "choice",
			Instructions: instructions,
			Criteria:     criteria(options, keys),
		}},
	})
	if err != nil {
		return Pick{}, err
	}
	pick, err := j.attempt(ctx, body, options, keys)
	var re retryableError
	if err == nil || !errors.As(err, &re) || ctx.Err() != nil {
		return pick, err
	}
	pick, err2 := j.attempt(ctx, body, options, keys)
	if err2 != nil {
		return Pick{}, fmt.Errorf("jev: failed (%w), then failed again: %w", err, err2)
	}
	return pick, nil
}

type request struct {
	Model     string              `json:"model"`
	State     string              `json:"state"`
	Questions map[string]question `json:"questions"`
}

type question struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

type response struct {
	Answers map[string]struct {
		Choice        string             `json:"choice"`
		Confidence    *float64           `json:"confidence"`
		Probabilities map[string]float64 `json:"probabilities"`
		Distribution  map[string]float64 `json:"distribution"`
	} `json:"answers"`
}

func (j *Jev) attempt(ctx context.Context, body []byte, options []Option, keys []string) (Pick, error) {
	actx, cancel := context.WithTimeout(ctx, j.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(actx, http.MethodPost, strings.TrimRight(j.BaseURL, "/")+systemOnePath, bytes.NewReader(body))
	if err != nil {
		return Pick{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+j.APIKey)

	resp, err := j.HTTP.Do(req)
	if err != nil {
		return Pick{}, retryableError{err}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Pick{}, retryableError{fmt.Errorf("read reply: %w", err)}
	}
	if resp.StatusCode >= 500 {
		return Pick{}, retryableError{fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet(raw))}
	}
	if resp.StatusCode != http.StatusOK {
		return Pick{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet(raw))
	}

	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return Pick{}, retryableError{fmt.Errorf("unreadable reply: %s", snippet(raw))}
	}
	a, ok := r.Answers[questionID]
	if !ok || a.Choice == "" {
		return Pick{}, retryableError{fmt.Errorf("no choice in reply: %s", snippet(raw))}
	}
	for i, k := range keys {
		if k != a.Choice {
			continue
		}
		conf := 0.0
		switch {
		case a.Confidence != nil:
			conf = *a.Confidence
		case a.Probabilities != nil:
			conf = a.Probabilities[k]
		default:
			conf = a.Distribution[k]
		}
		return Pick{ID: options[i].ID, Confidence: conf}, nil
	}
	return Pick{}, retryableError{fmt.Errorf("choice %q was not offered", a.Choice)}
}

// optionKeys gives each option a short ASCII key: its name as a slug
// ("Loan installment" -> "loan_installment"), or "c<id>" when the slug is
// empty or taken.
func optionKeys(options []Option) []string {
	keys := make([]string, len(options))
	used := map[string]bool{}
	for i, o := range options {
		k := slug(o.Name)
		if k == "" || used[k] {
			k = "c" + strconv.FormatInt(o.ID, 10)
		}
		used[k] = true
		keys[i] = k
	}
	return keys
}

func slug(name string) string {
	var b strings.Builder
	underscore := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if underscore && b.Len() > 0 {
				b.WriteByte('_')
			}
			underscore = false
			b.WriteRune(r)
			if b.Len() >= 40 {
				break
			}
			continue
		}
		underscore = true
	}
	return b.String()
}

// criteria maps each key to "<Name>. <hint>" (just the name when the hint
// adds nothing). The none option also says when to pick it.
func criteria(options []Option, keys []string) map[string]string {
	c := make(map[string]string, len(options))
	for i, o := range options {
		parts := []string{o.Name}
		if o.None {
			parts = append(parts, noneText)
		}
		if h := strings.TrimSpace(o.Hint); h != "" && h != o.Name {
			parts = append(parts, h)
		}
		c[keys[i]] = strings.Join(parts, ". ")
	}
	return c
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
