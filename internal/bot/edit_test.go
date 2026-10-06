package bot_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/extract"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// markupData returns the callback_data of the button labelled label.
func markupData(t *testing.T, m *bale.InlineKeyboardMarkup, label string) string {
	t.Helper()
	if m == nil {
		t.Fatalf("no buttons, want [%s]", label)
	}
	for _, row := range m.InlineKeyboard {
		for _, b := range row {
			if b.Text == label {
				if len(b.CallbackData) > 64 {
					t.Errorf("callback_data %q is over 64 bytes", b.CallbackData)
				}
				return b.CallbackData
			}
		}
	}
	t.Fatalf("buttons %+v have no [%s]", m.InlineKeyboard, label)
	return ""
}

func labels(m *bale.InlineKeyboardMarkup) []string {
	var out []string
	if m == nil {
		return out
	}
	for _, row := range m.InlineKeyboard {
		for _, b := range row {
			out = append(out, b.Text)
		}
	}
	return out
}

// lastEdit returns the last editMessageText call, failing if there is none.
func lastEdit(t *testing.T, h *bottest.Harness) bale.EditMessageTextParams {
	t.Helper()
	e := h.Bale.Edits()
	if len(e) == 0 {
		t.Fatal("bot edited nothing")
	}
	return e[len(e)-1]
}

// openEdit saves one Transaction and taps [Edit] on its confirmation. It
// returns the confirmation, with the sub-menu on it, and the id.
func openEdit(t *testing.T, h *bottest.Harness) (bale.Message, int64) {
	t.Helper()
	msg, id := saveOne(t, h)
	msg.ReplyMarkup = h.LastSent().ReplyMarkup
	h.Tap(msg, markupData(t, msg.ReplyMarkup, "Edit"))
	e := lastEdit(t, h)
	if e.MessageID != msg.MessageID || e.ChatID != msg.Chat.ID {
		t.Fatalf("edit went to message %d in chat %d, want the confirmation %d", e.MessageID, e.ChatID, msg.MessageID)
	}
	msg.Text, msg.ReplyMarkup = e.Text, e.ReplyMarkup
	return msg, id
}

func TestEditOpensASubMenu(t *testing.T) {
	h := bottest.New(t)
	msg, _ := openEdit(t, h)
	conf := h.Sent()[0]

	got := labels(msg.ReplyMarkup)
	want := []string{"Amount", "Direction", "Date", "Description", "Back"}
	if len(got) != len(want) {
		t.Fatalf("sub-menu = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sub-menu = %v, want %v", got, want)
		}
	}
	if msg.Text != conf.Text {
		t.Errorf("text changed to %q, want the confirmation text kept", msg.Text)
	}
}

// tapEdit taps the sub-menu button label on the confirmation.
func tapEdit(t *testing.T, h *bottest.Harness, msg bale.Message, label string) {
	t.Helper()
	h.Tap(msg, markupData(t, msg.ReplyMarkup, label))
}

// reply sends text as a reply to the bot's latest message (the fake hands
// out sent-message ids from 1001 upwards).
func reply(h *bottest.Harness, text string) {
	m := h.OwnerMessage(text)
	m.ReplyToMessage = &bale.Message{MessageID: 1000 + int64(len(h.Sent())), Chat: m.Chat}
	h.SendMessage(m)
}

// confirmationOf wraps the last sent message as the message the Owner taps.
func confirmationOf(conf bale.SendMessageParams, id int64) bale.Message {
	return bale.Message{MessageID: 5000 + id, Chat: bale.Chat{ID: conf.ChatID, Type: bale.ChatPrivate}, Text: conf.Text, ReplyMarkup: conf.ReplyMarkup}
}

func TestEditAmount(t *testing.T) {
	h := bottest.New(t)
	msg, id := openEdit(t, h)

	tapEdit(t, h, msg, "Amount")
	assertContains(t, h.LastSent().Text, "New amount")
	reply(h, "۴۵۰ هزار")

	tx := storedTransaction(t, h, id)
	if tx.AmountToman == nil || *tx.AmountToman != 450000 {
		t.Fatalf("amount = %v, want 450000", tx.AmountToman)
	}
	e := lastEdit(t, h)
	assertContains(t, e.Text, "450,000 toman")
	if got := labels(e.ReplyMarkup); len(got) != 2 || got[0] != "Undo" || got[1] != "Edit" {
		t.Errorf("buttons after edit = %v, want the confirmation's Undo and Edit", got)
	}
	if e.MessageID != msg.MessageID {
		t.Errorf("edited message %d, want the confirmation %d", e.MessageID, msg.MessageID)
	}
}

func TestEditAmountInvalidKeepsTheOldValueAndTheQuestionOpen(t *testing.T) {
	h := bottest.New(t)
	msg, id := openEdit(t, h)
	tapEdit(t, h, msg, "Amount")
	promptID := 1000 + int64(len(h.Sent()))

	reply(h, "ناهار")

	assertContains(t, h.LastSent().Text, "Couldn't read an amount")
	if tx := storedTransaction(t, h, id); tx.AmountToman == nil || *tx.AmountToman != 124000 {
		t.Errorf("amount = %v, want unchanged 124000", tx.AmountToman)
	}
	// The Owner can answer the same question again.
	m := h.OwnerMessage("500000")
	m.ReplyToMessage = &bale.Message{MessageID: promptID, Chat: m.Chat}
	h.SendMessage(m)
	if tx := storedTransaction(t, h, id); tx.AmountToman == nil || *tx.AmountToman != 500000 {
		t.Errorf("amount = %v, want 500000 after the second answer", tx.AmountToman)
	}
}

func TestEditAmountClearsTheMissingAmountFlag(t *testing.T) {
	h := bottest.New(t)
	r := melliReading("")
	r.Transactions[0].Amount = 0
	h.Extractor.Return(r)
	h.SendText(melliBale)
	conf := h.LastSent()
	id := savedID(t, conf)
	msg := confirmationOf(conf, id)
	h.Tap(msg, markupData(t, msg.ReplyMarkup, "Edit"))
	msg.ReplyMarkup = lastEdit(t, h).ReplyMarkup

	tapEdit(t, h, msg, "Amount")
	reply(h, "80000")

	tx := storedTransaction(t, h, id)
	if tx.Flagged || tx.FlagReason != "" || tx.AmountToman == nil || *tx.AmountToman != 80000 {
		t.Errorf("flagged %v reason %q amount %v", tx.Flagged, tx.FlagReason, tx.AmountToman)
	}
	if got := lastEdit(t, h).Text; strings.Contains(got, "flagged") || !strings.Contains(got, "Saved.") {
		t.Errorf("confirmation = %q, want it no longer flagged", got)
	}
}

func TestEditDirection(t *testing.T) {
	h := bottest.New(t)
	msg, id := openEdit(t, h)

	tapEdit(t, h, msg, "Direction")
	e := lastEdit(t, h)
	if got := labels(e.ReplyMarkup); len(got) < 3 || got[0] != "Expense" || got[1] != "Income" || got[2] != "Internal" {
		t.Fatalf("direction buttons = %v", got)
	}
	msg.ReplyMarkup = e.ReplyMarkup
	tapEdit(t, h, msg, "Income")

	if tx := storedTransaction(t, h, id); tx.Direction != storage.DirectionIn || tx.CategoryID != nil {
		t.Errorf("direction %q category %v, want in with Uncategorized", tx.Direction, tx.CategoryID)
	}
	assertContains(t, lastEdit(t, h).Text, "Income", "Uncategorized")
}

func TestEditDirectionToInternalClearsTheCategory(t *testing.T) {
	h := bottest.New(t)
	msg, id := openEdit(t, h)
	tapEdit(t, h, msg, "Direction")
	msg.ReplyMarkup = lastEdit(t, h).ReplyMarkup

	tapEdit(t, h, msg, "Internal")

	if tx := storedTransaction(t, h, id); tx.Direction != storage.DirectionInternal || tx.CategoryID != nil {
		t.Errorf("direction %q category %v", tx.Direction, tx.CategoryID)
	}
	assertContains(t, lastEdit(t, h).Text, "Internal transfer", "none")
}

func TestEditDirectionSettlesAnAmbiguousOne(t *testing.T) {
	h := bottest.New(t)
	r := melliReading("")
	r.Transactions[0].Direction = extract.DirAmbiguous
	h.Extractor.Return(r)
	// A bare «انتقال» with no sign leaves the Direction ambiguous.
	h.SendText("بانک ملی\nانتقال\nمبلغ: ۱,۲۴۰,۰۰۰ ریال\n۱۲:۰۵ ۱۴۰۵/۰۷/۰۴")
	conf := h.LastSent()
	id := savedID(t, conf)
	if tx := storedTransaction(t, h, id); !tx.Flagged || tx.FlagReason != storage.FlagDirectionAmbiguous {
		t.Fatalf("fixture is not ambiguous: %+v", tx)
	}
	msg := confirmationOf(conf, id)
	h.Tap(msg, markupData(t, msg.ReplyMarkup, "Edit"))
	msg.ReplyMarkup = lastEdit(t, h).ReplyMarkup
	tapEdit(t, h, msg, "Direction")
	msg.ReplyMarkup = lastEdit(t, h).ReplyMarkup
	tapEdit(t, h, msg, "Expense")

	if tx := storedTransaction(t, h, id); tx.Flagged || tx.Direction != storage.DirectionOut {
		t.Errorf("flagged %v direction %q", tx.Flagged, tx.Direction)
	}
}

func TestEditDate(t *testing.T) {
	h := bottest.New(t)
	msg, id := openEdit(t, h)

	tapEdit(t, h, msg, "Date")
	reply(h, "۱۴۰۵/۰۶/۲۰")

	got := storedTransaction(t, h, id).OccurredAt.In(clock.Tehran())
	want := time.Date(2026, 9, 11, 12, 5, 0, 0, clock.Tehran()) // 20 Shahrivar 1405, time of day kept
	if !got.Equal(want) {
		t.Errorf("occurred_at = %s, want %s", got, want)
	}
	assertContains(t, lastEdit(t, h).Text, "20 Shahrivar 1405")
}

func TestEditDateInvalid(t *testing.T) {
	for _, bad := range []string{"hello", "1405/13/40", "1406/01/01"} {
		t.Run(bad, func(t *testing.T) {
			h := bottest.New(t)
			msg, id := openEdit(t, h)
			before := storedTransaction(t, h, id).OccurredAt
			tapEdit(t, h, msg, "Date")

			reply(h, bad)

			assertContains(t, h.LastSent().Text, "Couldn't read a Jalali date")
			if got := storedTransaction(t, h, id).OccurredAt; !got.Equal(before) {
				t.Errorf("occurred_at changed to %s", got)
			}
		})
	}
}

func TestEditDescription(t *testing.T) {
	h := bottest.New(t)
	msg, id := openEdit(t, h)

	tapEdit(t, h, msg, "Description")
	reply(h, "شام  با دوستان")

	if tx := storedTransaction(t, h, id); tx.Description != "شام با دوستان" {
		t.Errorf("description = %q", tx.Description)
	}
	assertContains(t, lastEdit(t, h).Text, "شام با دوستان")
}

func TestEditDescriptionDashClearsAndTooLongIsRefused(t *testing.T) {
	h := bottest.New(t)
	msg, id := openEdit(t, h)
	tapEdit(t, h, msg, "Description")
	reply(h, strings.Repeat("ا", 500))
	assertContains(t, h.LastSent().Text, "too long")
	if tx := storedTransaction(t, h, id); tx.Description != "تنقلات" {
		t.Errorf("description = %q, want unchanged", tx.Description)
	}

	tapEdit(t, h, msg, "Description")
	reply(h, "-")
	if tx := storedTransaction(t, h, id); tx.Description != "" {
		t.Errorf("description = %q, want cleared", tx.Description)
	}
}

func TestEditBackRestoresTheConfirmationButtons(t *testing.T) {
	h := bottest.New(t)
	msg, _ := openEdit(t, h)

	tapEdit(t, h, msg, "Back")

	if got := labels(lastEdit(t, h).ReplyMarkup); len(got) != 2 || got[0] != "Undo" || got[1] != "Edit" {
		t.Errorf("buttons = %v", got)
	}
}

func TestEditOnADeletedTransaction(t *testing.T) {
	h := bottest.New(t)
	msg, id := openEdit(t, h)
	h.Tap(msg, fmt.Sprintf("t:%d:undo", id))
	edits := len(h.Bale.Edits())

	tapEdit(t, h, msg, "Amount")

	a := h.Bale.Answers()
	if got := a[len(a)-1].Text; !strings.Contains(got, "removed") {
		t.Errorf("toast = %q", got)
	}
	if len(h.Bale.Edits()) != edits {
		t.Error("the message was edited")
	}
}

func TestAReplyToSomethingElseIsAnInput(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(melliReading(""))
	m := h.OwnerMessage(melliBale)
	m.ReplyToMessage = &bale.Message{MessageID: 4, Chat: m.Chat}

	h.SendMessage(m)

	assertContains(t, h.LastSent().Text, "Saved.")
}

func TestEditPromptSurvivesARestart(t *testing.T) {
	h := bottest.New(t)
	msg, id := openEdit(t, h)
	tapEdit(t, h, msg, "Amount")
	h.Restart()

	reply(h, "90000")

	if tx := storedTransaction(t, h, id); tx.AmountToman == nil || *tx.AmountToman != 90000 {
		t.Errorf("amount = %v", tx.AmountToman)
	}
}
