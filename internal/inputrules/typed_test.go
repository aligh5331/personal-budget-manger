package inputrules_test

import (
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/inputrules"
	"github.com/aligh5331/personal-budget-manger/internal/jalali"
)

func TestTypedAmount(t *testing.T) {
	tests := []struct {
		in   string
		want int64 // 0 means not an amount
	}{
		{"450000", 450000},
		{"450,000", 450000},
		{"۴۵۰٬۰۰۰", 450000},
		{"۴۵۰,۰۰۰ تومان", 450000},
		{"12000", 12000},
		{"12000 تومن", 12000},
		{"250", 250000},
		{"۲۵۰ تومن", 250000},
		{"2", 2000},
		{"1500", 1500000},
		{"۴۵۰ هزار", 450000},
		{"450k", 450000},
		{"450 K", 450000},
		{"2 میلیون", 2000000},
		{"2.5 میلیون", 2500000},
		{"1240000 ریال", 124000},
		{"1240005 rial", 124001},
		{"5000 ریال", 500},
		{"450,000 toman", 450000},
		{"", 0},
		{"0", 0},
		{"abc", 0},
		{"ناهار", 0},
		{"12 34", 0},
		{"-5000", 0},
		{"450 هزار ناهار", 0},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := inputrules.TypedAmount(tc.in)
			if tc.want == 0 {
				if ok {
					t.Errorf("TypedAmount(%q) = %d, want not an amount", tc.in, got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Errorf("TypedAmount(%q) = %d, %v, want %d", tc.in, got, ok, tc.want)
			}
		})
	}
}

func TestTypedDate(t *testing.T) {
	// now is 14 Mehr 1405, 12:00 in Tehran.
	tests := []struct {
		in   string
		want jalali.Date // zero means not a date
	}{
		{"1405/07/04", jalali.Date{Year: 1405, Month: 7, Day: 4}},
		{"1405/7/4", jalali.Date{Year: 1405, Month: 7, Day: 4}},
		{"۱۴۰۵/۰۷/۰۴", jalali.Date{Year: 1405, Month: 7, Day: 4}},
		{"1405-07-04", jalali.Date{Year: 1405, Month: 7, Day: 4}},
		{"1404/12/29", jalali.Date{Year: 1404, Month: 12, Day: 29}},
		{"07/04", jalali.Date{Year: 1405, Month: 7, Day: 4}},
		{"7/14", jalali.Date{Year: 1405, Month: 7, Day: 14}},
		{"12/01", jalali.Date{Year: 1404, Month: 12, Day: 1}}, // later this year: last year's
		{"4 Mehr 1405", jalali.Date{Year: 1405, Month: 7, Day: 4}},
		{"4 mehr", jalali.Date{Year: 1405, Month: 7, Day: 4}},
		{"29 Esfand 1404", jalali.Date{Year: 1404, Month: 12, Day: 29}},
		{"امروز", jalali.Date{Year: 1405, Month: 7, Day: 14}},
		{"دیروز", jalali.Date{Year: 1405, Month: 7, Day: 13}},
		{"پریروز", jalali.Date{Year: 1405, Month: 7, Day: 12}},
		{"today", jalali.Date{Year: 1405, Month: 7, Day: 14}},
		{"Yesterday", jalali.Date{Year: 1405, Month: 7, Day: 13}},
		{"1405/07/15", jalali.Date{}}, // tomorrow
		{"1406/01/01", jalali.Date{}},
		{"1405/13/01", jalali.Date{}},
		{"1405/07/31", jalali.Date{}}, // Mehr has 30 days
		{"32 Mehr", jalali.Date{}},
		{"4 Foo 1405", jalali.Date{}},
		{"tomorrow", jalali.Date{}},
		{"450000", jalali.Date{}},
		{"", jalali.Date{}},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := inputrules.TypedDate(tc.in, now)
			if tc.want == (jalali.Date{}) {
				if ok {
					t.Errorf("TypedDate(%q) = %v, want not a date", tc.in, got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Errorf("TypedDate(%q) = %v, %v, want %v", tc.in, got, ok, tc.want)
			}
		})
	}
}
