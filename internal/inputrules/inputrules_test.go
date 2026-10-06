package inputrules_test

import (
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/extract"
	"github.com/aligh5331/personal-budget-manger/internal/inputrules"
)

// now is 14 Mehr 1405, 12:00 in Tehran.
var now = time.Date(2026, 10, 6, 12, 0, 0, 0, clock.Tehran())

// at returns a Tehran time for a Jalali Mehr/other date given as Gregorian.
func at(y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, clock.Tehran())
}

// Sanitized fixtures in the real Bank Melli formats (made-up balances).
const (
	baleCase1 = "اطلاع\u200cرسانی بانک ملّی ایران: *بانک ملّی ایران*\n" +
		"*#برداشت_با_POS*\n" +
		"مبلغ: *۱,۲۴۰,۰۰۰-* ریال\n" +
		"مانده: *۱۱,۱۱۱,۱۱۱* ریال\n" +
		"زمان: *۱۲:۰۵ ۱۴۰۵/۰۷/۰۴*\n" +
		"تنقلات"

	// Sample case 2: a one-line note with a typo right after the message.
	baleCase2 = "اطلاع\u200cرسانی بانک ملّی ایران: *بانک ملّی ایران*\n" +
		"*#برداشت_قسط*\n" +
		"مبلغ: *۲۰,۰۵۱,۰۶۱-* ریال\n" +
		"مانده: *۲,۲۲۲,۲۲۲* ریال\n" +
		"زمان: *۱۴:۱۱ ۱۴۰۵/۰۷/۱۲*\n" +
		"قست بانک"

	// Sample case 8: an internal pair in SMS format with one note.
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

	// Sample case 13: no amount line, only the balance.
	baleCase13 = "اطلاع\u200cرسانی بانک ملّی ایران: *بانک ملّی ایران*\n" +
		"*#برداشت_با_POS*\n" +
		"مانده: *۱۶,۵۰۰,۰۰۰* ریال\n" +
		"زمان: *۱۵:۳۰ ۱۴۰۵/۰۷/۱۰*\n" +
		"داروخانه"
)

func TestNormalize(t *testing.T) {
	tests := []struct{ in, want string }{
		{"۱۲۳٬۴۵۶", "123,456"},
		{"٠١٢٣٤٥٦٧٨٩", "0123456789"},
		{"بانك ملي", "بانک ملی"},
		{"ملّی", "ملی"},
		{"اطلاع\u200cرسانی", "اطلاع رسانی"},
		{"  قسط   ماشین \t", "قسط ماشین"},
		{"a\u200fb\u202ac", "abc"},
		{"line1\r\n  line2 ", "line1\nline2"},
	}
	for _, tc := range tests {
		if got := inputrules.Normalize(tc.in); got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBankMessageWithNote(t *testing.T) {
	out := inputrules.Apply(inputrules.Input{
		Text: baleCase1,
		Now:  now,
		Extraction: extract.Result{
			IsTransaction: true,
			Note:          "تنقلات",
			Transactions: []extract.Item{{
				Amount: 1240000, AmountUnit: extract.UnitRial, Direction: extract.DirOut,
				Date: "1405/07/04", Time: "12:05", BankLabel: "#برداشت_با_POS", Confidence: 0.95,
			}},
		},
	})
	if !out.IsTransaction || len(out.Drafts) != 1 {
		t.Fatalf("got %+v", out)
	}
	d := out.Drafts[0]
	want := inputrules.Draft{
		AmountToman: 124000, HasAmount: true,
		Direction:   inputrules.Out,
		OccurredAt:  at(2026, 9, 26, 12, 5),
		HasTime:     true,
		Description: "تنقلات",
		BankLabel:   "#برداشت_با_POS",
	}
	if !d.OccurredAt.Equal(want.OccurredAt) {
		t.Errorf("OccurredAt = %s, want %s", d.OccurredAt, want.OccurredAt)
	}
	d.OccurredAt = want.OccurredAt
	if d != want {
		t.Errorf("draft = %+v\nwant    %+v", d, want)
	}
}
