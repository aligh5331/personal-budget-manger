// Package jalali converts between the Gregorian and Jalali (Solar Hijri)
// calendars and formats Jalali dates for display. It is the only place that
// knows the Jalali calendar; storage keeps UTC unix seconds.
//
// The leap-year algorithm is the 33-year-cycle "breaks" table from
// jalaali-js (Borkowski), valid for Jalali years -61 to 3177.
package jalali

import (
	"fmt"
	"time"
)

// Date is a Jalali calendar date.
type Date struct {
	Year, Month, Day int
}

var monthNames = [...]string{
	"Farvardin", "Ordibehesht", "Khordad", "Tir", "Mordad", "Shahrivar",
	"Mehr", "Aban", "Azar", "Dey", "Bahman", "Esfand",
}

// MonthName returns the English-letter name of Jalali month m (1-12).
func MonthName(m int) string {
	if m < 1 || m > 12 {
		return fmt.Sprintf("month %d", m)
	}
	return monthNames[m-1]
}

// String returns "YYYY/MM/DD".
func (d Date) String() string { return fmt.Sprintf("%04d/%02d/%02d", d.Year, d.Month, d.Day) }

// Before reports whether d is earlier than o.
func (d Date) Before(o Date) bool {
	if d.Year != o.Year {
		return d.Year < o.Year
	}
	if d.Month != o.Month {
		return d.Month < o.Month
	}
	return d.Day < o.Day
}

// Valid reports whether d is a real Jalali date.
func (d Date) Valid() bool {
	if d.Year < 1 || d.Year >= 3177 || d.Month < 1 || d.Month > 12 || d.Day < 1 {
		return false
	}
	return d.Day <= DaysInMonth(d.Year, d.Month)
}

// IsLeap reports whether Jalali year y has 366 days.
func IsLeap(y int) bool {
	leap, _, _ := jalCal(y)
	return leap == 0
}

// DaysInMonth returns the number of days in Jalali month m of year y.
func DaysInMonth(y, m int) int {
	switch {
	case m <= 6:
		return 31
	case m <= 11:
		return 30
	case IsLeap(y):
		return 30
	default:
		return 29
	}
}

// FromTime returns the Jalali date of t's calendar day in t's location.
func FromTime(t time.Time) Date {
	gy, gm, gd := t.Date()
	day := civilDay(gy, gm, gd)
	jy := gy - 621
	start := farvardin1(jy)
	if day < start {
		jy--
		start = farvardin1(jy)
	}
	doy := day - start // 0-based day of year
	if doy < 186 {
		return Date{Year: jy, Month: doy/31 + 1, Day: doy%31 + 1}
	}
	doy -= 186
	return Date{Year: jy, Month: doy/30 + 7, Day: doy%30 + 1}
}

// At returns the instant d at hour:minute in loc.
func (d Date) At(hour, minute int, loc *time.Location) time.Time {
	doy := (d.Month-1)*31 + d.Day - 1
	if d.Month > 6 {
		doy = 186 + (d.Month-7)*30 + d.Day - 1
	}
	_, gy, march := jalCal(d.Year)
	return time.Date(gy, time.March, march+doy, hour, minute, 0, 0, loc)
}

// Format returns t's date in its own location as "14 Mehr 1405".
func Format(t time.Time) string {
	d := FromTime(t)
	return fmt.Sprintf("%d %s %d", d.Day, MonthName(d.Month), d.Year)
}

// civilDay counts days since 1970-01-01 for a Gregorian date.
func civilDay(y int, m time.Month, d int) int {
	return int(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}

// farvardin1 returns civilDay of 1 Farvardin of Jalali year jy.
func farvardin1(jy int) int {
	_, gy, march := jalCal(jy)
	return civilDay(gy, time.March, march)
}

var breaks = [...]int{-61, 9, 38, 199, 426, 686, 756, 818, 1111, 1181, 1210,
	1635, 2060, 2097, 2192, 2262, 2324, 2394, 2456, 3178}

// jalCal returns, for Jalali year jy, leap (0 means a leap year), the
// Gregorian year in which it starts and the March day of 1 Farvardin.
func jalCal(jy int) (leap, gy, march int) {
	gy = jy + 621
	leapJ := -14
	jp := breaks[0]
	jump := 0
	for i := 1; i < len(breaks); i++ {
		jm := breaks[i]
		jump = jm - jp
		if jy < jm {
			break
		}
		leapJ += jump/33*8 + jump%33/4
		jp = jm
	}
	n := jy - jp
	leapJ += n/33*8 + (n%33+3)/4
	if jump%33 == 4 && jump-n == 4 {
		leapJ++
	}
	leapG := gy/4 - (gy/100+1)*3/4 - 150
	march = 20 + leapJ - leapG
	if jump-n < 6 {
		n = n - jump + (jump+4)/33*33
	}
	leap = ((n+1)%33 - 1) % 4
	if leap == -1 {
		leap = 4
	}
	return leap, gy, march
}
