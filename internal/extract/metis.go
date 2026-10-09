package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/jalali"
)

// Metis defaults.
const (
	DefaultMetisBaseURL = "https://api.metisai.ir"
	PrimaryModel        = "gpt-4.1-mini"
	FallbackModel       = "gpt-5-mini"
	chatPath            = "/openai/v1/chat/completions"
)

// Metis is the production Extractor: Metis's OpenAI-compatible chat
// completions with a strict json_schema. A dropped call, HTTP 5xx, a timeout
// or an unreadable reply is retried once, immediately, on FallbackModel.
type Metis struct {
	BaseURL       string
	APIKey        string
	Model         string
	FallbackModel string
	// Timeout bounds each attempt.
	Timeout time.Duration
	HTTP    *http.Client
}

var _ Extractor = (*Metis)(nil)

// NewMetis returns a Metis extractor with the spec's models.
func NewMetis(apiKey string) *Metis {
	return &Metis{
		BaseURL:       DefaultMetisBaseURL,
		APIKey:        apiKey,
		Model:         PrimaryModel,
		FallbackModel: FallbackModel,
		Timeout:       45 * time.Second,
		HTTP:          &http.Client{},
	}
}

// retryableError marks a failure worth one retry.
type retryableError struct{ err error }

func (e retryableError) Error() string { return e.err.Error() }
func (e retryableError) Unwrap() error { return e.err }

// Extract implements Extractor.
func (m *Metis) Extract(ctx context.Context, input string, now time.Time) (Result, error) {
	system := SystemPrompt(jalali.FromTime(now.In(clock.Tehran())))
	res, err := m.attempt(ctx, m.Model, system, input)
	var re retryableError
	if err == nil || !errors.As(err, &re) || ctx.Err() != nil {
		return res, err
	}
	res, err2 := m.attempt(ctx, m.FallbackModel, system, input)
	if err2 != nil {
		return Result{}, fmt.Errorf("extract: %s failed (%w), then %s failed: %w", m.Model, err, m.FallbackModel, err2)
	}
	return res, nil
}

type chatRequest struct {
	Model          string          `json:"model"`
	Temperature    *float64        `json:"temperature,omitempty"`
	ResponseFormat json.RawMessage `json:"response_format"`
	Messages       []chatMessage   `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func (m *Metis) attempt(ctx context.Context, model, system, input string) (Result, error) {
	req := chatRequest{
		Model:          model,
		ResponseFormat: responseFormat,
		Messages:       []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: input}},
	}
	// gpt-5 models reject temperature.
	if !strings.HasPrefix(model, "gpt-5") {
		zero := 0.0
		req.Temperature = &zero
	}
	body, err := json.Marshal(req)
	if err != nil {
		return Result{}, err
	}

	actx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()
	hreq, err := http.NewRequestWithContext(actx, http.MethodPost, strings.TrimRight(m.BaseURL, "/")+chatPath, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Authorization", "Bearer "+m.APIKey)

	resp, err := m.HTTP.Do(hreq)
	if err != nil {
		return Result{}, retryableError{fmt.Errorf("%s: %w", model, err)}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, retryableError{fmt.Errorf("%s: read reply: %w", model, err)}
	}
	if resp.StatusCode >= 500 {
		return Result{}, retryableError{fmt.Errorf("%s: HTTP %d: %s", model, resp.StatusCode, snippet(raw))}
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("%s: HTTP %d: %s", model, resp.StatusCode, snippet(raw))
	}

	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil || len(cr.Choices) == 0 {
		return Result{}, retryableError{fmt.Errorf("%s: unreadable reply: %s", model, snippet(raw))}
	}
	var res Result
	if err := json.Unmarshal([]byte(cr.Choices[0].Message.Content), &res); err != nil {
		return Result{}, retryableError{fmt.Errorf("%s: invalid JSON content: %w", model, err)}
	}
	return res, nil
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// SystemPrompt is the extraction prompt for a given Jalali "today".
func SystemPrompt(today jalali.Date) string {
	return strings.Replace(systemPrompt, "{TODAY}", today.String(), 1)
}

// systemPrompt is the research prompt (research/extraction-rerun) plus the
// bank_label rule.
const systemPrompt = `You read Persian bank messages that the user forwarded to a budgeting bot, plus an optional short note the user typed (usually after the messages, sometimes before). Digits may be ASCII, Persian or Arabic-Indic. Today is {TODAY} (Jalali, Asia/Tehran).

Return JSON with this shape:
- is_transaction: false if the input is not a money movement at all (ad, bill notice, future-tense notice, chit-chat), else true.
- note: the user's own note copied verbatim (typos kept, not translated, not corrected), or null if there is none. Bank message text is never the note.
- transactions: one entry per bank message; if there is no bank message but the note states a payment, one entry from the note.
  - amount: the amount moved, as printed, in ASCII digits, sign and separators dropped. Never the balance (مانده). null if the message has no amount line.
  - amount_unit: "rial" for any amount read from a bank message; "toman" only for an amount stated in the user's note when there is no bank message.
  - direction: "out" or "in". The sign printed after the amount decides first (- is out, + is in), then the hashtag or keyword (برداشت, خرید = out; واریز = in), then the note. A bare transfer word (انتقال) with no sign is "ambiguous". Never output anything else.
  - date: Jalali "YYYY/MM/DD" as printed. A year-less "MMDD-HH:MM" means the current year. If the note gives a day (امروز = today), use it. Otherwise null; never invent a date.
  - time: "HH:MM" as printed, or null.
  - bank_label: the bank's own label or hashtag for this message, copied as printed (e.g. "#برداشت_با_POS", "خريداينترنتي"), or null. Never the user's note.
  - confidence: 0..1, how sure you are about amount and direction.`

// responseFormat is the strict json_schema response format.
var responseFormat = json.RawMessage(`{"type":"json_schema","json_schema":{
  "name": "input_extraction", "strict": true,
  "schema": {
    "type": "object", "additionalProperties": false,
    "required": ["is_transaction", "note", "transactions"],
    "properties": {
      "is_transaction": {"type": "boolean"},
      "note": {"type": ["string", "null"]},
      "transactions": {"type": "array", "items": {
        "type": "object", "additionalProperties": false,
        "required": ["amount", "amount_unit", "direction", "date", "time", "bank_label", "confidence"],
        "properties": {
          "amount": {"type": ["integer", "null"]},
          "amount_unit": {"type": "string", "enum": ["rial", "toman"]},
          "direction": {"type": "string", "enum": ["out", "in", "ambiguous"]},
          "date": {"type": ["string", "null"]},
          "time": {"type": ["string", "null"]},
          "bank_label": {"type": ["string", "null"]},
          "confidence": {"type": "number"}
        }}}
    }}}}`)
