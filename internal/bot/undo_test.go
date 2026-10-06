package bot_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
)

// saveOne records melliBale and returns the confirmation as the Owner sees it.
func saveOne(t *testing.T, h *bottest.Harness) (bale.Message, int64) {
	t.Helper()
	h.Extractor.Return(melliReading("تنقلات"))
	h.SendText(melliBale + "\nتنقلات")
	conf := h.LastSent()
	id := savedID(t, conf)
	msg := bale.Message{MessageID: 5000 + id, Chat: bale.Chat{ID: conf.ChatID, Type: bale.ChatPrivate}, Text: conf.Text}
	return msg, id
}

func TestUndoDeletesTheTransaction(t *testing.T) {
	h := bottest.New(t)
	msg, id := saveOne(t, h)

	h.Tap(msg, fmt.Sprintf("t:%d:undo", id))

	if _, ok, _ := h.Store.TransactionByID(context.Background(), id); ok {
		t.Error("Transaction still stored after Undo")
	}
	edits := h.Bale.Edits()
	if len(edits) != 1 || edits[0].MessageID != msg.MessageID || edits[0].Text != "Removed" || edits[0].ReplyMarkup != nil {
		t.Fatalf("edits = %+v, want the confirmation edited to \"Removed\" with no buttons", edits)
	}
	if a := h.Bale.Answers(); len(a) != 1 || a[0].Text != "Removed" {
		t.Errorf("answers = %+v", a)
	}
}

func TestUndoWorksOnAnOldConfirmation(t *testing.T) {
	h := bottest.New(t)
	msg, id := saveOne(t, h)
	h.Clock.Advance(30 * 24 * time.Hour)
	h.Restart()

	h.Tap(msg, fmt.Sprintf("t:%d:undo", id))

	if _, ok, _ := h.Store.TransactionByID(context.Background(), id); ok {
		t.Error("Transaction still stored after Undo")
	}
	if e := h.Bale.Edits(); len(e) != 1 || e[0].Text != "Removed" {
		t.Errorf("edits = %+v", e)
	}
}

func TestUndoTwiceSaysAlreadyRemoved(t *testing.T) {
	h := bottest.New(t)
	msg, id := saveOne(t, h)
	// A different payment, so it is not held back as a duplicate.
	r := melliReading("")
	r.Transactions[0].Amount = 1250000
	h.Extractor.Reset()
	h.Extractor.Return(r)
	h.SendText(otherPayment())
	conf := h.LastSent()
	otherID := savedID(t, conf)
	other := bale.Message{MessageID: 5000 + otherID, Chat: bale.Chat{ID: conf.ChatID, Type: bale.ChatPrivate}, Text: conf.Text}

	h.Tap(msg, fmt.Sprintf("t:%d:undo", id))
	h.Tap(msg, fmt.Sprintf("t:%d:undo", id))

	a := h.Bale.Answers()
	if len(a) != 2 || a[1].Text != "Already removed" {
		t.Errorf("answers = %+v", a)
	}
	if _, ok, _ := h.Store.TransactionByID(context.Background(), otherID); !ok {
		t.Errorf("Undo on %d removed the other Transaction %d (message %d)", id, otherID, other.MessageID)
	}
}

func TestBadTransactionButtonIsAnswered(t *testing.T) {
	h := bottest.New(t)
	msg, _ := saveOne(t, h)

	h.Tap(msg, "t:notanumber:undo")
	h.Tap(msg, "t:1:nosuchaction")

	a := h.Bale.Answers()
	if len(a) != 2 || a[0].Text == "" || a[1].Text == "" {
		t.Errorf("answers = %+v, want a toast for each stale button", a)
	}
	if len(h.Bale.Edits()) != 0 {
		t.Errorf("edits = %+v, want none", h.Bale.Edits())
	}
}
