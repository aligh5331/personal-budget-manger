package inputrules_test

import (
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/extract"
)

// melliPOS is a Bank Melli Bale message for amountRial (Persian digits),
// without a note.
func melliPOS(amountRial string) string {
	return "اطلاع رسانی بانک ملی ایران: *بانک ملی ایران*\n" +
		"*#برداشت_با_POS*\n" +
		"مبلغ: *" + amountRial + "-* ریال\n" +
		"مانده: *۳,۳۳۳,۳۳۳* ریال\n" +
		"زمان: *۱۰:۱۵ ۱۴۰۵/۰۷/۱۲*"
}

func TestNoteAmounts(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		note      string
		amount    int64 // what the model returned
		unit      string
		wantToman int64 // 0 means the amount is missing
	}{
		// Colloquial toman: under 10,000, no multiplier, no «ریال»: ×1,000.
		{"۲۵۰ is 250,000", "نون ۲۵۰ تومن", "نون ۲۵۰ تومن", 250, extract.UnitToman, 250000},
		{"۲ is 2,000", "شارژ ۲ تومن", "شارژ ۲ تومن", 2, extract.UnitToman, 2000},
		{"۱۵۰۰ is 1,500,000, never millions", "کفش ۱۵۰۰ تومن", "کفش ۱۵۰۰ تومن", 1500, extract.UnitToman, 1500000},
		{"۵۰۰۰ is 5,000,000", "اجاره پارکینگ ۵۰۰۰", "اجاره پارکینگ ۵۰۰۰", 5000, extract.UnitToman, 5000000},
		{"۱۲۰۰۰ stays 12,000", "بستنی ۱۲۰۰۰", "بستنی ۱۲۰۰۰", 12000, extract.UnitToman, 12000},
		{"10,000 is not under 10,000", "تاکسی ۱۰۰۰۰ تومن", "تاکسی ۱۰۰۰۰ تومن", 10000, extract.UnitToman, 10000},
		{"model already multiplied", "نون ۲۵۰ تومن", "نون ۲۵۰ تومن", 250000, extract.UnitToman, 250000},
		{"model said rial for a note-only Input", "نون ۲۵۰ تومن", "", 250, extract.UnitRial, 250000},
		{"Arabic letters and separators", "تاكسي ۸۰ تومن", "", 80, extract.UnitToman, 80000},

		// Explicit multipliers apply as written.
		{"۴۵۰ هزار is 450,000", "قبض ۴۵۰ هزار تومن", "قبض ۴۵۰ هزار تومن", 450, extract.UnitToman, 450000},
		{"۴۵۰ هزار read by the model as 450000", "قبض ۴۵۰ هزار تومن", "قبض ۴۵۰ هزار تومن", 450000, extract.UnitToman, 450000},
		{"هزار joined to the number", "قبض ۴۵۰هزار", "قبض ۴۵۰هزار", 450, extract.UnitToman, 450000},
		{"۲ میلیون is 2,000,000", "اجاره ۲ میلیون", "اجاره ۲ میلیون", 2, extract.UnitToman, 2000000},
		{"۱۲ هزار is 12,000", "نون ۱۲ هزار", "نون ۱۲ هزار", 12, extract.UnitToman, 12000},
		{"k", "snack 450k", "snack 450k", 450, extract.UnitToman, 450000},
		{"K with a space", "taxi 80 K", "taxi 80 K", 80, extract.UnitToman, 80000},

		// «ریال» is read literally as rial.
		{"ریال is literal", "پارکینگ ۵۰۰۰ ریال", "پارکینگ ۵۰۰۰ ریال", 5000, extract.UnitToman, 500},
		{"ریال with a multiplier", "قبض ۴۵۰ هزار ریال", "قبض ۴۵۰ هزار ریال", 450, extract.UnitRial, 45000},

		{"amount not in the note", "نون ۲۵۰ تومن", "نون ۲۵۰ تومن", 300, extract.UnitToman, 0},

		// Bank amounts never get the colloquial rule, even if the model says toman.
		{"bank amount under 10,000 is rial", melliPOS("۵,۰۰۰"), "", 5000, extract.UnitToman, 500},
		// When note and bank disagree, the bank amount wins.
		{"note agrees with the bank", melliPOS("۲,۵۰۰,۰۰۰") + "\nنون ۲۵۰", "نون ۲۵۰", 250, extract.UnitToman, 250000},
		{"bank wins over a different note amount", melliPOS("۲,۵۰۰,۰۰۰") + "\nنون ۳۰۰", "نون ۳۰۰", 300, extract.UnitToman, 250000},
		{"bank wins over a note amount with multiplier", melliPOS("۴,۸۰۰,۰۰۰") + "\nقبض برق ۴۹۰ هزار تومن", "قبض برق ۴۹۰ هزار تومن", 490000, extract.UnitToman, 480000},
		// The colloquial rule also holds next to a bank message that has no amount.
		{"note amount when the bank prints none", baleCase13 + " ۸۵ تومن", "داروخانه ۸۵ تومن", 85, extract.UnitToman, 85000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := one(t, tc.text, tc.note, extract.Item{Amount: tc.amount, AmountUnit: tc.unit, Direction: extract.DirOut, Confidence: 0.9})
			if tc.wantToman == 0 {
				if d.HasAmount {
					t.Errorf("amount = %d, want missing", d.AmountToman)
				}
				return
			}
			if !d.HasAmount || d.AmountToman != tc.wantToman {
				t.Errorf("amount = %d (has=%v), want %d", d.AmountToman, d.HasAmount, tc.wantToman)
			}
		})
	}
}
