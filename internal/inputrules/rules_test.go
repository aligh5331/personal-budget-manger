package inputrules_test

import (
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/extract"
	"github.com/aligh5331/personal-budget-manger/internal/inputrules"
)

// one runs Apply on text with a single model item and returns its draft.
func one(t *testing.T, text, note string, it extract.Item) inputrules.Draft {
	t.Helper()
	out := inputrules.Apply(inputrules.Input{
		Text:       text,
		Now:        now,
		Extraction: extract.Result{IsTransaction: true, Note: note, Transactions: []extract.Item{it}},
	})
	if len(out.Drafts) != 1 {
		t.Fatalf("got %d drafts, want 1", len(out.Drafts))
	}
	return out.Drafts[0]
}

func TestAmounts(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		amount    int64
		unit      string
		wantToman int64 // 0 means the amount is missing
	}{
		{"bank rial to toman", baleCase1, 1240000, extract.UnitRial, 124000},
		{"bank rial rounds half up", "بانك ملي ايران\nبرداشت:1,240,005-\n0704-19:30", 1240005, extract.UnitRial, 124001},
		{"bank rial rounds down", "بانك ملي ايران\nبرداشت:1,240,004-\n0704-19:30", 1240004, extract.UnitRial, 124000},
		{"missing unit is rial", "بانك ملي ايران\nبرداشت:2,000,000-\n0707-11:58", 2000000, "", 200000},
		{"Persian digits with Arabic separator", "مبلغ: ۲۵۰٬۰۰۰- ریال", 250000, extract.UnitRial, 25000},
		{"toman from the note stays toman", "تاکسی ۸۰۰۰۰ تومن", 80000, extract.UnitToman, 80000},
		{"amount not in the text", baleCase1, 1250000, extract.UnitRial, 0},
		{"no amount from the model", baleCase1, 0, extract.UnitRial, 0},
		{"balance is never the amount", baleCase13, 16500000, extract.UnitRial, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := one(t, tc.text, "", extract.Item{Amount: tc.amount, AmountUnit: tc.unit, Direction: extract.DirOut, Confidence: 0.9})
			if tc.wantToman == 0 {
				if d.HasAmount || d.FollowUp != inputrules.FieldAmount {
					t.Errorf("got amount %d (has=%v) follow-up %q, want missing amount", d.AmountToman, d.HasAmount, d.FollowUp)
				}
				return
			}
			if !d.HasAmount || d.AmountToman != tc.wantToman {
				t.Errorf("amount = %d (has=%v), want %d", d.AmountToman, d.HasAmount, tc.wantToman)
			}
		})
	}
}

func TestDirection(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		note     string
		amount   int64
		modelDir string
		conf     float64
		want     inputrules.Direction
	}{
		{"minus sign", baleCase1, "تنقلات", 1240000, extract.DirOut, 0.9, inputrules.Out},
		{"plus sign beats the transfer word", "بانک ملی ایران\n*#انتقالي_ازكارت*\nمبلغ: *۵۰,۰۰۰,۰۰۰+* ریال\nزمان: *۰۸:۳۰ ۱۴۰۵/۰۷/۰۱*\nحقوق", "حقوق", 50000000, extract.DirIn, 0.9, inputrules.In},
		{"minus sign beats a 'from card' transfer hashtag", "بانک ملی ایران\n*#انتقال_از_كارت_-_شتاب*\nمبلغ: *۱۵,۰۱۴,۵۰۰-* ریال\nزمان: *۱۹:۰۰ ۱۴۰۵/۰۷/۰۹*\nقرض دادم به رفیقم", "", 15014500, extract.DirIn, 0.9, inputrules.Out},
		{"sign beats the model and a low confidence", "بانك ملي ايران\nانتقالي:63,000,000+\n0705-13:23", "", 63000000, extract.DirOut, 0.3, inputrules.In},
		{"sign beats the note", "بانک ملی ایران\nانتقالی:63,000,000+\n0705-13:23\nخرید", "خرید", 63000000, extract.DirOut, 0.9, inputrules.In},
		{"out keyword without sign", "بانک ملت\nبرداشت:125,000\n0703-13:45", "", 125000, extract.DirIn, 0.9, inputrules.Out},
		{"in keyword without sign", "بانک ملت\nواریز:65,433\n0703-13:45", "", 65433, extract.DirOut, 0.9, inputrules.In},
		{"bare transfer is ambiguous", "بانک پاسارگاد\nانتقال\nمبلغ:1,000,000\n07/02 10:00", "", 1000000, extract.DirOut, 0.9, inputrules.Ambiguous},
		{"conflicting keywords are ambiguous", "بانک\nبرداشت واریز:1,000,000\n07/02 10:00", "", 1000000, extract.DirOut, 0.9, inputrules.Ambiguous},
		{"note keyword when the bank has no cue", "مبلغ: 1,000,000\nواریز از علی", "واریز از علی", 1000000, extract.DirOut, 0.9, inputrules.In},
		{"model reading of the note", "حقوق ۵۰۰۰۰ تومن", "حقوق", 50000, extract.DirIn, 0.9, inputrules.In},
		{"model under 0.7 is ambiguous", "Cr 1,000,000 IRR", "", 1000000, extract.DirIn, 0.6, inputrules.Ambiguous},
		{"no cue at all defaults to expense", "نون ۸۰۰۰۰ تومن", "نون", 80000, extract.DirAmbiguous, 0.9, inputrules.Out},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := one(t, tc.text, tc.note, extract.Item{Amount: tc.amount, AmountUnit: extract.UnitRial, Direction: tc.modelDir, Confidence: tc.conf})
			if d.Direction != tc.want {
				t.Errorf("Direction = %q, want %q", d.Direction, tc.want)
			}
			wantFollow := inputrules.FieldNone
			if tc.want == inputrules.Ambiguous {
				wantFollow = inputrules.FieldDirection
			}
			if d.FollowUp != wantFollow {
				t.Errorf("FollowUp = %q, want %q", d.FollowUp, wantFollow)
			}
		})
	}
}

func TestDates(t *testing.T) {
	sms := func(dateLine string) string {
		return "بانك ملي ايران\nخريداينترنتي:4,200,000-\n" + dateLine
	}
	tests := []struct {
		name     string
		text     string
		date     string
		time     string
		want     time.Time
		wantTime bool
	}{
		{"full bank date and time", "مبلغ: *۴,۲۰۰,۰۰۰-* ریال\nزمان: *۱۲:۰۵ ۱۴۰۵/۰۷/۰۴*", "1405/07/04", "12:05", at(2026, 9, 26, 12, 5), true},
		{"year-less SMS date gets this year", sms("0704-19:30"), "1405/07/04", "19:30", at(2026, 9, 26, 19, 30), true},
		{"year-less date ignores the model's year", sms("0704-19:30"), "1404/07/04", "19:30", at(2026, 9, 26, 19, 30), true},
		{"year-less date in the future goes back a year", sms("1220-10:00"), "1405/12/20", "10:00", at(2026, 3, 11, 10, 0), true},
		{"year-less today stays this year", sms("0714-11:00"), "1405/07/14", "11:00", at(2026, 10, 6, 11, 0), true},
		{"time not printed is dropped", "مبلغ: 4,200,000-\n1405/07/04", "1405/07/04", "12:00", at(2026, 9, 26, 0, 0), false},
		{"date not printed means now", "نون 4,200,000 ریال", "1405/07/01", "09:00", now, false},
		{"no date means now", "نون 4,200,000 ریال", "", "", now, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := one(t, tc.text, "", extract.Item{Amount: 4200000, AmountUnit: extract.UnitRial, Direction: extract.DirOut, Date: tc.date, Time: tc.time, Confidence: 0.9})
			if !d.OccurredAt.Equal(tc.want) || d.HasTime != tc.wantTime {
				t.Errorf("OccurredAt = %s (time=%v), want %s (time=%v)", d.OccurredAt, d.HasTime, tc.want, tc.wantTime)
			}
		})
	}
}

func TestDescription(t *testing.T) {
	const noNote = "اطلاع رسانی بانک ملی ایران: *بانک ملی ایران*\n*#برداشت_با_POS*\nمبلغ: *۲۰۰,۰۰۰-* ریال\nزمان: *۱۰:۱۵ ۱۴۰۵/۰۷/۱۲*"
	tests := []struct {
		name      string
		text      string
		modelNote string
		want      string
	}{
		{"model note kept", baleCase1, "تنقلات", "تنقلات"},
		{"case 2: dropped note cut out after the date line", baleCase2, "", "قست بانک"},
		{"case 8: dropped note after the last of two messages", smsCase8, "", "جابجایی بین حساب هام"},
		{"case 13: dropped note on a message with no amount", baleCase13, "", "داروخانه"},
		{"case 17: note before the bank header", "کافه\n" + noNote, "", "کافه"},
		{"no note at all", noNote, "", ""},
		{"tidied note accepted after normalizing", "بانك ملي ايران\nقسط:2,000,000-\n0707-11:58\nقسط  ماشين", "قسط ماشین", "قسط ماشین"},
		{"tidied «قسط» with a ZWNJ accepted", "بانك ملي ايران\nبرداشت:2,000,000-\n0707-11:58\nقسط\u200cها", "قسط ها", "قسط ها"},
		{"made-up note rejected for the cut-out", baleCase2, "قسط وام بانک", "قست بانک"},
		{"made-up note on a note-only Input gives nothing", "نون ۸۰۰۰۰ تومن", "خرید نان", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := one(t, tc.text, tc.modelNote, extract.Item{Amount: 2000000, AmountUnit: extract.UnitRial, Direction: extract.DirOut, Confidence: 0.9})
			if d.Description != tc.want {
				t.Errorf("Description = %q, want %q", d.Description, tc.want)
			}
		})
	}
}

func TestBankLabelMustBeInTheText(t *testing.T) {
	d := one(t, baleCase1, "", extract.Item{Amount: 1240000, BankLabel: "#برداشت_با_POS", Direction: extract.DirOut, Confidence: 0.9})
	if d.BankLabel != "#برداشت_با_POS" {
		t.Errorf("BankLabel = %q", d.BankLabel)
	}
	d = one(t, baleCase1, "", extract.Item{Amount: 1240000, BankLabel: "خرید فروشگاه", Direction: extract.DirOut, Confidence: 0.9})
	if d.BankLabel != "" {
		t.Errorf("invented BankLabel kept: %q", d.BankLabel)
	}
}

func TestNotATransaction(t *testing.T) {
	out := inputrules.Apply(inputrules.Input{Text: "رمز یکبار مصرف: 12345", Now: now, Extraction: extract.Result{IsTransaction: false}})
	if out.IsTransaction || len(out.Drafts) != 0 {
		t.Errorf("got %+v", out)
	}
	out = inputrules.Apply(inputrules.Input{Text: "سلام", Now: now, Extraction: extract.Result{IsTransaction: true}})
	if out.IsTransaction {
		t.Errorf("an Input with no money movement should not be a transaction: %+v", out)
	}
}

func TestRepeatedAmountsPairWithTheirOwnMessages(t *testing.T) {
	out := inputrules.Apply(inputrules.Input{
		Text: smsCase8,
		Now:  now,
		Extraction: extract.Result{IsTransaction: true, Transactions: []extract.Item{
			{Amount: 63000000, AmountUnit: extract.UnitRial, Direction: extract.DirOut, Date: "1405/07/05", Time: "13:23", Confidence: 0.9},
			{Amount: 63000000, AmountUnit: extract.UnitRial, Direction: extract.DirIn, Date: "1405/07/05", Time: "13:23", Confidence: 0.9},
		}},
	})
	if len(out.Drafts) != 2 || out.Drafts[0].Direction != inputrules.Out || out.Drafts[1].Direction != inputrules.In {
		t.Fatalf("got %+v", out.Drafts)
	}
	for _, d := range out.Drafts {
		if d.AmountToman != 6300000 || d.Description != "جابجایی بین حساب هام" {
			t.Errorf("draft %+v", d)
		}
	}
}
