package jalali

import (
	"testing"
	"time"
)

func TestMonthContaining(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		t.Skip("no tz database:", err)
	}
	tests := []struct {
		name string
		at   time.Time
		y, m int
	}{
		{"14 Mehr", time.Date(2026, 10, 6, 12, 0, 0, 0, loc), 1405, 7},
		{"last second of Mehr", time.Date(2026, 10, 22, 23, 59, 59, 0, loc), 1405, 7},
		{"first second of Aban", time.Date(2026, 10, 23, 0, 0, 0, 0, loc), 1405, 8},
		{"UTC evening is Tehran next day", time.Date(2026, 10, 22, 20, 30, 0, 0, time.UTC).In(loc), 1405, 8},
	}
	for _, tt := range tests {
		if y, m := MonthContaining(tt.at); y != tt.y || m != tt.m {
			t.Errorf("%s: %d/%d, want %d/%d", tt.name, y, m, tt.y, tt.m)
		}
	}
}
