package bot_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/extract"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// Sanitized Bank Melli Bale-bot notification (made-up balance).
const melliBale = "اطلاع\u200cرسانی بانک ملّی ایران: *بانک ملّی ایران*\n" +
	"*#برداشت_با_POS*\n" +
	"مبلغ: *۱,۲۴۰,۰۰۰-* ریال\n" +
	"مانده: *۱۱,۱۱۱,۱۱۱* ریال\n" +
	"زمان: *۱۲:۰۵ ۱۴۰۵/۰۷/۰۴*"

// melliReading is what the model returns for melliBale.
func melliReading(note string) extract.Result {
	return extract.Result{
		IsTransaction: true,
		Note:          note,
		Transactions: []extract.Item{{
			Amount: 1240000, AmountUnit: extract.UnitRial, Direction: extract.DirOut,
			Date: "1405/07/04", Time: "12:05", BankLabel: "#برداشت_با_POS", Confidence: 0.95,
		}},
	}
}

// savedID returns the Transaction id behind the confirmation's [Undo] button.
func savedID(t *testing.T, msg bale.SendMessageParams) int64 {
	t.Helper()
	data := buttonData(t, msg, "Undo")
	parts := strings.Split(data, ":")
	if len(parts) != 3 || parts[0] != "t" || parts[2] != "undo" {
		t.Fatalf("Undo callback_data = %q, want t:<id>:undo", data)
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		t.Fatalf("Undo callback_data = %q: %v", data, err)
	}
	return id
}

func buttonData(t *testing.T, msg bale.SendMessageParams, label string) string {
	t.Helper()
	if msg.ReplyMarkup == nil {
		t.Fatalf("message %q has no buttons", msg.Text)
	}
	for _, row := range msg.ReplyMarkup.InlineKeyboard {
		for _, b := range row {
			if b.Text == label {
				if len(b.CallbackData) > 64 {
					t.Errorf("callback_data %q is over 64 bytes", b.CallbackData)
				}
				return b.CallbackData
			}
		}
	}
	t.Fatalf("message %q has no [%s] button", msg.Text, label)
	return ""
}

func storedTransaction(t *testing.T, h *bottest.Harness, id int64) storage.Transaction {
	t.Helper()
	tx, ok, err := h.Store.TransactionByID(context.Background(), id)
	if err != nil || !ok {
		t.Fatalf("TransactionByID(%d) = ok %v, err %v", id, ok, err)
	}
	return tx
}

func assertContains(t *testing.T, text string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Errorf("message %q does not contain %q", text, w)
		}
	}
}

func TestForwardedBankMessageWithNoteIsSaved(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(melliReading("تنقلات"))

	input := melliBale + "\nتنقلات"
	u := h.SendText(input)

	if got := h.Extractor.Inputs(); len(got) != 1 || got[0] != input {
		t.Errorf("extractor got %q, want the Input verbatim", got)
	}
	sent := h.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d messages, want 1 confirmation", len(sent))
	}
	conf := sent[0]
	assertContains(t, conf.Text, "124,000 toman", "Expense", "Uncategorized", "تنقلات", "4 Mehr 1405")
	if strings.Contains(conf.Text, "#برداشت_با_POS") {
		t.Errorf("confirmation shows the bank label although the Owner wrote a note: %q", conf.Text)
	}

	tx := storedTransaction(t, h, savedID(t, conf))
	if tx.AmountToman == nil || *tx.AmountToman != 124000 {
		t.Errorf("amount_toman = %v, want 124000", tx.AmountToman)
	}
	wantAt := time.Date(2026, 9, 26, 12, 5, 0, 0, clock.Tehran())
	if tx.Direction != storage.DirectionOut || !tx.OccurredAt.Equal(wantAt) {
		t.Errorf("direction %q occurred_at %s, want out at %s", tx.Direction, tx.OccurredAt, wantAt)
	}
	if tx.Description != "تنقلات" || tx.BankLabel != "#برداشت_با_POS" {
		t.Errorf("description %q bank_label %q", tx.Description, tx.BankLabel)
	}
	if tx.RawText != input {
		t.Errorf("raw_text = %q, want the Input verbatim", tx.RawText)
	}
	if want := strconv.FormatInt(u.Message.MessageID, 10) + ":0"; tx.InputID != want {
		t.Errorf("input_id = %q, want %q", tx.InputID, want)
	}
	if tx.Flagged || tx.CategoryID == nil || !tx.CreatedAt.Equal(bottest.Start) {
		t.Errorf("flagged %v category %v created_at %s", tx.Flagged, tx.CategoryID, tx.CreatedAt)
	}
}

func TestWithoutANoteTheConfirmationShowsTheBankLabel(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(melliReading(""))

	h.SendText(melliBale)

	conf := h.LastSent()
	assertContains(t, conf.Text, "124,000 toman", "Bank text: «#برداشت_با_POS»")
	tx := storedTransaction(t, h, savedID(t, conf))
	if tx.Description != "" {
		t.Errorf("description = %q, want empty: it holds only the Owner's words", tx.Description)
	}
}

func TestANoteTheModelDroppedIsStillSaved(t *testing.T) {
	// Sample case 2: the model returns no note for a one-line note with a typo.
	h := bottest.New(t)
	h.Extractor.Return(melliReading(""))

	h.SendText(melliBale + "\nقست بانک")

	conf := h.LastSent()
	assertContains(t, conf.Text, "قست بانک")
	if tx := storedTransaction(t, h, savedID(t, conf)); tx.Description != "قست بانک" {
		t.Errorf("description = %q", tx.Description)
	}
}

func TestAMadeUpNoteIsNotSaved(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(melliReading("خرید تنقلات از سوپرمارکت"))

	h.SendText(melliBale + "\nتنقلات")

	if tx := storedTransaction(t, h, savedID(t, h.LastSent())); tx.Description != "تنقلات" {
		t.Errorf("description = %q, want the Owner's own words", tx.Description)
	}
}

func TestNotATransactionSavesNothing(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(extract.Result{IsTransaction: false})

	h.SendText("رمز یکبار مصرف: 12345")

	assertContains(t, h.LastSent().Text, "No transaction found")
	if _, ok, _ := h.Store.TransactionByID(context.Background(), 1); ok {
		t.Error("a Transaction was saved")
	}
}
