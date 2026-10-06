package jalali_test

import (
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/jalali"
)

func TestMonthRange(t *testing.T) {
	teh := clock.Tehran()
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, teh) }
	tests := []struct {
		name       string
		y, m       int
		start, end time.Time
	}{
		{"Mehr 1405 (30 days)", 1405, 7, day(2026, 9, 23), day(2026, 10, 23)},
		{"Shahrivar 1405 (31 days)", 1405, 6, day(2026, 8, 23), day(2026, 9, 23)},
		{"Farvardin 1405 starts the year", 1405, 1, day(2026, 3, 21), day(2026, 4, 21)},
		{"Esfand 1404 (29 days) ends at new year", 1404, 12, day(2026, 2, 20), day(2026, 3, 21)},
		{"Esfand 1399 (leap, 30 days)", 1399, 12, day(2021, 2, 19), day(2021, 3, 21)},
	}
	for _, tc := range tests {
		start, end := jalali.MonthRange(tc.y, tc.m, teh)
		if !start.Equal(tc.start) || !end.Equal(tc.end) {
			t.Errorf("%s: got [%s, %s), want [%s, %s)", tc.name, start, end, tc.start, tc.end)
		}
		if got := jalali.FromTime(start); got != (jalali.Date{Year: tc.y, Month: tc.m, Day: 1}) {
			t.Errorf("%s: start is %v", tc.name, got)
		}
		if got := jalali.FromTime(end.Add(-time.Second)); got.Month != tc.m || got.Day != jalali.DaysInMonth(tc.y, tc.m) {
			t.Errorf("%s: last second is %v", tc.name, got)
		}
	}
}

func TestAddMonths(t *testing.T) {
	tests := []struct{ y, m, n, wantY, wantM int }{
		{1405, 7, 0, 1405, 7},
		{1405, 7, -1, 1405, 6},
		{1405, 1, -1, 1404, 12},
		{1405, 12, 1, 1406, 1},
		{1405, 7, 12, 1406, 7},
		{1405, 7, -7, 1404, 12},
		{1405, 7, -19, 1403, 12},
	}
	for _, tc := range tests {
		y, m := jalali.AddMonths(tc.y, tc.m, tc.n)
		if y != tc.wantY || m != tc.wantM {
			t.Errorf("AddMonths(%d, %d, %d) = %d/%d, want %d/%d", tc.y, tc.m, tc.n, y, m, tc.wantY, tc.wantM)
		}
	}
}
