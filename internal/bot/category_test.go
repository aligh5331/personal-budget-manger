package bot_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
	"github.com/aligh5331/personal-budget-manger/internal/extract"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

var expenseNames = []string{
	"Food", "Snacks", "Groceries", "Transport", "Fuel", "Education", "Entertainment", "Health",
	"Bills", "Loan installment", "Shopping", "Cash withdrawal", "Rent", "Gifts", "Other",
}

// categoryNamed finds a seeded Category by name.
func categoryNamed(t *testing.T, h *bottest.Harness, name string) storage.Category {
	t.Helper()
	ctx := context.Background()
	if un, err := h.Store.Uncategorized(ctx); err == nil && un.Name == name {
		return un
	}
	for _, kind := range []string{storage.KindExpense, storage.KindIncome} {
		cs, err := h.Store.ActiveCategories(ctx, kind)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			if c.Name == name {
				return c
			}
		}
	}
	t.Fatalf("no active Category %q", name)
	return storage.Category{}
}

func assertCategory(t *testing.T, h *bottest.Harness, tx storage.Transaction, name string) {
	t.Helper()
	want := categoryNamed(t, h, name)
	if tx.CategoryID == nil || *tx.CategoryID != want.ID {
		t.Errorf("category_id = %v, want %s (%d)", tx.CategoryID, name, want.ID)
	}
}

func TestTheStartingCategoryList(t *testing.T) {
	h := bottest.New(t)
	ctx := context.Background()

	expense, err := h.Store.ActiveCategories(ctx, storage.KindExpense)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range expense {
		names = append(names, c.Name)
		if c.Hint == "" {
			t.Errorf("%s has no hint", c.Name)
		}
	}
	if !slices.Equal(names, expenseNames) {
		t.Errorf("expense Categories = %q, want %q", names, expenseNames)
	}
	income, _ := h.Store.ActiveCategories(ctx, storage.KindIncome)
	if len(income) != 2 || income[0].Name != "Salary" || income[1].Name != "Other income" {
		t.Errorf("income Categories = %+v", income)
	}
	un, err := h.Store.Uncategorized(ctx)
	if err != nil || un.Name != "Uncategorized" || !un.BuiltIn || un.Kind != storage.KindAny {
		t.Errorf("Uncategorized = %+v, %v", un, err)
	}
}

func TestAConfidentPickIsSaved(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(melliReading("تنقلات"))
	h.Categorizer.Choose("Snacks", 0.92)

	input := melliBale + "\nتنقلات"
	h.SendText(input)

	calls := h.Categorizer.Calls()
	if len(calls) != 1 {
		t.Fatalf("categorizer called %d times, want 1", len(calls))
	}
	if calls[0].Text != input {
		t.Errorf("categorizer text = %q, want the Input", calls[0].Text)
	}
	if want := append(slices.Clone(expenseNames), "Uncategorized"); !slices.Equal(calls[0].OptionNames(), want) {
		t.Errorf("options = %q, want expense Categories plus Uncategorized", calls[0].OptionNames())
	}
	conf := h.LastSent()
	assertContains(t, conf.Text, "Category: Snacks")
	tx := storedTransaction(t, h, savedID(t, conf))
	assertCategory(t, h, tx, "Snacks")
	if tx.Flagged || tx.CategorizePending {
		t.Errorf("flagged %v pending %v", tx.Flagged, tx.CategorizePending)
	}
	if !h.Logs.Mentions("0.92") {
		t.Error("the Category confidence is not logged")
	}
}

func TestIncomeIsOfferedIncomeCategories(t *testing.T) {
	h := bottest.New(t)
	r := melliReading("حقوق")
	r.Transactions[0].Direction = extract.DirIn
	h.Extractor.Return(r)
	h.Categorizer.Choose("Salary", 0.95)

	h.SendText(strings.Replace(melliBale, "-*", "+*", 1) + "\nحقوق")

	calls := h.Categorizer.Calls()
	if len(calls) != 1 || !slices.Equal(calls[0].OptionNames(), []string{"Salary", "Other income", "Uncategorized"}) {
		t.Fatalf("calls = %+v, want income Categories plus Uncategorized", calls)
	}
	conf := h.LastSent()
	assertContains(t, conf.Text, "Income", "Category: Salary")
	assertCategory(t, h, storedTransaction(t, h, savedID(t, conf)), "Salary")
}

func TestALowConfidencePickSavesUncategorized(t *testing.T) {
	for _, tc := range []struct {
		confidence float64
		want       string
	}{
		{0.69, "Uncategorized"},
		{0.7, "Snacks"},
	} {
		t.Run(fmt.Sprint(tc.confidence), func(t *testing.T) {
			h := bottest.New(t)
			h.Extractor.Return(melliReading("تنقلات"))
			h.Categorizer.Choose("Snacks", tc.confidence)

			h.SendText(melliBale + "\nتنقلات")

			if n := len(h.Sent()); n != 1 {
				t.Errorf("sent %d messages, want only the confirmation (no Follow-up)", n)
			}
			conf := h.LastSent()
			assertContains(t, conf.Text, "Saved.\n", "Category: "+tc.want)
			tx := storedTransaction(t, h, savedID(t, conf))
			assertCategory(t, h, tx, tc.want)
			if tx.Flagged || tx.CategorizePending {
				t.Errorf("flagged %v pending %v, want neither", tx.Flagged, tx.CategorizePending)
			}
			if !h.Logs.Mentions(fmt.Sprint(tc.confidence)) {
				t.Error("the Category confidence is not logged")
			}
		})
	}
}

func TestACategorizerFailureSavesUncategorizedAndMarksItPending(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(melliReading("تنقلات"))
	h.Categorizer.Fail(errors.New("503 no healthy upstream"))

	h.SendText(melliBale + "\nتنقلات")

	conf := h.LastSent()
	assertContains(t, conf.Text, "Saved.\n", "Category: Uncategorized")
	tx := storedTransaction(t, h, savedID(t, conf))
	assertCategory(t, h, tx, "Uncategorized")
	if tx.Flagged || !tx.CategorizePending {
		t.Errorf("flagged %v pending %v, want pending only", tx.Flagged, tx.CategorizePending)
	}
}

func TestATransactionWithNoAmountIsNotCategorized(t *testing.T) {
	h := bottest.New(t)
	r := melliReading("")
	r.Transactions[0].Amount = 0
	h.Extractor.Return(r)

	h.SendText("اطلاع\u200cرسانی بانک ملّی ایران\n*#برداشت_با_POS*\nمانده: *۱۱,۱۱۱,۱۱۱* ریال")

	if n := len(h.Categorizer.Calls()); n != 0 {
		t.Errorf("categorizer called %d times for a Transaction with no amount", n)
	}
	assertContains(t, h.LastSent().Text, "Category: Uncategorized")
}

func TestInternalTransfersHaveNoCategory(t *testing.T) {
	h := bottest.New(t)
	amount := int64(500000)
	id, err := h.Store.SaveTransaction(context.Background(), storage.Transaction{
		CreatedAt: bottest.Start, OccurredAt: bottest.Start, AmountToman: &amount,
		Direction: storage.DirectionInternal, RawText: "pair", InputID: "1:0",
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := bale.Message{MessageID: 900, Chat: bale.Chat{ID: bottest.OwnerID, Type: bale.ChatPrivate}}

	h.Tap(msg, fmt.Sprintf("t:%d:cat", id))

	if a := h.Bale.Answers(); len(a) != 1 || a[0].Text != "Internal transfers have no Category." {
		t.Errorf("answers = %+v", a)
	}
	if len(h.Bale.Edits()) != 0 || len(h.Categorizer.Calls()) != 0 {
		t.Errorf("edits %+v, categorizer calls %d", h.Bale.Edits(), len(h.Categorizer.Calls()))
	}
	if tx := storedTransaction(t, h, id); tx.CategoryID != nil {
		t.Errorf("category_id = %v, want none", *tx.CategoryID)
	}
}

// buttonLabels lists the labels of an inline keyboard, row by row.
func buttonLabels(m *bale.InlineKeyboardMarkup) []string {
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

func findButton(t *testing.T, m *bale.InlineKeyboardMarkup, label string) string {
	t.Helper()
	if m != nil {
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
	}
	t.Fatalf("no [%s] button in %q", label, buttonLabels(m))
	return ""
}

// openPicker saves melliBale as Snacks and taps [Category] on its confirmation.
func openPicker(t *testing.T, h *bottest.Harness) (bale.Message, int64, bale.EditMessageTextParams) {
	t.Helper()
	h.Categorizer.Choose("Snacks", 0.9)
	msg, id := saveOne(t, h)
	h.Tap(msg, buttonData(t, h.LastSent(), "Category"))
	edits := h.Bale.Edits()
	if len(edits) != 1 {
		t.Fatalf("edits = %+v, want the confirmation turned into a picker", edits)
	}
	return msg, id, edits[0]
}

func TestTheCategoryPickerOffersMatchingActiveCategories(t *testing.T) {
	h := bottest.New(t)
	if _, err := h.Store.DB().Exec(`UPDATE categories SET archived = 1 WHERE name = 'Rent'`); err != nil {
		t.Fatal(err)
	}
	msg, _, picker := openPicker(t, h)

	if picker.MessageID != msg.MessageID {
		t.Errorf("edited message %d, want the confirmation %d", picker.MessageID, msg.MessageID)
	}
	assertContains(t, picker.Text, "Category: Snacks")
	labels := buttonLabels(picker.ReplyMarkup)
	for _, name := range expenseNames {
		want := name
		if name == "Snacks" {
			want = "• Snacks" // the current one is marked
		}
		if has := slices.Contains(labels, want); has == (name == "Rent") {
			t.Errorf("picker %q: has %q = %v", labels, want, has)
		}
	}
	for _, want := range []string{"Uncategorized", "Back"} {
		if !slices.Contains(labels, want) {
			t.Errorf("picker %q has no %q", labels, want)
		}
	}
	if slices.Contains(labels, "Salary") {
		t.Errorf("picker %q offers an income Category for an expense", labels)
	}
	if a := h.Bale.Answers(); len(a) != 1 {
		t.Errorf("answers = %+v", a)
	}
}

func TestPickingACategoryUpdatesTheTransactionAndConfirmation(t *testing.T) {
	h := bottest.New(t)
	msg, id, picker := openPicker(t, h)

	h.Tap(msg, findButton(t, picker.ReplyMarkup, "Food"))

	tx := storedTransaction(t, h, id)
	assertCategory(t, h, tx, "Food")
	edits := h.Bale.Edits()
	if len(edits) != 2 {
		t.Fatalf("edits = %+v", edits)
	}
	last := edits[1]
	assertContains(t, last.Text, "Category: Food", "124,000 toman")
	if labels := buttonLabels(last.ReplyMarkup); !slices.Contains(labels, "Undo") || !slices.Contains(labels, "Category") {
		t.Errorf("buttons after the pick = %q, want the confirmation's buttons back", labels)
	}
	if a := h.Bale.Answers(); a[len(a)-1].Text != "Category: Food" {
		t.Errorf("answers = %+v", a)
	}
}

func TestAnOwnerPickClearsThePendingMark(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(melliReading("تنقلات"))
	h.Categorizer.Fail(errors.New("timeout"))
	h.SendText(melliBale + "\nتنقلات")
	conf := h.LastSent()
	id := savedID(t, conf)
	msg := bale.Message{MessageID: 77, Chat: bale.Chat{ID: conf.ChatID, Type: bale.ChatPrivate}, Text: conf.Text}

	h.Tap(msg, buttonData(t, conf, "Category"))
	h.Tap(msg, findButton(t, h.Bale.Edits()[0].ReplyMarkup, "Snacks"))

	tx := storedTransaction(t, h, id)
	assertCategory(t, h, tx, "Snacks")
	if tx.CategorizePending {
		t.Error("categorize_pending still set after the Owner picked a Category")
	}
}

func TestBackLeavesTheCategoryAlone(t *testing.T) {
	h := bottest.New(t)
	msg, id, picker := openPicker(t, h)

	h.Tap(msg, findButton(t, picker.ReplyMarkup, "Back"))

	assertCategory(t, h, storedTransaction(t, h, id), "Snacks")
	edits := h.Bale.Edits()
	if len(edits) != 2 || !slices.Contains(buttonLabels(edits[1].ReplyMarkup), "Undo") {
		t.Fatalf("edits = %+v, want the confirmation's buttons back", edits)
	}
	assertContains(t, edits[1].Text, "Category: Snacks")
}

func TestAStalePickIsRefused(t *testing.T) {
	h := bottest.New(t)
	msg, id, picker := openPicker(t, h)
	rent := findButton(t, picker.ReplyMarkup, "Rent")
	salary := categoryNamed(t, h, "Salary")
	if _, err := h.Store.DB().Exec(`UPDATE categories SET archived = 1 WHERE name = 'Rent'`); err != nil {
		t.Fatal(err)
	}

	h.Tap(msg, rent)
	h.Tap(msg, fmt.Sprintf("t:%d:cat:%d", id, salary.ID)) // wrong kind
	h.Tap(msg, fmt.Sprintf("t:%d:cat:99999", id))         // no such Category

	assertCategory(t, h, storedTransaction(t, h, id), "Snacks")
	for _, a := range h.Bale.Answers()[1:] {
		if a.Text != "That Category is no longer available." {
			t.Errorf("answer = %q", a.Text)
		}
	}
}

func TestPickingOnARemovedTransactionSaysSo(t *testing.T) {
	h := bottest.New(t)
	msg, id, picker := openPicker(t, h)
	food := findButton(t, picker.ReplyMarkup, "Food")
	if _, err := h.Store.DeleteTransaction(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	h.Tap(msg, food)
	h.Tap(msg, fmt.Sprintf("t:%d:cat", id))

	for _, a := range h.Bale.Answers()[1:] {
		if a.Text != "This Transaction was removed." {
			t.Errorf("answer = %q", a.Text)
		}
	}
}
