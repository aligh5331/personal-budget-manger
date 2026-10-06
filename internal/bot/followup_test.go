package bot_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
	"github.com/aligh5331/personal-budget-manger/internal/extract"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// expireFollowUps lets 31 minutes pass with nobody answering and runs the
// expiry sweep the app's timer runs.
func expireFollowUps(h *bottest.Harness) {
	h.Clock.Advance(31 * time.Minute)
	h.ExpireFollowUps()
}

// otherPayment is melliBale with another amount (1,250,000 rial), so it is
// not held back as a duplicate of it.
func otherPayment() string {
	lines := strings.Split(melliBale, "\n")
	lines[2] = "مبلغ: *1,250,000-* ریال"
	return strings.Join(lines, "\n")
}

// noAmountReading is melliBale with the amount unreadable.
func noAmountReading() extract.Result {
	r := melliReading("")
	r.Transactions[0].Amount = 0
	return r
}

// A bare «انتقال» (a transfer, no sign) leaves the Direction ambiguous.
const ambiguousTransfer = "بانک ملی\nانتقال\nمبلغ: ۱,۲۴۰,۰۰۰ ریال\n۱۲:۰۵ ۱۴۰۵/۰۷/۰۴"

func ambiguousReading() extract.Result {
	r := melliReading("")
	r.Transactions[0].Direction = extract.DirAmbiguous
	return r
}

// promptID is the Bale id of the last message the bot sent (the fake hands
// out ids from 1001 upwards).
func promptID(h *bottest.Harness) int64 { return 1000 + int64(len(h.Sent())) }

// replyToQuestion answers the bot's latest message.
func replyToQuestion(h *bottest.Harness, text string) {
	m := h.OwnerMessage(text)
	m.ReplyToMessage = &bale.Message{MessageID: promptID(h), Chat: m.Chat}
	h.SendMessage(m)
}

// questionMessage is the last sent message as the Owner sees it, for taps.
func questionMessage(h *bottest.Harness) bale.Message {
	s := h.LastSent()
	return bale.Message{MessageID: promptID(h), Chat: bale.Chat{ID: s.ChatID, Type: bale.ChatPrivate}, Text: s.Text, ReplyMarkup: s.ReplyMarkup}
}

func allTransactions(t *testing.T, h *bottest.Harness) []storage.Transaction {
	t.Helper()
	all, err := h.Store.AllTransactions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return all
}

func onlyTransaction(t *testing.T, h *bottest.Harness) storage.Transaction {
	t.Helper()
	all := allTransactions(t, h)
	if len(all) != 1 {
		t.Fatalf("%d Transactions stored, want 1: %+v", len(all), all)
	}
	return all[0]
}

func TestMissingAmountAsksForItAndTheReplyCompletesIt(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(noAmountReading())

	h.SendText(melliBale)

	q := h.LastSent()
	assertContains(t, q.Text, "#برداشت_با_POS", "amount", "toman", "Reply")
	if len(h.Sent()) != 1 {
		t.Fatalf("sent %d messages, want just the question", len(h.Sent()))
	}
	if n := len(allTransactions(t, h)); n != 0 {
		t.Fatalf("%d Transactions saved before the answer", n)
	}

	replyToQuestion(h, "۴۵۰ هزار")

	tx := onlyTransaction(t, h)
	if tx.Flagged || tx.AmountToman == nil || *tx.AmountToman != 450000 || tx.Direction != storage.DirectionOut {
		t.Errorf("flagged %v amount %v direction %q", tx.Flagged, tx.AmountToman, tx.Direction)
	}
	if tx.RawText != melliBale || tx.CategoryID == nil {
		t.Errorf("raw %q category %v", tx.RawText, tx.CategoryID)
	}
	assertContains(t, h.LastSent().Text, "Saved.", "450,000 toman")
	if n := len(h.Categorizer.Calls()); n != 1 {
		t.Errorf("categorizer called %d times, want 1 (after the answer)", n)
	}
}

func TestAmountReplyIsReadOnlyAsAnAmount(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(noAmountReading())
	h.SendText(melliBale)

	replyToQuestion(h, "just lunch") // not an amount: no second question, kept Flagged

	tx := onlyTransaction(t, h)
	if !tx.Flagged || tx.FlagReason != storage.FlagFollowupUnparsed || tx.AmountToman != nil {
		t.Errorf("flagged %v reason %q amount %v", tx.Flagged, tx.FlagReason, tx.AmountToman)
	}
	if tx.RawText != melliBale {
		t.Errorf("raw_text = %q", tx.RawText)
	}
	if len(h.Sent()) != 2 {
		t.Errorf("sent %d messages, want the question and one flagged confirmation", len(h.Sent()))
	}
	assertContains(t, h.LastSent().Text, "flagged")
	if n := len(h.Categorizer.Calls()); n != 0 {
		t.Errorf("categorizer called %d times for a Transaction with no amount", n)
	}
}

func TestAmbiguousDirectionAsksWithButtons(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(ambiguousReading())
	h.SendText(ambiguousTransfer)

	q := h.LastSent()
	assertContains(t, q.Text, "124,000 toman", "4 Mehr 1405", "Tap one")
	for _, label := range []string{"Expense", "Income", "Internal"} {
		if d := buttonData(t, q, label); !strings.HasPrefix(d, "f:") {
			t.Errorf("[%s] callback_data = %q, want the f: prefix", label, d)
		}
	}
	if n := len(allTransactions(t, h)); n != 0 {
		t.Fatalf("%d Transactions saved before the answer", n)
	}

	h.Categorizer.Choose("Other income", 0.9)
	h.Tap(questionMessage(h), buttonData(t, q, "Income"))

	tx := onlyTransaction(t, h)
	if tx.Flagged || tx.Direction != storage.DirectionIn || tx.AmountToman == nil || *tx.AmountToman != 124000 {
		t.Errorf("flagged %v direction %q amount %v", tx.Flagged, tx.Direction, tx.AmountToman)
	}
	// Categorizing runs after the Direction is answered.
	if n := len(h.Categorizer.Calls()); n != 1 {
		t.Fatalf("categorizer called %d times, want 1", n)
	}
	c, ok, err := h.Store.CategoryByID(context.Background(), *tx.CategoryID)
	if err != nil || !ok || c.Name != "Other income" {
		t.Errorf("category = %+v ok %v err %v", c, ok, err)
	}
	assertContains(t, h.LastSent().Text, "Saved.", "Income", "Other income")
	if e := h.Bale.Edits(); len(e) == 0 || e[len(e)-1].ReplyMarkup != nil {
		t.Errorf("the question keeps its buttons: %+v", e)
	}
}

func TestAmbiguousDirectionAnsweredInternalIsNotCategorized(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(ambiguousReading())
	h.SendText(ambiguousTransfer)
	q := h.LastSent()

	h.Tap(questionMessage(h), buttonData(t, q, "Internal"))

	tx := onlyTransaction(t, h)
	if tx.Direction != storage.DirectionInternal || tx.CategoryID != nil || tx.Flagged {
		t.Errorf("direction %q category %v flagged %v", tx.Direction, tx.CategoryID, tx.Flagged)
	}
	if n := len(h.Categorizer.Calls()); n != 0 {
		t.Errorf("categorizer called %d times for an internal transfer", n)
	}
}

func TestDirectionQuestionAcceptsATypedWordAndRejectsOthers(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(ambiguousReading())
	h.SendText(ambiguousTransfer)
	replyToQuestion(h, "income")
	if tx := onlyTransaction(t, h); tx.Flagged || tx.Direction != storage.DirectionIn {
		t.Errorf("flagged %v direction %q", tx.Flagged, tx.Direction)
	}

	h = bottest.New(t)
	h.Extractor.Return(ambiguousReading())
	h.SendText(ambiguousTransfer)
	replyToQuestion(h, "۴۵۰ هزار")
	if tx := onlyTransaction(t, h); !tx.Flagged || tx.FlagReason != storage.FlagFollowupUnparsed {
		t.Errorf("flagged %v reason %q", tx.Flagged, tx.FlagReason)
	}
}

func TestNoAnswerWithinThirtyMinutesSavesItFlagged(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(noAmountReading())
	h.SendText(melliBale)

	h.Clock.Advance(29 * time.Minute)
	h.ExpireFollowUps()
	if n := len(allTransactions(t, h)); n != 0 {
		t.Fatalf("saved after 29 minutes")
	}

	h.Clock.Advance(2 * time.Minute)
	h.ExpireFollowUps()

	tx := onlyTransaction(t, h)
	if !tx.Flagged || tx.FlagReason != storage.FlagFollowupTimeout || tx.AmountToman != nil || tx.RawText != melliBale {
		t.Errorf("flagged %v reason %q amount %v raw %q", tx.Flagged, tx.FlagReason, tx.AmountToman, tx.RawText)
	}
	assertContains(t, h.LastSent().Text, "No answer", "flagged")

	// Sweeping again changes nothing.
	n := len(h.Sent())
	h.ExpireFollowUps()
	if len(h.Sent()) != n || len(allTransactions(t, h)) != 1 {
		t.Error("a second sweep sent or saved something")
	}
}

func TestADirectionTimeoutKeepsTheAmount(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(ambiguousReading())
	h.SendText(ambiguousTransfer)

	expireFollowUps(h)

	tx := onlyTransaction(t, h)
	if !tx.Flagged || tx.FlagReason != storage.FlagFollowupTimeout || tx.AmountToman == nil || *tx.AmountToman != 124000 {
		t.Errorf("flagged %v reason %q amount %v", tx.Flagged, tx.FlagReason, tx.AmountToman)
	}
	if n := len(h.Categorizer.Calls()); n != 0 {
		t.Errorf("categorizer called %d times before the Direction is known", n)
	}
	// The question's buttons are gone.
	if e := h.Bale.Edits(); len(e) == 0 || e[len(e)-1].ReplyMarkup != nil {
		t.Errorf("the question keeps its buttons: %+v", e)
	}
}

func TestAnAnswerAfterTheDeadlineIsTooLate(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(noAmountReading())
	h.SendText(melliBale)
	h.Clock.Advance(31 * time.Minute) // no sweep ran yet

	replyToQuestion(h, "450000")

	tx := onlyTransaction(t, h)
	if !tx.Flagged || tx.FlagReason != storage.FlagFollowupTimeout || tx.AmountToman != nil {
		t.Errorf("flagged %v reason %q amount %v", tx.Flagged, tx.FlagReason, tx.AmountToman)
	}
}

func TestANewInputClosesTheOpenFollowUpAsFlagged(t *testing.T) {
	for _, tc := range []struct {
		name   string
		first  extract.Result
		text   string
		reason string
	}{
		{"amount", noAmountReading(), melliBale, storage.FlagAmountMissing},
		{"direction", ambiguousReading(), ambiguousTransfer, storage.FlagDirectionAmbiguous},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := bottest.New(t)
			h.Extractor.Return(tc.first)
			h.SendText(tc.text)
			q := h.LastSent()
			qm := questionMessage(h)

			other := melliReading("")
			other.Transactions[0].Amount = 1250000
			h.Extractor.Reset()
			h.Extractor.Return(other)
			h.SendText(otherPayment())

			all := allTransactions(t, h)
			if len(all) != 2 {
				t.Fatalf("%d Transactions, want the flagged one and the new one", len(all))
			}
			if !all[0].Flagged || all[0].FlagReason != tc.reason || all[0].RawText != tc.text {
				t.Errorf("first: flagged %v reason %q raw %q", all[0].Flagged, all[0].FlagReason, all[0].RawText)
			}
			if all[1].Flagged || all[1].AmountToman == nil || *all[1].AmountToman != 125000 {
				t.Errorf("second: %+v", all[1])
			}
			sent := h.Sent()
			assertContains(t, sent[len(sent)-2].Text, "moved on", "flagged")
			assertContains(t, sent[len(sent)-1].Text, "125,000 toman")

			// The closed question can no longer be answered.
			if q.ReplyMarkup != nil {
				h.Tap(qm, buttonData(t, q, "Income"))
				a := h.Bale.Answers()
				if a[len(a)-1].Text != "This question is closed." {
					t.Errorf("stale tap answered %q", a[len(a)-1].Text)
				}
			}
		})
	}
}

func TestExtractionFailureSavesTheInputFlaggedWithItsText(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Fail(errors.New("503 no healthy upstream"))

	h.SendText(melliBale)

	tx := onlyTransaction(t, h)
	if !tx.Flagged || tx.FlagReason != storage.FlagAmountMissing || tx.AmountToman != nil || tx.RawText != melliBale {
		t.Errorf("flagged %v reason %q amount %v raw %q", tx.Flagged, tx.FlagReason, tx.AmountToman, tx.RawText)
	}
	assertContains(t, h.LastSent().Text, "Couldn't read", "flagged")
}

func TestFollowUpsSurviveARestart(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(noAmountReading())
	h.SendText(melliBale)
	id := promptID(h)

	h.Restart()
	m := h.OwnerMessage("250")
	m.ReplyToMessage = &bale.Message{MessageID: id, Chat: m.Chat}
	h.SendMessage(m)

	if tx := onlyTransaction(t, h); tx.Flagged || tx.AmountToman == nil || *tx.AmountToman != 250000 {
		t.Errorf("flagged %v amount %v", tx.Flagged, tx.AmountToman)
	}
}

func TestFollowUpsThatExpiredDuringDowntimeAreFlaggedAtStartup(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(noAmountReading())
	h.SendText(melliBale)

	h.Clock.Advance(2 * time.Hour) // the bot was down
	h.Restart()

	tx := onlyTransaction(t, h)
	if !tx.Flagged || tx.FlagReason != storage.FlagFollowupTimeout || tx.RawText != melliBale {
		t.Errorf("flagged %v reason %q raw %q", tx.Flagged, tx.FlagReason, tx.RawText)
	}
	assertContains(t, h.LastSent().Text, "flagged")
}

func TestABatchAsksItsFollowUpsOneAtATime(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(extract.Result{IsTransaction: true, Transactions: []extract.Item{
		{Direction: extract.DirOut, Confidence: 0.9},
		{Direction: extract.DirOut, Confidence: 0.9},
	}})
	h.SendText("خرید فروشگاه الف\n\nخرید فروشگاه ب")

	if len(h.Sent()) != 1 {
		t.Fatalf("sent %d messages, want only the first question", len(h.Sent()))
	}
	assertContains(t, h.LastSent().Text, "فروشگاه الف")

	replyToQuestion(h, "100000")

	sent := h.Sent()
	if len(sent) != 3 {
		t.Fatalf("sent %d messages, want question, confirmation, second question", len(sent))
	}
	assertContains(t, sent[1].Text, "Saved.", "100,000 toman")
	assertContains(t, sent[2].Text, "فروشگاه ب")
	if n := len(allTransactions(t, h)); n != 1 {
		t.Fatalf("%d Transactions saved, want 1 so far", n)
	}

	replyToQuestion(h, "200000")

	all := allTransactions(t, h)
	if len(all) != 2 || all[1].AmountToman == nil || *all[1].AmountToman != 200000 || all[1].Flagged {
		t.Fatalf("Transactions = %+v", all)
	}
}

func TestABatchFollowUpNotAnsweredIsFlaggedAndTheNextIsAsked(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(extract.Result{IsTransaction: true, Transactions: []extract.Item{
		{Direction: extract.DirOut, Confidence: 0.9},
		{Direction: extract.DirOut, Confidence: 0.9},
	}})
	h.SendText("خرید فروشگاه الف\n\nخرید فروشگاه ب")

	expireFollowUps(h)

	all := allTransactions(t, h)
	if len(all) != 1 || all[0].FlagReason != storage.FlagFollowupTimeout {
		t.Fatalf("Transactions = %+v", all)
	}
	assertContains(t, h.LastSent().Text, "فروشگاه ب")
}

func TestAnAmountAnswerLeavesAnAmbiguousDirectionFlagged(t *testing.T) {
	h := bottest.New(t)
	r := noAmountReading()
	r.Transactions[0].Direction = extract.DirAmbiguous
	h.Extractor.Return(r)
	h.SendText("بانک ملی\nانتقال")

	replyToQuestion(h, "450000")

	// At most one Follow-up per Transaction: no second question.
	tx := onlyTransaction(t, h)
	if !tx.Flagged || tx.FlagReason != storage.FlagDirectionAmbiguous || tx.AmountToman == nil || *tx.AmountToman != 450000 {
		t.Errorf("flagged %v reason %q amount %v", tx.Flagged, tx.FlagReason, tx.AmountToman)
	}
	if len(h.Sent()) != 2 {
		t.Errorf("sent %d messages, want the question and the confirmation", len(h.Sent()))
	}
}

func TestOpenFollowUpReAsksTheMissingFieldOfAFlaggedTransaction(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(noAmountReading())
	h.SendText(melliBale)
	expireFollowUps(h)
	id := onlyTransaction(t, h).ID
	ctx := context.Background()

	ok, err := h.Bot.OpenFollowUp(ctx, bottest.OwnerID, id)
	if err != nil || !ok {
		t.Fatalf("OpenFollowUp = %v, %v", ok, err)
	}
	assertContains(t, h.LastSent().Text, "amount")
	if again, err := h.Bot.OpenFollowUp(ctx, bottest.OwnerID, id); err != nil || again {
		t.Errorf("a second OpenFollowUp = %v, %v, want false (one per Transaction)", again, err)
	}

	replyToQuestion(h, "80000")

	tx := storedTransaction(t, h, id)
	if tx.Flagged || tx.FlagReason != "" || tx.AmountToman == nil || *tx.AmountToman != 80000 || tx.CategoryID == nil {
		t.Errorf("flagged %v reason %q amount %v category %v", tx.Flagged, tx.FlagReason, tx.AmountToman, tx.CategoryID)
	}
	if n := len(allTransactions(t, h)); n != 1 {
		t.Errorf("%d Transactions, want the same one", n)
	}
	assertContains(t, h.LastSent().Text, "Saved.", "80,000 toman")

	if ok, _ := h.Bot.OpenFollowUp(ctx, bottest.OwnerID, id); ok {
		t.Error("OpenFollowUp on a fixed Transaction opened a question")
	}
}

func TestAnUnreadableFixAnswerLeavesTheTransactionFlagged(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(noAmountReading())
	h.SendText(melliBale)
	expireFollowUps(h)
	id := onlyTransaction(t, h).ID
	if ok, err := h.Bot.OpenFollowUp(context.Background(), bottest.OwnerID, id); err != nil || !ok {
		t.Fatalf("OpenFollowUp = %v, %v", ok, err)
	}

	replyToQuestion(h, "no idea")

	if tx := storedTransaction(t, h, id); !tx.Flagged || tx.AmountToman != nil {
		t.Errorf("flagged %v amount %v", tx.Flagged, tx.AmountToman)
	}
}
