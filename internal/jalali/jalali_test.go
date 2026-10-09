package jalali_test

import (
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/jalali"
)

func TestFromTimeAndBack(t *testing.T) {
	teh := clock.Tehran()
	tests := []struct {
		greg time.Time
		want jalali.Date
	}{
		{time.Date(2026, 10, 6, 12, 0, 0, 0, teh), jalali.Date{Year: 1405, Month: 7, Day: 14}},
		{time.Date(2024, 3, 20, 0, 0, 0, 0, teh), jalali.Date{Year: 1403, Month: 1, Day: 1}},
		{time.Date(2025, 3, 20, 0, 0, 0, 0, teh), jalali.Date{Year: 1403, Month: 12, Day: 30}}, // 1403 is leap
		{time.Date(2025, 3, 21, 0, 0, 0, 0, teh), jalali.Date{Year: 1404, Month: 1, Day: 1}},
		{time.Date(2021, 3, 20, 0, 0, 0, 0, teh), jalali.Date{Year: 1399, Month: 12, Day: 30}}, // 1399 is leap
		{time.Date(2026, 3, 20, 0, 0, 0, 0, teh), jalali.Date{Year: 1404, Month: 12, Day: 29}},
		{time.Date(2026, 9, 22, 0, 0, 0, 0, teh), jalali.Date{Year: 1405, Month: 6, Day: 31}},
		{time.Date(2026, 9, 23, 0, 0, 0, 0, teh), jalali.Date{Year: 1405, Month: 7, Day: 1}},
	}
	for _, tc := range tests {
		if got := jalali.FromTime(tc.greg); got != tc.want {
			t.Errorf("FromTime(%s) = %v, want %v", tc.greg.Format("2006-01-02"), got, tc.want)
		}
		back := tc.want.At(0, 0, teh)
		if !back.Equal(time.Date(tc.greg.Year(), tc.greg.Month(), tc.greg.Day(), 0, 0, 0, 0, teh)) {
			t.Errorf("%v.At = %s, want %s", tc.want, back.Format("2006-01-02"), tc.greg.Format("2006-01-02"))
		}
	}
}

func TestFromTimeUsesTheTimesZone(t *testing.T) {
	// 22:00 UTC on 5 Oct 2026 is already 6 Oct (14 Mehr) in Tehran.
	utc := time.Date(2026, 10, 5, 22, 0, 0, 0, time.UTC)
	if got := jalali.FromTime(utc.In(clock.Tehran())); got != (jalali.Date{Year: 1405, Month: 7, Day: 14}) {
		t.Errorf("got %v", got)
	}
}

func TestValid(t *testing.T) {
	tests := []struct {
		d    jalali.Date
		want bool
	}{
		{jalali.Date{Year: 1405, Month: 7, Day: 30}, true},
		{jalali.Date{Year: 1405, Month: 7, Day: 31}, false},
		{jalali.Date{Year: 1405, Month: 6, Day: 31}, true},
		{jalali.Date{Year: 1403, Month: 12, Day: 30}, true},
		{jalali.Date{Year: 1404, Month: 12, Day: 30}, false},
		{jalali.Date{Year: 1405, Month: 13, Day: 1}, false},
		{jalali.Date{Year: 1405, Month: 1, Day: 0}, false},
	}
	for _, tc := range tests {
		if got := tc.d.Valid(); got != tc.want {
			t.Errorf("%v.Valid() = %v, want %v", tc.d, got, tc.want)
		}
	}
}

func TestFormat(t *testing.T) {
	teh := clock.Tehran()
	tests := []struct {
		t    time.Time
		want string
	}{
		{time.Date(2026, 10, 6, 12, 0, 0, 0, teh), "14 Mehr 1405"},
		{time.Date(2025, 3, 21, 0, 0, 0, 0, teh), "1 Farvardin 1404"},
		{time.Date(2026, 3, 20, 0, 0, 0, 0, teh), "29 Esfand 1404"},
	}
	for _, tc := range tests {
		if got := jalali.Format(tc.t); got != tc.want {
			t.Errorf("Format = %q, want %q", got, tc.want)
		}
	}
	if got := (jalali.Date{Year: 1405, Month: 7, Day: 4}).String(); got != "1405/07/04" {
		t.Errorf("String = %q", got)
	}
}
