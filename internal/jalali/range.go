package jalali

import "time"

// MonthContaining returns the Jalali year and month of t's calendar day in
// t's own location (pass t.In(clock.Tehran())). Pair it with MonthRange.
func MonthContaining(t time.Time) (y, m int) {
	d := FromTime(t)
	return d.Year, d.Month
}
