// Package balefake is a recording, in-memory bale.Client for tests.
//
// It records every call, hands out increasing message ids for sends, and can
// be told to fail any method on demand.
package balefake

import (
	"context"
	"sync"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
)

// Method names as recorded in Call.Method.
const (
	MethodGetUpdates          = "getUpdates"
	MethodSetWebhook          = "setWebhook"
	MethodDeleteWebhook       = "deleteWebhook"
	MethodGetWebhookInfo      = "getWebhookInfo"
	MethodGetMe               = "getMe"
	MethodSendMessage         = "sendMessage"
	MethodEditMessageText     = "editMessageText"
	MethodAnswerCallbackQuery = "answerCallbackQuery"
	MethodSendDocument        = "sendDocument"
)

// Call is one recorded call. Params holds the typed params value
// (bale.SendMessageParams, bale.EditMessageTextParams, ...), or the webhook
// URL string for setWebhook, or nil.
type Call struct {
	Method string
	Params any
}

// Fake implements bale.Client. The zero value is not usable; call New.
type Fake struct {
	mu          sync.Mutex
	calls       []Call
	fail        map[string]error
	nextMsgID   int64
	webhookInfo bale.WebhookInfo
	updates     [][]bale.Update
}

var _ bale.Client = (*Fake)(nil)

// New returns an empty fake. Sent messages get ids from 1000 upwards so they
// never collide with the small ids tests use for incoming messages.
func New() *Fake {
	return &Fake{fail: map[string]error{}, nextMsgID: 1000}
}

// Fail makes every later call of method return err. Pass nil to stop failing.
func (f *Fake) Fail(method string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		delete(f.fail, method)
		return
	}
	f.fail[method] = err
}

// QueueUpdates makes the next GetUpdates call return us.
func (f *Fake) QueueUpdates(us ...bale.Update) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, us)
}

// SetWebhookInfo sets what GetWebhookInfo returns (SetWebhook also updates the URL).
func (f *Fake) SetWebhookInfo(info bale.WebhookInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.webhookInfo = info
}

// Calls returns a copy of every recorded call, in order.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// Reset forgets recorded calls (failures and queued updates are kept).
func (f *Fake) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

// Sent returns the params of every successful sendMessage call, in order.
func (f *Fake) Sent() []bale.SendMessageParams {
	return collect[bale.SendMessageParams](f, MethodSendMessage)
}

// Edits returns the params of every successful editMessageText call, in order.
func (f *Fake) Edits() []bale.EditMessageTextParams {
	return collect[bale.EditMessageTextParams](f, MethodEditMessageText)
}

// Answers returns the params of every successful answerCallbackQuery call.
func (f *Fake) Answers() []bale.AnswerCallbackQueryParams {
	return collect[bale.AnswerCallbackQueryParams](f, MethodAnswerCallbackQuery)
}

// Documents returns the params of every successful sendDocument call.
func (f *Fake) Documents() []bale.SendDocumentParams {
	return collect[bale.SendDocumentParams](f, MethodSendDocument)
}

func collect[T any](f *Fake, method string) []T {
	var out []T
	for _, c := range f.Calls() {
		if c.Method == method {
			if p, ok := c.Params.(T); ok {
				out = append(out, p)
			}
		}
	}
	return out
}

// record stores a successful call or returns the configured failure. Failed
// calls are not recorded, so Sent() is "what the Owner would see".
func (f *Fake) record(method string, params any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail[method]; err != nil {
		return err
	}
	f.calls = append(f.calls, Call{Method: method, Params: params})
	return nil
}

func (f *Fake) newMessageID() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextMsgID++
	return f.nextMsgID
}

// GetUpdates returns the next queued batch, or nothing.
func (f *Fake) GetUpdates(_ context.Context, p bale.GetUpdatesParams) ([]bale.Update, error) {
	if err := f.record(MethodGetUpdates, p); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.updates) == 0 {
		return nil, nil
	}
	us := f.updates[0]
	f.updates = f.updates[1:]
	return us, nil
}

// SetWebhook records the URL and makes GetWebhookInfo report it.
func (f *Fake) SetWebhook(_ context.Context, url string) error {
	if err := f.record(MethodSetWebhook, url); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.webhookInfo.URL = url
	return nil
}

// DeleteWebhook clears the webhook URL.
func (f *Fake) DeleteWebhook(_ context.Context) error {
	if err := f.record(MethodDeleteWebhook, nil); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.webhookInfo.URL = ""
	return nil
}

// GetWebhookInfo returns the stored webhook info.
func (f *Fake) GetWebhookInfo(_ context.Context) (bale.WebhookInfo, error) {
	if err := f.record(MethodGetWebhookInfo, nil); err != nil {
		return bale.WebhookInfo{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.webhookInfo, nil
}

// GetMe returns a fixed bot user.
func (f *Fake) GetMe(_ context.Context) (bale.User, error) {
	if err := f.record(MethodGetMe, nil); err != nil {
		return bale.User{}, err
	}
	return bale.User{ID: 1, IsBot: true, Username: "personal_bm_bot"}, nil
}

// SendMessage records the message and returns it with a fresh message id.
func (f *Fake) SendMessage(_ context.Context, p bale.SendMessageParams) (bale.Message, error) {
	if err := f.record(MethodSendMessage, p); err != nil {
		return bale.Message{}, err
	}
	return bale.Message{
		MessageID:   f.newMessageID(),
		Chat:        bale.Chat{ID: p.ChatID, Type: bale.ChatPrivate},
		Text:        p.Text,
		ReplyMarkup: p.ReplyMarkup,
	}, nil
}

// EditMessageText records the edit.
func (f *Fake) EditMessageText(_ context.Context, p bale.EditMessageTextParams) error {
	return f.record(MethodEditMessageText, p)
}

// AnswerCallbackQuery records the answer.
func (f *Fake) AnswerCallbackQuery(_ context.Context, p bale.AnswerCallbackQueryParams) error {
	return f.record(MethodAnswerCallbackQuery, p)
}

// SendDocument records the upload and returns a message with a fresh id.
func (f *Fake) SendDocument(_ context.Context, p bale.SendDocumentParams) (bale.Message, error) {
	if err := f.record(MethodSendDocument, p); err != nil {
		return bale.Message{}, err
	}
	return bale.Message{MessageID: f.newMessageID(), Chat: bale.Chat{ID: p.ChatID, Type: bale.ChatPrivate}}, nil
}
