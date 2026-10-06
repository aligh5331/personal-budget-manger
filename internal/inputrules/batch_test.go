package inputrules_test

import (
	"strings"
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/extract"
	"github.com/aligh5331/personal-budget-manger/internal/inputrules"
)

// case8Items is the model's reading of smsCase8: the same amount out and in.
func case8Items(inTime string) []extract.Item {
	return []extract.Item{
		{Amount: 63000000, AmountUnit: extract.UnitRial, Direction: extract.DirOut, Date: "07/05", Time: "13:23", BankLabel: "انتقالي", Confidence: 0.9},
		{Amount: 63000000, AmountUnit: extract.UnitRial, Direction: extract.DirIn, Date: "07/05", Time: inTime, BankLabel: "انتقالي", Confidence: 0.9},
	}
}

func TestAnEqualOutInPairAtTheSameMinuteIsOneInternalTransfer(t *testing.T) {
	out := inputrules.Apply(inputrules.Input{
		Text:       smsCase8,
		Now:        now,
		Extraction: extract.Result{IsTransaction: true, Note: "جابجایی بین حساب هام", Transactions: case8Items("13:23")},
	})
	if len(out.Drafts) != 1 {
		t.Fatalf("got %d drafts, want one internal transfer: %+v", len(out.Drafts), out.Drafts)
	}
	d := out.Drafts[0]
	if d.Direction != inputrules.Internal || d.AmountToman != 6300000 || !d.HasAmount || d.FollowUp != inputrules.FieldNone {
		t.Errorf("draft %+v, want internal 6,300,000 toman with no Follow-up", d)
	}
	if !d.OccurredAt.Equal(at(2026, 9, 27, 13, 23)) || !d.HasTime {
		t.Errorf("occurred_at %s (has time %v), want 5 Mehr 1405 13:23", d.OccurredAt, d.HasTime)
	}
	if d.Description != "جابجایی بین حساب هام" {
		t.Errorf("description = %q", d.Description)
	}
}

func TestPairsThatAreNotTheSameTransferStaySeparate(t *testing.T) {
	differentMinute := strings.Replace(smsCase8, "0705-13:23\nجابجایی", "0705-13:24\nجابجایی", 1)
	differentAmount := strings.Replace(smsCase8, "63,000,000+", "62,000,000+", 1)
	differentAmountItems := case8Items("13:23")
	differentAmountItems[1].Amount = 62000000
	bothOut := strings.Replace(smsCase8, "63,000,000+", "63,000,000-", 1)
	bothOutItems := case8Items("13:23")
	bothOutItems[1].Direction = extract.DirOut
	noTime := "بانك ملي ايران\nبرداشت:63,000,000-\n0705\n\nبانك ملي ايران\nواریز:63,000,000+\n0705"
	noTimeItems := case8Items("")
	noTimeItems[0].Time = ""

	tests := []struct {
		name  string
		text  string
		items []extract.Item
	}{
		{"different minute", differentMinute, case8Items("13:24")},
		{"different amount", differentAmount, differentAmountItems},
		{"both out", bothOut, bothOutItems},
		{"no bank time", noTime, noTimeItems},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := inputrules.Apply(inputrules.Input{Text: tc.text, Now: now, Extraction: extract.Result{IsTransaction: true, Transactions: tc.items}})
			if len(out.Drafts) != 2 {
				t.Fatalf("got %d drafts, want 2: %+v", len(out.Drafts), out.Drafts)
			}
			for _, d := range out.Drafts {
				if d.Direction == inputrules.Internal {
					t.Errorf("draft merged into an internal transfer: %+v", d)
				}
			}
		})
	}
}

func TestOnlyTheMatchingPairMergesInABatch(t *testing.T) {
	text := "بانك ملي ايران\nبرداشت:1,500,000-\n0711-12:00\n\n" + smsCase8
	items := append([]extract.Item{{Amount: 1500000, AmountUnit: extract.UnitRial, Direction: extract.DirOut, Date: "07/11", Time: "12:00", Confidence: 0.9}}, case8Items("13:23")...)

	out := inputrules.Apply(inputrules.Input{Text: text, Now: now, Extraction: extract.Result{IsTransaction: true, Transactions: items}})

	if len(out.Drafts) != 2 {
		t.Fatalf("got %d drafts, want 2: %+v", len(out.Drafts), out.Drafts)
	}
	if d := out.Drafts[0]; d.Direction != inputrules.Out || d.AmountToman != 150000 {
		t.Errorf("first draft %+v, want the 150,000 toman expense", d)
	}
	if d := out.Drafts[1]; d.Direction != inputrules.Internal || d.AmountToman != 6300000 {
		t.Errorf("second draft %+v, want the internal transfer", d)
	}
}

func TestMoreThanFiveBankMessagesAreRefused(t *testing.T) {
	msg := "بانك ملي ايران\nبرداشت:1,500,000-\n0711-12:00"
	item := extract.Item{Amount: 1500000, AmountUnit: extract.UnitRial, Direction: extract.DirOut, Date: "07/11", Time: "12:00", Confidence: 0.9}
	for _, n := range []int{5, 6} {
		texts := make([]string, n)
		items := make([]extract.Item, n)
		for i := range n {
			texts[i], items[i] = msg, item
		}
		out := inputrules.Apply(inputrules.Input{
			Text:       strings.Join(texts, "\n\n"),
			Now:        now,
			Extraction: extract.Result{IsTransaction: true, Transactions: items},
		})
		if n <= inputrules.MaxBankMessages {
			if out.TooMany || len(out.Drafts) != n {
				t.Errorf("%d messages: too many %v, %d drafts; want %d drafts", n, out.TooMany, len(out.Drafts), n)
			}
			continue
		}
		if !out.TooMany || len(out.Drafts) != 0 {
			t.Errorf("%d messages: too many %v, %d drafts; want refused with no drafts", n, out.TooMany, len(out.Drafts))
		}
	}
}
