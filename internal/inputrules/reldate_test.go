package inputrules_test

import (
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/extract"
	"github.com/aligh5331/personal-budget-manger/internal/inputrules"
)

func TestRelativeDates(t *testing.T) {
	// 14 Mehr 1405 is 6 Oct 2026. Just after midnight in Tehran it is still
	// 5 Oct in UTC.
	justAfterMidnight := time.Date(2026, 10, 5, 20, 45, 0, 0, time.UTC)
	tests := []struct {
		name string
		text string
		now  time.Time
		date string // the model's date
		want time.Time
	}{
		{"امروز is today", "امروز ۲۵۰ تومن نون خریدم", now, "1405/07/14", at(2026, 10, 6, 0, 0)},
		{"دیروز is yesterday", "تاکسی دیروز ۸۰ تومن", now, "1405/07/13", at(2026, 10, 5, 0, 0)},
		{"پریروز is two days ago", "پریروز نون ۲۰ تومن", now, "", at(2026, 10, 4, 0, 0)},
		{"پری روز written apart", "پری" + string(rune(0x200C)) + "روز نون ۲۰ تومن", now, "", at(2026, 10, 4, 0, 0)},
		{"دیروز across a month boundary", "دیروز نون ۲۰ تومن", at(2026, 9, 23, 9, 0), "", at(2026, 9, 22, 0, 0)}, // 1 Mehr -> 31 Shahrivar
		{"yesterday is taken in Tehran", "دیروز نون ۲۰ تومن", justAfterMidnight, "", at(2026, 10, 5, 0, 0)},
		{"no date means now", "نون ۲۰ تومن", now, "", now},
		{"the bank date beats the note", melliPOS("۲۰۰,۰۰۰") + "\nدیروز", now, "1405/07/12", at(2026, 10, 4, 0, 0)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := inputrules.Apply(inputrules.Input{
				Text: tc.text,
				Now:  tc.now,
				Extraction: extract.Result{IsTransaction: true, Transactions: []extract.Item{
					{Amount: 20, AmountUnit: extract.UnitToman, Direction: extract.DirOut, Date: tc.date, Confidence: 0.9},
				}},
			})
			if len(out.Drafts) != 1 {
				t.Fatalf("got %d drafts", len(out.Drafts))
			}
			d := out.Drafts[0]
			if !d.OccurredAt.Equal(tc.want) || d.HasTime {
				t.Errorf("OccurredAt = %s (time=%v), want %s with no time", d.OccurredAt, d.HasTime, tc.want)
			}
		})
	}
}
