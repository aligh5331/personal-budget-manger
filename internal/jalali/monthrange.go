package jalali

import "time"

// MonthRange returns the half-open instant range [start, end) covering
// Jalali month m of year y in loc: start is midnight on the 1st, end is
// midnight on the 1st of the next month. Storage takes it as a plain time
// range.
func MonthRange(y, m int, loc *time.Location) (start, end time.Time) {
	ny, nm := AddMonths(y, m, 1)
	return Date{Year: y, Month: m, Day: 1}.At(0, 0, loc),
		Date{Year: ny, Month: nm, Day: 1}.At(0, 0, loc)
}

// AddMonths returns the Jalali year and month n months after (or, for
// negative n, before) month m of year y.
func AddMonths(y, m, n int) (year, month int) {
	idx := y*12 + (m - 1) + n
	return idx / 12, idx%12 + 1
}
