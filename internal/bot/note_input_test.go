package bot_test

import (
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/extract"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

func TestNoteOnlyInputUsesColloquialToman(t *testing.T) {
	// Sample case 14: a cash payment with no bank message.
	h := bottest.New(t)
	const input = "امروز ۲۵۰ تومن نون خریدم"
	h.Extractor.Return(extract.Result{
		IsTransaction: true,
		Note:          input,
		Transactions: []extract.Item{{
			Amount: 250, AmountUnit: extract.UnitToman, Direction: extract.DirOut,
			Date: "1405/07/14", Confidence: 0.9,
		}},
	})

	h.SendText(input)

	conf := h.LastSent()
	assertContains(t, conf.Text, "250,000 toman", "Expense", "14 Mehr 1405")
	tx := storedTransaction(t, h, savedID(t, conf))
	if tx.AmountToman == nil || *tx.AmountToman != 250000 {
		t.Errorf("amount_toman = %v, want 250000", tx.AmountToman)
	}
	if want := time.Date(2026, 10, 6, 0, 0, 0, 0, clock.Tehran()); !tx.OccurredAt.Equal(want) {
		t.Errorf("occurred_at = %s, want %s", tx.OccurredAt, want)
	}
	if tx.Direction != storage.DirectionOut || tx.Flagged || tx.Description != "امروز 250 تومن نون خریدم" {
		t.Errorf("direction %q flagged %v description %q", tx.Direction, tx.Flagged, tx.Description)
	}
}

func TestNoteWithYesterdayIsDatedYesterday(t *testing.T) {
	h := bottest.New(t)
	const input = "تاکسی دیروز ۸۰ تومن"
	h.Extractor.Return(extract.Result{
		IsTransaction: true,
		Note:          input,
		Transactions:  []extract.Item{{Amount: 80, AmountUnit: extract.UnitToman, Direction: extract.DirOut, Confidence: 0.9}},
	})

	h.SendText(input)

	conf := h.LastSent()
	assertContains(t, conf.Text, "80,000 toman", "13 Mehr 1405")
	tx := storedTransaction(t, h, savedID(t, conf))
	if want := time.Date(2026, 10, 5, 0, 0, 0, 0, clock.Tehran()); !tx.OccurredAt.Equal(want) || *tx.AmountToman != 80000 {
		t.Errorf("occurred_at %s amount %v, want %s and 80000", tx.OccurredAt, tx.AmountToman, want)
	}
}
