package bot_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bale/balefake"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
)

const webhookEndpoint = bottest.WebhookURL + "/bale/" + bottest.WebhookSecret

// modeMessage is the /mode message as the Owner sees it, after any edits.
type modeMessage struct {
	msg    bale.Message
	text   string
	button bale.InlineKeyboardButton
}

func sendMode(t *testing.T, h *bottest.Harness) modeMessage {
	t.Helper()
	h.SendText("/mode")
	sent := h.LastSent()
	return modeMessage{
		msg:    bale.Message{MessageID: 5001, Chat: bale.Chat{ID: bottest.OwnerID, Type: bale.ChatPrivate}, Text: sent.Text},
		text:   sent.Text,
		button: onlyButton(t, sent.ReplyMarkup),
	}
}

// tapSwitch taps the switch button and returns the message as edited.
func tapSwitch(t *testing.T, h *bottest.Harness, m modeMessage) modeMessage {
	t.Helper()
	h.Tap(m.msg, m.button.CallbackData)
	edits := h.Bale.Edits()
	if len(edits) == 0 {
		t.Fatal("the /mode message was not edited")
	}
	e := edits[len(edits)-1]
	if e.MessageID != m.msg.MessageID || e.ChatID != bottest.OwnerID {
		t.Fatalf("edited message %d in chat %d, want the /mode message", e.MessageID, e.ChatID)
	}
	return modeMessage{msg: m.msg, text: e.Text, button: onlyButton(t, e.ReplyMarkup)}
}

func onlyButton(t *testing.T, mk *bale.InlineKeyboardMarkup) bale.InlineKeyboardButton {
	t.Helper()
	if mk == nil || len(mk.InlineKeyboard) != 1 || len(mk.InlineKeyboard[0]) != 1 {
		t.Fatalf("want exactly one button, got %+v", mk)
	}
	return mk.InlineKeyboard[0][0]
}

func wantMode(t *testing.T, m modeMessage, mode, button string) {
	t.Helper()
	if !strings.Contains(m.text, mode) {
		t.Errorf("message does not say %s:\n%s", mode, m.text)
	}
	if m.button.Text != button {
		t.Errorf("button %q, want %q", m.button.Text, button)
	}
}

// methods returns the Bale methods called so far, in order, without the
// getUpdates long polls (which run continuously while polling).
func methods(h *bottest.Harness) []string {
	var out []string
	for _, c := range h.Bale.Calls() {
		if c.Method == balefake.MethodGetUpdates {
			if p := c.Params.(bale.GetUpdatesParams); p.Timeout > 0 {
				continue
			}
			out = append(out, "commit")
			continue
		}
		if c.Method == balefake.MethodSetWebhook {
			out = append(out, "setWebhook "+c.Params.(string))
			continue
		}
		out = append(out, c.Method)
	}
	return out
}

func countLongPolls(h *bottest.Harness) int {
	n := 0
	for _, c := range h.Bale.Calls() {
		if c.Method == balefake.MethodGetUpdates && c.Params.(bale.GetUpdatesParams).Timeout > 0 {
			n++
		}
	}
	return n
}

// wantPolling checks that a message queued at Bale is fetched and answered.
func wantPolling(t *testing.T, h *bottest.Harness) {
	t.Helper()
	before := len(h.Sent())
	h.Bale.QueueUpdates(bale.Update{UpdateID: h.NextUpdateID(), Message: h.OwnerMessage("/help")})
	deadline := time.Now().Add(5 * time.Second)
	for len(h.Sent()) == before {
		if time.Now().After(deadline) {
			t.Fatal("a message sent while polling was never answered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.WaitIdle()
}

// wantNotPolling checks that no getUpdates long poll runs.
func wantNotPolling(t *testing.T, h *bottest.Harness) {
	t.Helper()
	n := countLongPolls(h)
	time.Sleep(3 * balefake.LongPoll)
	if m := countLongPolls(h); m != n {
		t.Fatalf("still polling: %d more getUpdates calls", m-n)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestModeShowsPollingWithSwitchButton(t *testing.T) {
	h := bottest.New(t)
	h.StartUpdates()

	m := sendMode(t, h)
	wantMode(t, m, "polling", "Switch to webhook")
	if m.button.CallbackData != "mode:webhook" {
		t.Errorf("callback data %q", m.button.CallbackData)
	}
	if !contains(methods(h), balefake.MethodDeleteWebhook) {
		t.Errorf("polling at boot was not asserted with deleteWebhook: %v", methods(h))
	}
}

func TestSwitchToWebhook(t *testing.T) {
	h := bottest.New(t)
	h.StartUpdates()
	m := sendMode(t, h)
	h.Bale.Reset()

	m = tapSwitch(t, h, m)

	wantMode(t, m, "webhook", "Switch to polling")
	want := []string{"commit", "setWebhook " + webhookEndpoint, balefake.MethodGetWebhookInfo}
	got := methods(h)
	if len(got) < len(want) || strings.Join(got[:len(want)], ",") != strings.Join(want, ",") {
		t.Fatalf("calls %v, want %v first", got, want)
	}
	wantNotPolling(t, h)
	if a := h.Bale.Answers(); len(a) == 0 || a[len(a)-1].Text == "" {
		t.Errorf("tap not answered with a toast: %+v", a)
	}

	// The choice survives a restart and is asserted against Bale.
	h.Bale.Reset()
	h.Restart()
	if !contains(methods(h), "setWebhook "+webhookEndpoint) {
		t.Fatalf("webhook not set again at boot: %v", methods(h))
	}
	wantNotPolling(t, h)
	wantMode(t, sendMode(t, h), "webhook", "Switch to polling")
}

func TestNoUpdateIsDroppedWhileSwitchingToWebhook(t *testing.T) {
	h := bottest.New(t)
	h.StartUpdates()
	m := sendMode(t, h)
	before := len(h.Sent())

	// The tap arrives by polling together with a message; another message
	// is waiting at Bale when polling stops.
	tap := bale.Update{UpdateID: h.NextUpdateID(), CallbackQuery: &bale.CallbackQuery{
		ID: "cb-switch", From: bale.User{ID: bottest.OwnerID}, Message: &m.msg, Data: m.button.CallbackData,
	}}
	first := bale.Update{UpdateID: h.NextUpdateID(), Message: h.OwnerMessage("/help")}
	second := bale.Update{UpdateID: h.NextUpdateID(), Message: h.OwnerMessage("/help")}
	h.Bale.QueueUpdates(tap, first)
	h.Bale.QueueUpdates(second)

	deadline := time.Now().Add(5 * time.Second)
	for len(h.Sent()) < before+2 {
		if time.Now().After(deadline) {
			t.Fatalf("answered %d of 2 messages around the switch", len(h.Sent())-before)
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.WaitIdle()
	if n := len(h.Sent()) - before; n != 2 {
		t.Fatalf("sent %d messages, want 2 help replies", n)
	}
	edits := h.Bale.Edits()
	if len(edits) != 1 || !strings.Contains(edits[0].Text, "webhook") {
		t.Fatalf("edits %+v, want the switch to webhook", edits)
	}
	wantNotPolling(t, h)
}

func TestSwitchToWebhookRefusesShortSecret(t *testing.T) {
	h := bottest.New(t)
	h.UpdatesConfig.SecretPath = "too-short-secret"
	h.StartUpdates()
	m := sendMode(t, h)

	m = tapSwitch(t, h, m)

	wantMode(t, m, "polling", "Switch to webhook")
	if !strings.Contains(m.text, "32") {
		t.Errorf("error does not explain the 32-character minimum:\n%s", m.text)
	}
	for _, c := range methods(h) {
		if strings.HasPrefix(c, "setWebhook") {
			t.Fatalf("setWebhook called with a short secret: %v", methods(h))
		}
	}
	wantPolling(t, h)
}

func TestSwitchToWebhookRefusesMissingURL(t *testing.T) {
	h := bottest.New(t)
	h.UpdatesConfig.WebhookURL = ""
	h.StartUpdates()

	m := tapSwitch(t, h, sendMode(t, h))

	wantMode(t, m, "polling", "Switch to webhook")
	if !strings.Contains(m.text, "WEBHOOK_URL") {
		t.Errorf("error does not name WEBHOOK_URL:\n%s", m.text)
	}
	wantPolling(t, h)
}

func TestSwitchToWebhookRollsBackWhenSetWebhookFails(t *testing.T) {
	h := bottest.New(t)
	h.StartUpdates()
	m := sendMode(t, h)
	h.Bale.Fail(balefake.MethodSetWebhook, errors.New("bale setWebhook: 400 Bad Request: bad webhook"))

	m = tapSwitch(t, h, m)

	wantMode(t, m, "polling", "Switch to webhook")
	if !strings.Contains(m.text, "bad webhook") {
		t.Errorf("error not shown:\n%s", m.text)
	}
	wantPolling(t, h)

	// Not persisted: a restart stays on polling.
	h.Bale.Fail(balefake.MethodSetWebhook, nil)
	h.Bale.Reset()
	h.Restart()
	if contains(methods(h), "setWebhook "+webhookEndpoint) {
		t.Fatalf("failed switch was persisted: %v", methods(h))
	}
	wantMode(t, sendMode(t, h), "polling", "Switch to webhook")
}

func TestSwitchToWebhookRollsBackWhenVerificationFails(t *testing.T) {
	h := bottest.New(t)
	h.StartUpdates()
	m := sendMode(t, h)
	h.Bale.Fail(balefake.MethodGetWebhookInfo, errors.New("bale getWebhookInfo: 502 Bad Gateway"))
	h.Bale.Reset()

	m = tapSwitch(t, h, m)

	wantMode(t, m, "polling", "Switch to webhook")
	if !strings.Contains(m.text, "502") {
		t.Errorf("error not shown:\n%s", m.text)
	}
	got := methods(h)
	if !contains(got, "setWebhook "+webhookEndpoint) || !contains(got, balefake.MethodDeleteWebhook) {
		t.Fatalf("calls %v, want setWebhook undone with deleteWebhook", got)
	}
	wantPolling(t, h)
}

func TestSwitchToPolling(t *testing.T) {
	h := bottest.New(t)
	h.UpdatesConfig.ModeDefault = "webhook"
	h.StartUpdates()
	m := sendMode(t, h)
	wantMode(t, m, "webhook", "Switch to polling")
	if m.button.CallbackData != "mode:polling" {
		t.Errorf("callback data %q", m.button.CallbackData)
	}
	h.Bale.Reset()

	m = tapSwitch(t, h, m)

	wantMode(t, m, "polling", "Switch to webhook")
	if !contains(methods(h), balefake.MethodDeleteWebhook) {
		t.Fatalf("deleteWebhook not called: %v", methods(h))
	}
	wantPolling(t, h)

	// The saved choice wins over MODE_DEFAULT after a restart.
	h.Bale.Reset()
	h.Restart()
	if contains(methods(h), "setWebhook "+webhookEndpoint) {
		t.Fatalf("webhook set at boot despite the saved polling choice: %v", methods(h))
	}
	wantPolling(t, h)
}

func TestSwitchToPollingRollsBackWhenDeleteWebhookFails(t *testing.T) {
	h := bottest.New(t)
	h.UpdatesConfig.ModeDefault = "webhook"
	h.StartUpdates()
	m := sendMode(t, h)
	h.Bale.Fail(balefake.MethodDeleteWebhook, errors.New("bale deleteWebhook: 500 Internal Server Error"))

	m = tapSwitch(t, h, m)

	wantMode(t, m, "webhook", "Switch to polling")
	if !strings.Contains(m.text, "500") {
		t.Errorf("error not shown:\n%s", m.text)
	}
	wantNotPolling(t, h)
}

func TestBootFallsBackToPollingWhenSetWebhookFails(t *testing.T) {
	h := bottest.New(t)
	h.StartUpdates()
	tapSwitch(t, h, sendMode(t, h)) // saved choice: webhook
	h.Bale.Fail(balefake.MethodSetWebhook, errors.New("bale setWebhook: 400 Bad Request: no route"))
	before := len(h.Sent())

	h.Restart()

	sent := h.Sent()[before:]
	if len(sent) != 1 || sent[0].ChatID != bottest.OwnerID {
		t.Fatalf("Owner got %d messages at boot, want 1", len(sent))
	}
	for _, want := range []string{"no route", "polling"} {
		if !strings.Contains(sent[0].Text, want) {
			t.Errorf("boot message lacks %q:\n%s", want, sent[0].Text)
		}
	}
	wantPolling(t, h)
	m := sendMode(t, h)
	wantMode(t, m, "polling", "Switch to webhook")

	// The fallback is for this run only: the next boot tries webhook again.
	h.Bale.Fail(balefake.MethodSetWebhook, nil)
	h.Bale.Reset()
	h.Restart()
	if !contains(methods(h), "setWebhook "+webhookEndpoint) {
		t.Fatalf("saved webhook choice lost after a fallback: %v", methods(h))
	}
}

func TestBootRefusesWebhookWithShortSecret(t *testing.T) {
	h := bottest.New(t)
	h.UpdatesConfig.ModeDefault = "webhook"
	h.UpdatesConfig.SecretPath = "short"
	h.StartUpdates()

	if contains(methods(h), "setWebhook "+bottest.WebhookURL+"/bale/short") {
		t.Fatal("setWebhook called with a short secret")
	}
	if !strings.Contains(h.LastSent().Text, "32") {
		t.Errorf("boot message does not explain the secret length:\n%s", h.LastSent().Text)
	}
	wantPolling(t, h)
}

func TestModeDefaultWebhookIsUsedWhenNothingSaved(t *testing.T) {
	h := bottest.New(t)
	h.UpdatesConfig.ModeDefault = "webhook"
	h.StartUpdates()

	if !contains(methods(h), "setWebhook "+webhookEndpoint) {
		t.Fatalf("calls %v, want setWebhook at boot", methods(h))
	}
	if len(h.Sent()) != 0 {
		t.Fatalf("unexpected messages at boot: %+v", h.Sent())
	}
	wantNotPolling(t, h)
}

func startWebhook(t *testing.T) *bottest.Harness {
	t.Helper()
	h := bottest.New(t)
	h.UpdatesConfig.ModeDefault = "webhook"
	h.StartUpdates()
	if err := h.Updates.Healthy(); err != nil {
		t.Fatalf("webhook unhealthy right after boot: %v", err)
	}
	return h
}

func TestWebhookCheckFailsOverOnWrongURL(t *testing.T) {
	h := startWebhook(t)
	h.Bale.SetWebhookInfo(bale.WebhookInfo{URL: "https://someone-else.example.com/hook"})
	h.Bale.Reset()

	h.Updates.CheckWebhook(context.Background())

	if !contains(methods(h), balefake.MethodDeleteWebhook) {
		t.Errorf("calls %v, want deleteWebhook before polling", methods(h))
	}
	wantPolling(t, h)
	notices := h.Sent()[:len(h.Sent())-1] // the last one answers wantPolling's /help
	if len(notices) != 1 || !strings.Contains(notices[0].Text, "polling") {
		t.Fatalf("Owner notices %+v, want one about polling", notices)
	}
	if strings.Contains(notices[0].Text, bottest.WebhookSecret) {
		t.Error("notice leaks the secret")
	}

	// Told once: later checks stay quiet.
	n := len(h.Sent())
	h.Clock.Advance(10 * time.Minute)
	h.Updates.CheckWebhook(context.Background())
	if len(h.Sent()) != n {
		t.Fatalf("Owner told again: %+v", h.Sent()[n:])
	}
	m := sendMode(t, h)
	wantMode(t, m, "polling", "Switch to webhook")
}

func TestWebhookCheckFailsOverOnRecentLastError(t *testing.T) {
	h := startWebhook(t)
	h.Bale.SetWebhookInfo(bale.WebhookInfo{
		URL:              webhookEndpoint,
		LastErrorDate:    h.Clock.Now().Add(-2 * time.Minute).Unix(),
		LastErrorMessage: "Connection refused",
	})

	h.Updates.CheckWebhook(context.Background())

	if len(h.Sent()) != 1 || !strings.Contains(h.Sent()[0].Text, "Connection refused") {
		t.Fatalf("Owner messages %+v, want one naming the error", h.Sent())
	}
	wantPolling(t, h)
}

func TestWebhookCheckIgnoresOldLastError(t *testing.T) {
	h := startWebhook(t)
	h.Bale.SetWebhookInfo(bale.WebhookInfo{
		URL:              webhookEndpoint,
		LastErrorDate:    h.Clock.Now().Add(-time.Hour).Unix(),
		LastErrorMessage: "Connection refused",
	})

	h.Updates.CheckWebhook(context.Background())

	if len(h.Sent()) != 0 {
		t.Fatalf("Owner told about an old error: %+v", h.Sent())
	}
	if err := h.Updates.Healthy(); err != nil {
		t.Fatalf("unhealthy: %v", err)
	}
	wantNotPolling(t, h)
}

func TestHealthReportsWebhookFromLastCheck(t *testing.T) {
	h := startWebhook(t)
	h.Bale.Fail(balefake.MethodGetWebhookInfo, errors.New("bale getWebhookInfo: 502 Bad Gateway"))

	h.Updates.CheckWebhook(context.Background())

	err := h.Updates.Healthy()
	if err == nil || !strings.Contains(err.Error(), "webhook") {
		t.Fatalf("Healthy() = %v, want a webhook failure", err)
	}
	if len(h.Sent()) != 0 {
		t.Fatalf("a failed check call is not a broken webhook; Owner told: %+v", h.Sent())
	}

	h.Bale.Fail(balefake.MethodGetWebhookInfo, nil)
	h.Updates.CheckWebhook(context.Background())
	if err := h.Updates.Healthy(); err != nil {
		t.Fatalf("still unhealthy after a good check: %v", err)
	}
}

func TestSecretIsNeverLogged(t *testing.T) {
	h := bottest.New(t)
	h.StartUpdates()
	h.Bale.Fail(balefake.MethodSetWebhook, errors.New("bale setWebhook: bad url "+webhookEndpoint))
	m := tapSwitch(t, h, sendMode(t, h))

	if strings.Contains(m.text, bottest.WebhookSecret) {
		t.Errorf("error shown to the Owner leaks the secret:\n%s", m.text)
	}
	if h.Logs.Mentions(bottest.WebhookSecret) {
		t.Error("the secret was logged")
	}
}
