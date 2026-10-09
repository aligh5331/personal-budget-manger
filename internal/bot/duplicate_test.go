package bot_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
	"github.com/aligh5331/personal-budget-manger/internal/extract"
)

func countSaved(t *testing.T, h *bottest.Harness) int {
	t.Helper()
	n := 0
	for id := int64(1); id < 50; id++ {
		if _, ok, _ := h.Store.TransactionByID(context.Background(), id); ok {
			n++
		}
	}
	return n
}

func TestForwardingTheSameMessageTwiceIsCaughtAsADuplicate(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Repeat() // the same message is sent twice
	h.Extractor.Return(melliReading(""))
	h.SendText(melliBale)

	h.SendText(melliBale)

	sent := h.Sent()
	if len(sent) != 2 {
		t.Fatalf("sent %d messages, want confirmation + duplicate warning", len(sent))
	}
	dup := sent[1]
	assertContains(t, dup.Text, "looks like a duplicate", "124,000 toman")
	buttonData(t, dup, "Save anyway")
	if n := countSaved(t, h); n != 1 {
		t.Errorf("%d Transactions stored, want 1", n)
	}
}

func TestSaveAnywaySavesTheDuplicate(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Repeat() // the same message is sent twice
	h.Extractor.Return(melliReading("تنقلات"))
	h.SendText(melliBale + "\nتنقلات")
	h.SendText(melliBale + "\nتنقلات")
	dup := h.LastSent()

	h.Tap(asReceived(dup, 700), buttonData(t, dup, "Save anyway"))

	if n := countSaved(t, h); n != 2 {
		t.Fatalf("%d Transactions stored, want 2", n)
	}
	edits := h.Bale.Edits()
	if len(edits) != 1 || edits[0].MessageID != 700 {
		t.Fatalf("edits = %+v, want the warning edited into a confirmation", edits)
	}
	assertContains(t, edits[0].Text, "Saved", "124,000 toman", "تنقلات")
	id := savedID(t, bale.SendMessageParams{Text: edits[0].Text, ReplyMarkup: edits[0].ReplyMarkup})
	if tx := storedTransaction(t, h, id); tx.Description != "تنقلات" || tx.AmountToman == nil || *tx.AmountToman != 124000 {
		t.Errorf("saved %+v", tx)
	}

	// A second tap on the same button saves nothing more.
	h.Tap(asReceived(dup, 700), buttonData(t, dup, "Save anyway"))
	if n := countSaved(t, h); n != 2 {
		t.Errorf("%d Transactions stored after a second tap, want 2", n)
	}
}

func TestTheSameAmountAtAnotherMinuteIsNotADuplicate(t *testing.T) {
	h := bottest.New(t)
	r := melliReading("")
	r.Transactions[0].Time = "12:06"
	h.Extractor.Return(melliReading(""))
	h.Extractor.Return(r)
	h.SendText(melliBale)

	h.SendText(strings.Split(melliBale, "زمان")[0] + "زمان: *12:06 1405/07/04*")

	assertContains(t, h.LastSent().Text, "Saved")
	if n := countSaved(t, h); n != 2 {
		t.Errorf("%d Transactions stored, want 2", n)
	}
}

func TestTheSameAmountInTheOtherDirectionIsNotADuplicate(t *testing.T) {
	h := bottest.New(t)
	r := melliReading("")
	r.Transactions[0].Direction = extract.DirIn
	h.Extractor.Return(melliReading(""))
	h.Extractor.Return(r)
	h.SendText(melliBale)

	h.SendText(strings.Replace(melliBale, "۱,۲۴۰,۰۰۰-", "۱,۲۴۰,۰۰۰+", 1))

	assertContains(t, h.LastSent().Text, "Saved", "Income")
	if n := countSaved(t, h); n != 2 {
		t.Errorf("%d Transactions stored, want 2", n)
	}
}

func TestWithoutABankTimeTheSameDateCounts(t *testing.T) {
	const sms = "بانك ملي ايران\nبرداشت:2,000,000-\n0707"
	item := extract.Item{Amount: 2000000, AmountUnit: extract.UnitRial, Direction: extract.DirOut, Date: "07/07", Confidence: 0.9}
	other := item
	other.Date = "07/08"
	h := bottest.New(t)
	h.Extractor.Return(extract.Result{IsTransaction: true, Transactions: []extract.Item{item}})
	h.Extractor.Return(extract.Result{IsTransaction: true, Transactions: []extract.Item{item}})
	h.Extractor.Return(extract.Result{IsTransaction: true, Transactions: []extract.Item{other}})
	h.SendText(sms)
	h.SendText(sms)

	assertContains(t, h.LastSent().Text, "looks like a duplicate")
	if n := countSaved(t, h); n != 1 {
		t.Errorf("%d Transactions stored, want 1", n)
	}

	// Another date is a different payment.
	h.SendText(strings.Replace(sms, "0707", "0708", 1))
	assertContains(t, h.LastSent().Text, "Saved")
	if n := countSaved(t, h); n != 2 {
		t.Errorf("%d Transactions stored, want 2", n)
	}
}

func TestADuplicateInsideABatchIsHeldBackOnItsOwn(t *testing.T) {
	h := bottest.New(t)
	// The second Input is a new message followed by the same two again.
	fresh := extract.Item{Amount: 500000, AmountUnit: extract.UnitRial, Direction: extract.DirOut, Date: "07/11", Time: "12:10", Confidence: 0.9}
	second := case16Reading()
	second.Transactions = append([]extract.Item{fresh}, second.Transactions...)
	h.Extractor.Return(case16Reading())
	h.Extractor.Return(second)
	h.SendText(smsCase16)

	h.SendText("بانك ملي ايران\nخريداينترنتي:500,000-\n0711-12:10\n\n" + smsCase16)

	sent := h.Sent()
	if len(sent) != 4 {
		t.Fatalf("sent %d messages, want first confirmation + new one + 2 duplicate warnings", len(sent))
	}
	assertContains(t, sent[1].Text, "50,000 toman")
	for _, m := range sent[2:] {
		assertContains(t, m.Text, "looks like a duplicate")
	}
	if n := countSaved(t, h); n != 3 {
		t.Errorf("%d Transactions stored, want 3", n)
	}
}
