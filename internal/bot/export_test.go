package bot_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func seedTransaction(t *testing.T, h *bottest.Harness, tx storage.Transaction) int64 {
	t.Helper()
	tx.CreatedAt = h.Clock.Now()
	id, err := h.Store.SaveTransaction(context.Background(), tx)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// exportedRows sends /export and returns the parsed CSV of the one document sent.
func exportedRows(t *testing.T, h *bottest.Harness) [][]string {
	t.Helper()
	h.SendText("/export")
	docs := h.Bale.Documents()
	if len(docs) != 1 {
		t.Fatalf("documents sent = %d, want 1", len(docs))
	}
	d := docs[0]
	if d.ChatID != bottest.OwnerID {
		t.Errorf("document chat = %d, want the Owner's chat", d.ChatID)
	}
	if !bytes.HasPrefix(d.Content, utf8BOM) {
		t.Fatalf("CSV does not start with a UTF-8 BOM: % x", d.Content[:min(3, len(d.Content))])
	}
	rows, err := csv.NewReader(bytes.NewReader(d.Content[len(utf8BOM):])).ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	return rows
}

func column(t *testing.T, header, row []string, name string) string {
	t.Helper()
	for i, h := range header {
		if h == name {
			return row[i]
		}
	}
	t.Fatalf("no column %q in header %v", name, header)
	return ""
}

func TestExportSendsEveryTransactionAsCSV(t *testing.T) {
	h := bottest.New(t)
	amount := int64(125000)
	// 13 Mehr 1405, 23:30 in Tehran is 20:00 UTC on 5 Oct 2026.
	occurred := time.Date(2026, 10, 5, 23, 30, 0, 0, bottest.Start.Location())
	seedTransaction(t, h, storage.Transaction{
		OccurredAt:  occurred,
		AmountToman: &amount,
		Direction:   storage.DirectionOut,
		Description: "ناهار",
		BankLabel:   "خرید",
		RawText:     "بانک ملی\nخرید ۱۲۵,۰۰۰ تومان\nناهار",
		InputID:     "7:0",
	})
	seedTransaction(t, h, storage.Transaction{
		OccurredAt: bottest.Start,
		Direction:  storage.DirectionOut,
		RawText:    "یه چیزی خریدم",
		InputID:    "8:0",
		Flagged:    true,
		FlagReason: storage.FlagAmountMissing,
	})

	rows := exportedRows(t, h)

	if len(rows) != 3 {
		t.Fatalf("rows = %d, want a header and 2 Transactions: %v", len(rows), rows)
	}
	header, r := rows[0], rows[1]
	want := map[string]string{
		"id":                   "1",
		"occurred_at_utc":      "2026-10-05T20:00:00Z",
		"occurred_date_jalali": "1405/07/13",
		"amount_toman":         "125000",
		"direction":            "out",
		"category_id":          "",
		"category":             "",
		"description":          "ناهار",
		"bank_label":           "خرید",
		"raw_text":             "بانک ملی\nخرید ۱۲۵,۰۰۰ تومان\nناهار",
		"input_id":             "7:0",
		"flagged":              "0",
		"flag_reason":          "",
		"categorize_pending":   "0",
		"created_at_utc":       "2026-10-06T08:30:00Z",
		"created_date_jalali":  "1405/07/14",
	}
	for name, v := range want {
		if got := column(t, header, r, name); got != v {
			t.Errorf("%s = %q, want %q", name, got, v)
		}
	}
	if got := column(t, header, rows[2], "amount_toman"); got != "" {
		t.Errorf("missing amount exported as %q, want empty", got)
	}
	if got := column(t, header, rows[2], "flag_reason"); got != "amount_missing" {
		t.Errorf("flag_reason = %q", got)
	}
}

func TestExportWithNoTransactionsSendsJustTheHeader(t *testing.T) {
	h := bottest.New(t)

	rows := exportedRows(t, h)

	if len(rows) != 1 || rows[0][0] != "id" {
		t.Fatalf("rows = %v, want only the header", rows)
	}
}

func TestExportShowsCategoryNamesIncludingArchivedOnes(t *testing.T) {
	h := bottest.New(t)
	amount := int64(1000)
	food := categoryNamed(t, h, "Food")
	rent := categoryNamed(t, h, "Rent")
	for i, cat := range []*int64{&food.ID, &rent.ID, nil} {
		direction := storage.DirectionOut
		if cat == nil {
			direction = storage.DirectionInternal
		}
		seedTransaction(t, h, storage.Transaction{
			OccurredAt: bottest.Start, AmountToman: &amount, Direction: direction,
			CategoryID: cat, RawText: "x", InputID: strconv.Itoa(i) + ":0",
		})
	}
	if _, err := h.Store.DB().Exec(`UPDATE categories SET archived = 1 WHERE name = 'Rent'`); err != nil {
		t.Fatal(err)
	}

	rows := exportedRows(t, h)

	var got []string
	for _, r := range rows[1:] {
		got = append(got, column(t, rows[0], r, "category"))
	}
	if want := []string{"Food", "Rent", ""}; !slices.Equal(got, want) {
		t.Errorf("category column = %q, want %q", got, want)
	}
	if id := column(t, rows[0], rows[1], "category_id"); id != strconv.FormatInt(food.ID, 10) {
		t.Errorf("category_id = %q", id)
	}
}
