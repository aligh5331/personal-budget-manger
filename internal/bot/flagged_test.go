package bot_test

import (
	"strings"
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

func TestFlaggedListsNewestFirstFivePerPageWithRawTextAndReason(t *testing.T) {
	h := bottest.New(t)
	for day := 1; day <= 7; day++ {
		seedTx(t, h, seed{year: 1405, month: 7, day: day, amount: int64(day) * 1000, flagged: true})
	}
	seedTx(t, h, seed{year: 1405, month: 7, day: 9, amount: 999, desc: "fine"})

	h.SendText("/flagged")
	first := h.LastSent()
	assertContains(t, first.Text, "7 in all", "page 1 of 2", "raw text 7000", "raw text 3000", "no answer")
	if strings.Contains(first.Text, "raw text 2000") || strings.Contains(first.Text, "raw text 999") {
		t.Errorf("page 1 shows too much:\n%s", first.Text)
	}
	if strings.Index(first.Text, "raw text 7000") > strings.Index(first.Text, "raw text 6000") {
		t.Error("not newest first")
	}
	if hasLabel(first.ReplyMarkup, "Newer") || !hasLabel(first.ReplyMarkup, "Older") {
		t.Errorf("page 1 nav: %v", labels(first.ReplyMarkup))
	}

	h.Tap(questionMessage(h), buttonData(t, first, "Older"))
	edits := h.Bale.Edits()
	page2 := edits[len(edits)-1]
	assertContains(t, page2.Text, "page 2 of 2", "raw text 2000", "raw text 1000")
	if strings.Contains(page2.Text, "raw text 3000") || hasLabel(page2.ReplyMarkup, "Older") || !hasLabel(page2.ReplyMarkup, "Newer") {
		t.Errorf("page 2: %v\n%s", labels(page2.ReplyMarkup), page2.Text)
	}
}

func TestFlaggedWithNothingFlagged(t *testing.T) {
	h := bottest.New(t)
	h.SendText("/flagged")
	assertContains(t, h.LastSent().Text, "Nothing is flagged")
}

func TestFlaggedFixAsksForTheAmountAndTheAnswerClearsTheFlag(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(noAmountReading())
	h.SendText(melliBale)
	expireFollowUps(h)
	id := onlyTransaction(t, h).ID

	h.SendText("/flagged")
	list := h.LastSent()
	assertContains(t, list.Text, "amount unknown", "no answer")
	h.Tap(questionMessage(h), buttonData(t, list, "Fix 1"))
	assertContains(t, h.LastSent().Text, "How much, in toman?")

	replyToQuestion(h, "80000")

	tx := storedTransaction(t, h, id)
	if tx.Flagged || tx.FlagReason != "" || tx.AmountToman == nil || *tx.AmountToman != 80000 {
		t.Errorf("flagged %v reason %q amount %v", tx.Flagged, tx.FlagReason, tx.AmountToman)
	}
	assertContains(t, h.LastSent().Text, "80,000 toman")
	h.SendText("/flagged")
	assertContains(t, h.LastSent().Text, "Nothing is flagged")
}

func TestFlaggedFixAsksForTheDirectionByButtons(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(ambiguousReading())
	h.SendText(ambiguousTransfer)
	expireFollowUps(h)
	id := onlyTransaction(t, h).ID

	h.SendText("/flagged")
	list := h.LastSent()
	h.Tap(questionMessage(h), buttonData(t, list, "Fix 1"))
	question := h.LastSent()
	assertContains(t, question.Text, "out or came in")

	h.Tap(questionMessage(h), buttonData(t, question, "Income"))

	tx := storedTransaction(t, h, id)
	if tx.Flagged || tx.Direction != storage.DirectionIn {
		t.Errorf("flagged %v direction %q", tx.Flagged, tx.Direction)
	}
}

func TestFlaggedFixTwiceDoesNotAskAgain(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(noAmountReading())
	h.SendText(melliBale)
	expireFollowUps(h)
	h.SendText("/flagged")
	list := h.LastSent()
	msg := questionMessage(h)
	h.Tap(msg, buttonData(t, list, "Fix 1"))
	sent := len(h.Sent())
	h.Tap(msg, buttonData(t, list, "Fix 1"))
	if len(h.Sent()) != sent {
		t.Error("a second Fix asked again")
	}
}

func TestFlaggedDeleteRemovesTheTransactionAndRedrawsThePage(t *testing.T) {
	h := bottest.New(t)
	keep := seedTx(t, h, seed{year: 1405, month: 7, day: 1, amount: 1000, flagged: true})
	gone := seedTx(t, h, seed{year: 1405, month: 7, day: 2, amount: 2000, flagged: true})

	h.SendText("/flagged")
	list := h.LastSent()
	h.Tap(questionMessage(h), buttonData(t, list, "Delete 1")) // the newest

	if _, ok, _ := h.Store.TransactionByID(t.Context(), gone); ok {
		t.Error("Transaction still stored")
	}
	if _, ok, _ := h.Store.TransactionByID(t.Context(), keep); !ok {
		t.Error("the other Transaction was deleted")
	}
	edits := h.Bale.Edits()
	assertContains(t, edits[len(edits)-1].Text, "raw text 1000")
	if strings.Contains(edits[len(edits)-1].Text, "raw text 2000") {
		t.Error("deleted row still listed")
	}
}
