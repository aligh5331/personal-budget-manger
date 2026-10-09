// Package bale is a small net/http client for the Bale Bot API.
//
// Everything else in the bot talks to Bale through the Client interface, so
// the bot core can be tested with the recording fake in package balefake.
package bale

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client is the Bale Bot API surface the bot uses.
type Client interface {
	GetUpdates(ctx context.Context, p GetUpdatesParams) ([]Update, error)
	SetWebhook(ctx context.Context, url string) error
	DeleteWebhook(ctx context.Context) error
	GetWebhookInfo(ctx context.Context) (WebhookInfo, error)
	GetMe(ctx context.Context) (User, error)
	SendMessage(ctx context.Context, p SendMessageParams) (Message, error)
	EditMessageText(ctx context.Context, p EditMessageTextParams) error
	AnswerCallbackQuery(ctx context.Context, p AnswerCallbackQueryParams) error
	SendDocument(ctx context.Context, p SendDocumentParams) (Message, error)
}

// DefaultBaseURL is Bale's Bot API endpoint.
const DefaultBaseURL = "https://tapi.bale.ai"

// APIError is a Bale reply with ok=false, or a non-2xx status.
type APIError struct {
	Method      string
	Code        int
	Description string
	RetryAfter  time.Duration // set on 429 when Bale sends parameters.retry_after
}

func (e *APIError) Error() string {
	return fmt.Sprintf("bale %s: %d %s", e.Method, e.Code, e.Description)
}

// IsWebhookActive reports whether err says getUpdates is refused because a
// webhook is set. Bale does not document the exact reply, so this matches the
// Telegram shape (409 Conflict) and any description mentioning a webhook.
func IsWebhookActive(err error) bool {
	var ae *APIError
	if !errors.As(err, &ae) {
		return false
	}
	return ae.Code == http.StatusConflict || strings.Contains(strings.ToLower(ae.Description), "webhook")
}

// HTTPClient implements Client over net/http.
type HTTPClient struct {
	baseURL string
	token   string
	http    *http.Client
	// CallTimeout bounds every call except getUpdates, whose deadline is
	// its long-poll timeout plus PollSlack.
	CallTimeout time.Duration
	PollSlack   time.Duration
}

// NewHTTPClient returns a client for the given bot token. baseURL is usually
// DefaultBaseURL; tests pass an httptest server URL.
func NewHTTPClient(baseURL, token string) *HTTPClient {
	return &HTTPClient{
		baseURL:     strings.TrimRight(baseURL, "/"),
		token:       token,
		http:        &http.Client{},
		CallTimeout: 15 * time.Second,
		PollSlack:   15 * time.Second,
	}
}

type envelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
	Parameters  *struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

func (c *HTTPClient) url(method string) string {
	return c.baseURL + "/bot" + c.token + "/" + method
}

// call posts a JSON body and decodes result into out (if non-nil).
func (c *HTTPClient) call(ctx context.Context, method string, timeout time.Duration, body any, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("bale %s: encode: %w", method, err)
	}
	return c.do(ctx, method, timeout, "application/json", bytes.NewReader(buf), out)
}

func (c *HTTPClient) do(ctx context.Context, method string, timeout time.Duration, contentType string, body io.Reader, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(method), body)
	if err != nil {
		return fmt.Errorf("bale %s: %w", method, c.redact(err))
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("bale %s: %w", method, c.redact(err))
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("bale %s: read: %w", method, c.redact(err))
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		if resp.StatusCode/100 != 2 {
			return &APIError{Method: method, Code: resp.StatusCode, Description: http.StatusText(resp.StatusCode)}
		}
		return fmt.Errorf("bale %s: decode: %w", method, err)
	}
	if !env.OK || resp.StatusCode/100 != 2 {
		ae := &APIError{Method: method, Code: env.ErrorCode, Description: env.Description}
		if ae.Code == 0 {
			ae.Code = resp.StatusCode
		}
		if env.Parameters != nil && env.Parameters.RetryAfter > 0 {
			ae.RetryAfter = time.Duration(env.Parameters.RetryAfter) * time.Second
		}
		return ae
	}
	if out != nil {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("bale %s: decode result: %w", method, err)
		}
	}
	return nil
}

// redact keeps the bot token out of errors (net/url errors include the URL).
func (c *HTTPClient) redact(err error) error {
	if c.token == "" || !strings.Contains(err.Error(), c.token) {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), c.token, "<token>"))
}

// GetUpdates long-polls for updates.
func (c *HTTPClient) GetUpdates(ctx context.Context, p GetUpdatesParams) ([]Update, error) {
	var out []Update
	timeout := time.Duration(p.Timeout)*time.Second + c.PollSlack
	err := c.call(ctx, "getUpdates", timeout, p, &out)
	return out, err
}

// SetWebhook registers url for webhook delivery.
func (c *HTTPClient) SetWebhook(ctx context.Context, url string) error {
	return c.call(ctx, "setWebhook", c.CallTimeout, map[string]string{"url": url}, nil)
}

// DeleteWebhook removes the webhook so getUpdates works again.
func (c *HTTPClient) DeleteWebhook(ctx context.Context) error {
	return c.call(ctx, "deleteWebhook", c.CallTimeout, struct{}{}, nil)
}

// GetWebhookInfo returns the current webhook status.
func (c *HTTPClient) GetWebhookInfo(ctx context.Context) (WebhookInfo, error) {
	var out WebhookInfo
	err := c.call(ctx, "getWebhookInfo", c.CallTimeout, struct{}{}, &out)
	return out, err
}

// GetMe returns the bot's own account.
func (c *HTTPClient) GetMe(ctx context.Context) (User, error) {
	var out User
	err := c.call(ctx, "getMe", c.CallTimeout, struct{}{}, &out)
	return out, err
}

// SendMessage sends a text message.
func (c *HTTPClient) SendMessage(ctx context.Context, p SendMessageParams) (Message, error) {
	var out Message
	err := c.call(ctx, "sendMessage", c.CallTimeout, p, &out)
	return out, err
}

// EditMessageText replaces a message's text and buttons.
func (c *HTTPClient) EditMessageText(ctx context.Context, p EditMessageTextParams) error {
	return c.call(ctx, "editMessageText", c.CallTimeout, p, nil)
}

// AnswerCallbackQuery stops the button's loading state.
func (c *HTTPClient) AnswerCallbackQuery(ctx context.Context, p AnswerCallbackQueryParams) error {
	return c.call(ctx, "answerCallbackQuery", c.CallTimeout, p, nil)
}

// SendDocument uploads a file to a chat.
func (c *HTTPClient) SendDocument(ctx context.Context, p SendDocumentParams) (Message, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("chat_id", strconv.FormatInt(p.ChatID, 10))
	if p.Caption != "" {
		_ = w.WriteField("caption", p.Caption)
	}
	part, err := w.CreateFormFile("document", p.FileName)
	if err != nil {
		return Message{}, fmt.Errorf("bale sendDocument: %w", err)
	}
	if _, err := part.Write(p.Content); err != nil {
		return Message{}, fmt.Errorf("bale sendDocument: %w", err)
	}
	if err := w.Close(); err != nil {
		return Message{}, fmt.Errorf("bale sendDocument: %w", err)
	}
	var out Message
	err = c.do(ctx, "sendDocument", 2*time.Minute, w.FormDataContentType(), &body, &out)
	return out, err
}
