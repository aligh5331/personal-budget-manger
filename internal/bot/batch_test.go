package bot_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
	"github.com/aligh5331/personal-budget-manger/internal/extract"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// Sanitized SMS-format fixtures (made-up balances).
const (
	// Sample case 16: two purchases and one note for both.
	smsCase16 = "بانك ملي ايران\n" +
		"خريداينترنتي:1,500,000-\n" +
		"مانده:10,200,000\n" +
		"0711-12:00\n" +
		"\n" +
		"بانك ملي ايران\n" +
		"خريداينترنتي:2,300,000-\n" +
		"مانده:7,900,000\n" +
		"0711-12:05\n" +
		"هديه تولد مامان"

	// Sample case 8: money moved between the Owner's own accounts.
	smsCase8 = "بانك ملي ايران\n" +
		"انتقالي:63,000,000-\n" +
		"مانده:333,333\n" +
		"0705-13:23\n" +
		"\n" +
		"بانك ملي ايران\n" +
		"انتقالي:63,000,000+\n" +
		"مانده:44,444,444\n" +
		"0705-13:23\n" +
		"جابجایی بین حساب هام"
)

func case16Reading() extract.Result {
	return extract.Result{IsTransaction: true, Note: "هديه تولد مامان", Transactions: []extract.Item{
		{Amount: 1500000, AmountUnit: extract.UnitRial, Direction: extract.DirOut, Date: "07/11", Time: "12:00", BankLabel: "خريداينترنتي", Confidence: 0.9},
		{Amount: 2300000, AmountUnit: extract.UnitRial, Direction: extract.DirOut, Date: "07/11", Time: "12:05", BankLabel: "خريداينترنتي", Confidence: 0.9},
	}}
}

func case8Reading() extract.Result {
	return extract.Result{IsTransaction: true, Note: "جابجایی بین حساب هام", Transactions: []extract.Item{
		{Amount: 63000000, AmountUnit: extract.UnitRial, Direction: extract.DirOut, Date: "07/05", Time: "13:23", BankLabel: "انتقالي", Confidence: 0.9},
		{Amount: 63000000, AmountUnit: extract.UnitRial, Direction: extract.DirIn, Date: "07/05", Time: "13:23", BankLabel: "انتقالي", Confidence: 0.9},
	}}
}

// confirmedIDs returns the Transaction ids behind a confirmation's [Undo]
// buttons, in the order they are listed.
func confirmedIDs(t *testing.T, msg bale.SendMessageParams) []int64 {
	t.Helper()
	if msg.ReplyMarkup == nil {
		t.Fatalf("message %q has no buttons", msg.Text)
	}
	var ids []int64
	for _, row := range msg.ReplyMarkup.InlineKeyboard {
		for _, b := range row {
			parts := strings.Split(b.CallbackData, ":")
			if len(parts) == 3 && parts[0] == "t" && parts[2] == "undo" {
				id, err := strconv.ParseInt(parts[1], 10, 64)
				if err != nil {
					t.Fatalf("callback_data %q: %v", b.CallbackData, err)
				}
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// asReceived is the confirmation as Bale hands it back on a button tap.
func asReceived(msg bale.SendMessageParams, messageID int64) bale.Message {
	return bale.Message{
		MessageID:   messageID,
		Chat:        bale.Chat{ID: msg.ChatID, Type: bale.ChatPrivate},
		Text:        msg.Text,
		ReplyMarkup: msg.ReplyMarkup,
	}
}

func TestABatchSavesOneTransactionPerBankMessage(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(case16Reading())

	u := h.SendText(smsCase16)

	sent := h.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d messages, want one confirmation listing both", len(sent))
	}
	conf := sent[0]
	assertContains(t, conf.Text, "150,000 toman", "230,000 toman", "11 Mehr 1405, 12:00", "11 Mehr 1405, 12:05")
	if n := strings.Count(conf.Text, "هدیه تولد مامان"); n != 2 {
		t.Errorf("note shown %d times, want once per Transaction: %q", n, conf.Text)
	}
	ids := confirmedIDs(t, conf)
	if len(ids) != 2 {
		t.Fatalf("confirmation has %d [Undo] buttons, want one per Transaction", len(ids))
	}
	for i, want := range []int64{150000, 230000} {
		tx := storedTransaction(t, h, ids[i])
		if tx.AmountToman == nil || *tx.AmountToman != want || tx.Direction != storage.DirectionOut {
			t.Errorf("Transaction %d: amount %v direction %q, want %d out", i, tx.AmountToman, tx.Direction, want)
		}
		if tx.Description != "هدیه تولد مامان" {
			t.Errorf("Transaction %d: description %q, want the shared note", i, tx.Description)
		}
		if want := strconv.FormatInt(u.Message.MessageID, 10) + ":" + strconv.Itoa(i); tx.InputID != want {
			t.Errorf("Transaction %d: input_id %q, want %q", i, tx.InputID, want)
		}
	}
}

func TestUndoInABatchRemovesOnlyThatTransaction(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(case16Reading())
	h.SendText(smsCase16)
	conf := h.LastSent()
	ids := confirmedIDs(t, conf)

	h.Tap(asReceived(conf, 900), buttonData(t, conf, "Undo 1"))

	if _, ok, _ := h.Store.TransactionByID(context.Background(), ids[0]); ok {
		t.Error("the first Transaction is still stored")
	}
	storedTransaction(t, h, ids[1])
	edits := h.Bale.Edits()
	if len(edits) != 1 || edits[0].MessageID != 900 {
		t.Fatalf("edits = %+v, want the confirmation edited", edits)
	}
	// The confirmation now lists only what is still saved.
	e := edits[0]
	assertContains(t, e.Text, "230,000 toman")
	if strings.Contains(e.Text, "150,000 toman") {
		t.Errorf("edited confirmation still shows the removed Transaction: %q", e.Text)
	}
	if e.ReplyMarkup == nil {
		t.Fatal("edited confirmation lost the buttons of the Transaction still saved")
	}
	left := confirmedIDs(t, bale.SendMessageParams{Text: e.Text, ReplyMarkup: e.ReplyMarkup})
	if len(left) != 1 || left[0] != ids[1] {
		t.Errorf("buttons left for %v, want only %d", left, ids[1])
	}

	// Undoing the last one leaves "Removed".
	h.Tap(bale.Message{MessageID: 900, Chat: bale.Chat{ID: conf.ChatID, Type: bale.ChatPrivate}, Text: e.Text, ReplyMarkup: e.ReplyMarkup},
		buttonData(t, bale.SendMessageParams{Text: e.Text, ReplyMarkup: e.ReplyMarkup}, "Undo"))
	if edits := h.Bale.Edits(); len(edits) != 2 || edits[1].Text != "Removed" || edits[1].ReplyMarkup != nil {
		t.Errorf("last edit = %+v, want \"Removed\" with no buttons", edits[len(edits)-1])
	}
}

func TestMoreThanFiveBankMessagesSavesNothing(t *testing.T) {
	h := bottest.New(t)
	msg := "بانك ملي ايران\nخريداينترنتي:1,500,000-\n0711-12:00"
	item := extract.Item{Amount: 1500000, AmountUnit: extract.UnitRial, Direction: extract.DirOut, Date: "07/11", Time: "12:00", Confidence: 0.9}
	texts := make([]string, 6)
	items := make([]extract.Item, 6)
	for i := range 6 {
		texts[i], items[i] = msg, item
	}
	h.Extractor.Return(extract.Result{IsTransaction: true, Transactions: items})

	h.SendText(strings.Join(texts, "\n\n"))

	sent := h.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d messages, want one reply", len(sent))
	}
	assertContains(t, sent[0].Text, "5", "smaller batches")
	if sent[0].ReplyMarkup != nil {
		t.Errorf("reply has buttons: %+v", sent[0].ReplyMarkup)
	}
	if _, ok, _ := h.Store.TransactionByID(context.Background(), 1); ok {
		t.Error("a Transaction was saved")
	}
}

func TestAnInternalPairIsSavedAsOneInternalTransfer(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(case8Reading())

	h.SendText(smsCase8)

	sent := h.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d messages, want one confirmation", len(sent))
	}
	conf := sent[0]
	assertContains(t, conf.Text, "6,300,000 toman", "Internal transfer", "5 Mehr 1405, 13:23", "جابجایی بین حساب هام")
	ids := confirmedIDs(t, conf)
	if len(ids) != 1 {
		t.Fatalf("confirmation lists %d Transactions, want 1", len(ids))
	}
	tx := storedTransaction(t, h, ids[0])
	if tx.Direction != storage.DirectionInternal || tx.AmountToman == nil || *tx.AmountToman != 6300000 {
		t.Errorf("direction %q amount %v, want internal 6,300,000 (stored once)", tx.Direction, tx.AmountToman)
	}
	if tx.CategoryID != nil || tx.Flagged {
		t.Errorf("category %v flagged %v, want no Category and not flagged", tx.CategoryID, tx.Flagged)
	}
}
