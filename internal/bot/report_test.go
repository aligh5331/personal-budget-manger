package bot_test

import (
	"strings"
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/jalali"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// seedAt saves a Transaction on a Jalali day at noon Tehran time.
func seedAt(t *testing.T, h *bottest.Harness, y, m, d int, tx storage.Transaction) int64 {
	t.Helper()
	tx.OccurredAt = jalali.Date{Year: y, Month: m, Day: d}.At(12, 0, clock.Tehran())
	if tx.RawText == "" {
		tx.RawText = "seed"
	}
	if tx.InputID == "" {
		tx.InputID = "seed"
	}
	return seedTransaction(t, h, tx)
}

func toman(n int64) *int64 { return &n }

func spent(t *testing.T, h *bottest.Harness, y, m, d int, cat string, amount int64) {
	t.Helper()
	id := categoryNamed(t, h, cat).ID
	seedAt(t, h, y, m, d, storage.Transaction{AmountToman: toman(amount), Direction: storage.DirectionOut, CategoryID: &id})
}

func earned(t *testing.T, h *bottest.Harness, y, m, d int, cat string, amount int64) {
	t.Helper()
	id := categoryNamed(t, h, cat).ID
	seedAt(t, h, y, m, d, storage.Transaction{AmountToman: toman(amount), Direction: storage.DirectionIn, CategoryID: &id})
}

func TestReportMonthWithMixedData(t *testing.T) {
	h := bottest.New(t)
	spent(t, h, 1405, 7, 2, "Food", 450_000)
	spent(t, h, 1405, 7, 9, "Food", 150_000)
	spent(t, h, 1405, 7, 3, "Transport", 300_000)
	spent(t, h, 1405, 7, 14, "Snacks", 100_000)
	earned(t, h, 1405, 7, 1, "Salary", 5_000_000)
	// Outside Mehr: not in this Report.
	spent(t, h, 1405, 6, 31, "Food", 999_000)
	spent(t, h, 1405, 8, 1, "Food", 888_000)
	// Internal transfers and Flagged transactions stay out of the totals.
	seedAt(t, h, 1405, 7, 5, storage.Transaction{AmountToman: toman(2_000_000), Direction: storage.DirectionInternal})
	seedAt(t, h, 1405, 7, 6, storage.Transaction{AmountToman: toman(1_000_000), Direction: storage.DirectionInternal})
	food := categoryNamed(t, h, "Food").ID
	seedAt(t, h, 1405, 7, 7, storage.Transaction{AmountToman: toman(777_000), Direction: storage.DirectionOut, CategoryID: &food, Flagged: true, FlagReason: storage.FlagFollowupTimeout})
	seedAt(t, h, 1405, 7, 8, storage.Transaction{Direction: storage.DirectionOut, Flagged: true, FlagReason: storage.FlagAmountMissing})

	h.SendText("/report")

	want := strings.Join([]string{
		"Report: Mehr 1405",
		"",
		"Expenses",
		"Food: 600,000 (60%)",
		"Transport: 300,000 (30%)",
		"Snacks: 100,000 (10%)",
		"",
		"Income",
		"Salary: 5,000,000 (100%)",
		"",
		"Expenses: 1,000,000",
		"Income: 5,000,000",
		"Net: 4,000,000",
		"",
		"Internal transfers: 3,000,000",
		"",
		"2 flagged, not counted: /flagged",
	}, "\n")
	got := h.LastSent()
	if got.Text != want {
		t.Errorf("report:\n%s\nwant:\n%s", got.Text, want)
	}
	if l := buttonLabels(got.ReplyMarkup); len(l) != 2 || l[0] != "Last month" || l[1] != "This month" {
		t.Errorf("buttons = %v, want [Last month This month]", l)
	}
	if len(h.Sent()) != 1 {
		t.Errorf("sent %d messages, want 1", len(h.Sent()))
	}
}

func TestReportNegativeNet(t *testing.T) {
	h := bottest.New(t)
	spent(t, h, 1405, 7, 2, "Rent", 3_000_000)
	earned(t, h, 1405, 7, 3, "Salary", 1_000_000)
	h.SendText("/report")
	if got := h.LastSent().Text; !strings.Contains(got, "Net: -2,000,000") {
		t.Errorf("report:\n%s", got)
	}
}

func TestReportOfAnEmptyMonth(t *testing.T) {
	h := bottest.New(t)
	h.SendText("/report")
	got := h.LastSent()
	if got.Text != "No transactions in Mehr 1405." {
		t.Errorf("text = %q", got.Text)
	}
	if l := buttonLabels(got.ReplyMarkup); len(l) != 2 {
		t.Errorf("buttons = %v, want the two month buttons even when empty", l)
	}
}

func TestReportWithOnlyFlaggedTransactions(t *testing.T) {
	h := bottest.New(t)
	seedAt(t, h, 1405, 7, 7, storage.Transaction{Direction: storage.DirectionOut, Flagged: true, FlagReason: storage.FlagAmountMissing})
	h.SendText("/report")
	want := "Report: Mehr 1405\n\n1 flagged, not counted: /flagged"
	if got := h.LastSent().Text; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestReportSwitchesMonths(t *testing.T) {
	h := bottest.New(t)
	spent(t, h, 1405, 7, 2, "Food", 400_000)
	spent(t, h, 1405, 6, 20, "Fuel", 250_000)
	earned(t, h, 1405, 6, 25, "Salary", 900_000)

	h.SendText("/report")
	report := h.LastSent()
	msg := bale.Message{MessageID: 4242, Chat: bale.Chat{ID: bottest.OwnerID, Type: bale.ChatPrivate}, Text: report.Text, ReplyMarkup: report.ReplyMarkup}
	var last, this string
	for _, row := range report.ReplyMarkup.InlineKeyboard {
		for _, b := range row {
			switch b.Text {
			case "Last month":
				last = b.CallbackData
			case "This month":
				this = b.CallbackData
			}
		}
	}

	h.Tap(msg, last)
	edits := h.Bale.Edits()
	if len(edits) != 1 {
		t.Fatalf("edits = %d, want 1", len(edits))
	}
	e := edits[0]
	if e.MessageID != 4242 || e.ChatID != bottest.OwnerID {
		t.Errorf("edited message %d in chat %d, want the Report message", e.MessageID, e.ChatID)
	}
	if !strings.HasPrefix(e.Text, "Report: Shahrivar 1405") || !strings.Contains(e.Text, "Fuel: 250,000 (100%)") || strings.Contains(e.Text, "Food") {
		t.Errorf("last month:\n%s", e.Text)
	}
	if !strings.Contains(e.Text, "Net: 650,000") {
		t.Errorf("last month net:\n%s", e.Text)
	}
	if l := buttonLabels(e.ReplyMarkup); len(l) != 2 {
		t.Errorf("buttons after switching = %v", l)
	}
	if len(h.Sent()) != 1 {
		t.Errorf("switching months sent %d messages, want only the original Report", len(h.Sent()))
	}

	h.Tap(msg, this)
	e = h.Bale.Edits()[1]
	if !strings.HasPrefix(e.Text, "Report: Mehr 1405") || !strings.Contains(e.Text, "Food: 400,000 (100%)") || strings.Contains(e.Text, "Fuel") {
		t.Errorf("this month:\n%s", e.Text)
	}
}

func TestReportLastMonthAcrossTheYearBoundary(t *testing.T) {
	h := bottest.New(t)
	// 3 Farvardin 1406 is 24 March 2027.
	h.Clock.Set(jalali.Date{Year: 1406, Month: 1, Day: 3}.At(10, 0, clock.Tehran()))
	spent(t, h, 1405, 12, 28, "Gifts", 500_000)
	h.SendText("/report")
	report := h.LastSent()
	if report.Text != "No transactions in Farvardin 1406." {
		t.Errorf("text = %q", report.Text)
	}
	msg := bale.Message{MessageID: 7, Chat: bale.Chat{ID: bottest.OwnerID, Type: bale.ChatPrivate}}
	h.Tap(msg, report.ReplyMarkup.InlineKeyboard[0][0].CallbackData)
	e := h.Bale.Edits()[0]
	if !strings.HasPrefix(e.Text, "Report: Esfand 1405") || !strings.Contains(e.Text, "Gifts: 500,000 (100%)") {
		t.Errorf("last month:\n%s", e.Text)
	}
}

func TestReportKeepsArchivedCategories(t *testing.T) {
	h := bottest.New(t)
	spent(t, h, 1405, 7, 2, "Gifts", 120_000)
	if _, err := h.Store.DB().Exec(`UPDATE categories SET archived = 1 WHERE name = 'Gifts'`); err != nil {
		t.Fatal(err)
	}
	h.SendText("/report")
	if got := h.LastSent().Text; !strings.Contains(got, "Gifts: 120,000 (100%)") {
		t.Errorf("report:\n%s", got)
	}
}

func TestReportCountsUncategorizedAsACategory(t *testing.T) {
	h := bottest.New(t)
	un, err := h.Store.Uncategorized(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	seedAt(t, h, 1405, 7, 2, storage.Transaction{AmountToman: toman(50_000), Direction: storage.DirectionOut, CategoryID: &un.ID})
	h.SendText("/report")
	if got := h.LastSent().Text; !strings.Contains(got, "Uncategorized: 50,000 (100%)") {
		t.Errorf("report:\n%s", got)
	}
}

func TestReportBoundariesFollowTehranMidnight(t *testing.T) {
	h := bottest.New(t)
	teh := clock.Tehran()
	food := categoryNamed(t, h, "Food").ID
	save := func(d jalali.Date, hour, min int, amount int64) {
		tx := storage.Transaction{AmountToman: toman(amount), Direction: storage.DirectionOut, CategoryID: &food, RawText: "x", InputID: "x"}
		tx.OccurredAt = d.At(hour, min, teh)
		seedTransaction(t, h, tx)
	}
	save(jalali.Date{Year: 1405, Month: 6, Day: 31}, 23, 59, 1_000) // last minute of Shahrivar
	save(jalali.Date{Year: 1405, Month: 7, Day: 1}, 0, 0, 20)       // first minute of Mehr
	save(jalali.Date{Year: 1405, Month: 7, Day: 30}, 23, 59, 300)   // last minute of Mehr
	save(jalali.Date{Year: 1405, Month: 8, Day: 1}, 0, 0, 4_000)    // first minute of Aban
	h.SendText("/report")
	if got := h.LastSent().Text; !strings.Contains(got, "Food: 320 (100%)") {
		t.Errorf("report:\n%s", got)
	}
}

func TestReportStaleOrBadButtonData(t *testing.T) {
	h := bottest.New(t)
	msg := bale.Message{MessageID: 1, Chat: bale.Chat{ID: bottest.OwnerID, Type: bale.ChatPrivate}}
	h.Tap(msg, "rep:garbage")
	h.Tap(msg, "rep:140513")
	if n := len(h.Bale.Edits()); n != 0 {
		t.Errorf("edits = %d, want none", n)
	}
	for _, a := range h.Bale.Answers() {
		if a.Text != "Unknown month" {
			t.Errorf("toast = %q", a.Text)
		}
	}
}

func TestReportIsListedInHelp(t *testing.T) {
	h := bottest.New(t)
	h.SendText("/help")
	if !strings.Contains(h.LastSent().Text, "/report") {
		t.Errorf("help:\n%s", h.LastSent().Text)
	}
}
