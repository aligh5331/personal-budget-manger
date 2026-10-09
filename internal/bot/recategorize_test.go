package bot_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bale/balefake"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
)

// pendingTransaction saves a Transaction while Jev is down and returns its
// id and the confirmation. The Categorizer fails on the first call; later
// calls get the answers queued by then (none: it keeps failing).
func pendingTransaction(t *testing.T, h *bottest.Harness, then ...func()) (int64, bale.SendMessageParams) {
	t.Helper()
	h.Extractor.Return(melliReading("تنقلات"))
	h.Categorizer.Fail(errors.New("503 no healthy upstream"))
	for _, f := range then {
		f()
	}
	h.SendText(melliBale + "\nتنقلات")
	conf := h.LastSent()
	return savedID(t, conf), conf
}

func recategorize(t *testing.T, h *bottest.Harness, after time.Duration) {
	t.Helper()
	h.Clock.Advance(after)
	if err := h.Bot.RecategorizeDue(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAnOutageIsHealedInTheBackgroundAndTheConfirmationEdited(t *testing.T) {
	h := bottest.New(t)
	id, conf := pendingTransaction(t, h, func() { h.Categorizer.Choose("Snacks", 0.9) })

	recategorize(t, h, 9*time.Minute)
	if n := len(h.Categorizer.Calls()); n != 1 {
		t.Fatalf("categorizer called %d times before 10 minutes passed, want 1", n)
	}
	recategorize(t, h, time.Minute)

	tx := storedTransaction(t, h, id)
	assertCategory(t, h, tx, "Snacks")
	if tx.CategorizePending {
		t.Error("categorize_pending still set after success")
	}
	edits := h.Bale.Edits()
	if len(edits) != 1 {
		t.Fatalf("edits = %+v, want one", edits)
	}
	assertContains(t, edits[0].Text, "Category: Snacks", "124,000 toman")
	if edits[0].ChatID != conf.ChatID || edits[0].MessageID == 0 {
		t.Errorf("edit targets chat %d message %d", edits[0].ChatID, edits[0].MessageID)
	}
	if len(buttonLabels(edits[0].ReplyMarkup)) == 0 {
		t.Error("edited confirmation lost its buttons")
	}

	recategorize(t, h, time.Hour)
	if n := len(h.Categorizer.Calls()); n != 2 {
		t.Errorf("categorizer called %d times in total, want 2 (no retry after success)", n)
	}
}

func TestTheOwnersChoiceBeatsALateResult(t *testing.T) {
	h := bottest.New(t)
	id, conf := pendingTransaction(t, h, func() { h.Categorizer.Choose("Food", 0.95) })
	msg := bale.Message{MessageID: 77, Chat: bale.Chat{ID: conf.ChatID, Type: bale.ChatPrivate}, Text: conf.Text}
	h.Tap(msg, buttonData(t, conf, "Category"))
	h.Tap(msg, findButton(t, h.Bale.Edits()[0].ReplyMarkup, "Rent"))
	edits := len(h.Bale.Edits())

	recategorize(t, h, 10*time.Minute)

	assertCategory(t, h, storedTransaction(t, h, id), "Rent")
	if len(h.Bale.Edits()) != edits {
		t.Error("the confirmation was edited after the Owner's pick")
	}
	if n := len(h.Categorizer.Calls()); n != 1 {
		t.Errorf("categorizer called %d times, want 1 (the Owner's pick drops the retry)", n)
	}
}

func TestItGivesUpAfterSixTries(t *testing.T) {
	h := bottest.New(t)
	h.Categorizer.Repeat() // every retry fails the same way
	id, _ := pendingTransaction(t, h)

	for i := range 6 {
		recategorize(t, h, 10*time.Minute)
		if want := i + 2; len(h.Categorizer.Calls()) != want {
			t.Fatalf("after try %d: %d calls, want %d", i+1, len(h.Categorizer.Calls()), want)
		}
	}

	tx := storedTransaction(t, h, id)
	assertCategory(t, h, tx, "Uncategorized")
	if tx.CategorizePending {
		t.Error("mark not cleared after the 6th failed try")
	}
	recategorize(t, h, time.Hour)
	if n := len(h.Categorizer.Calls()); n != 7 {
		t.Errorf("categorizer called %d times in total, want 7", n)
	}
	if len(h.Bale.Edits()) != 0 {
		t.Error("the confirmation was edited though nothing was categorized")
	}
}

func TestALowConfidenceResultClearsTheMark(t *testing.T) {
	h := bottest.New(t)
	id, _ := pendingTransaction(t, h, func() { h.Categorizer.Choose("Food", 0.6) })

	recategorize(t, h, 10*time.Minute)

	tx := storedTransaction(t, h, id)
	assertCategory(t, h, tx, "Uncategorized")
	if tx.CategorizePending {
		t.Error("mark not cleared by a low-confidence answer")
	}
	recategorize(t, h, time.Hour)
	if n := len(h.Categorizer.Calls()); n != 2 {
		t.Errorf("categorizer called %d times, want 2", n)
	}
}

func TestThePendingMarkSurvivesARestart(t *testing.T) {
	h := bottest.New(t)
	id, _ := pendingTransaction(t, h, func() { h.Categorizer.Choose("Snacks", 0.9) })

	h.Restart()
	recategorize(t, h, 10*time.Minute)

	assertCategory(t, h, storedTransaction(t, h, id), "Snacks")
	if len(h.Bale.Edits()) != 1 {
		t.Errorf("edits = %d, want the original confirmation edited", len(h.Bale.Edits()))
	}
}

func TestAFailedEditStillUpdatesTheRow(t *testing.T) {
	h := bottest.New(t)
	id, _ := pendingTransaction(t, h, func() { h.Categorizer.Choose("Snacks", 0.9) })
	h.Bale.Fail(balefake.MethodEditMessageText, errors.New("message to edit not found"))

	recategorize(t, h, 10*time.Minute)

	tx := storedTransaction(t, h, id)
	assertCategory(t, h, tx, "Snacks")
	if tx.CategorizePending {
		t.Error("mark not cleared")
	}
}

func TestADeletedTransactionIsDropped(t *testing.T) {
	h := bottest.New(t)
	id, _ := pendingTransaction(t, h)
	if _, err := h.Store.DeleteTransaction(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	recategorize(t, h, 10*time.Minute)

	if n := len(h.Categorizer.Calls()); n != 1 {
		t.Errorf("categorizer called %d times for a deleted Transaction", n)
	}
}
