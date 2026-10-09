package inputrules

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/jalali"
)

// Typed values: an amount or a date the Owner types as the whole of a reply
// (an edit, a Follow-up answer). Unlike Apply there is no model reading to
// check; the text itself must be the value and nothing else.

const arabicDecimalSep = 0x066B // ٫

var typedAmountRe = regexp.MustCompile(
	`^(\d{1,3}(?:,\d{3})+|\d{1,15})(?:\.(\d{1,3}))?\s*(k|thousand|هزار|m|million|میلیون)?\s*(تومان|تومن|toman|ریال|rial)?$`)

// TypedAmount reads an amount in toman, the way note amounts are read:
// toman unless it says «ریال» (rial, ÷10 rounded); «هزار»/k and «میلیون»/m
// apply as written; a bare amount under 10,000 means thousands (250 is
// 250,000). The text must hold only the amount.
func TypedAmount(s string) (int64, bool) {
	s = strings.ToLower(strings.TrimSpace(Normalize(s)))
	s = strings.ReplaceAll(s, string(rune(arabicDecimalSep)), ".")
	p := typedAmountRe.FindStringSubmatch(s)
	if p == nil {
		return 0, false
	}
	whole, err := strconv.ParseInt(strings.ReplaceAll(p[1], ",", ""), 10, 64)
	if err != nil {
		return 0, false
	}
	frac, mult, unit := p[2], p[3], p[4]
	var m int64 = 1
	switch mult {
	case "k", "thousand", "هزار":
		m = 1_000
	case "m", "million", "میلیون":
		m = 1_000_000
	}
	if frac != "" && m == 1 {
		return 0, false // 2.5 toman: not a whole amount
	}
	n := whole * m
	if frac != "" {
		f, _ := strconv.ParseInt(frac, 10, 64)
		scale := int64(1)
		for range frac {
			scale *= 10
		}
		if f*m%scale != 0 {
			return 0, false
		}
		n += f * m / scale
	}
	rial := unit == "ریال" || unit == "rial"
	switch {
	case rial:
		n = (n + 5) / 10
	case mult == "" && n < 10_000:
		n *= 1_000
	}
	return n, n > 0
}

var (
	typedFullDateRe  = regexp.MustCompile(`^(\d{4})[/-](\d{1,2})[/-](\d{1,2})$`)
	typedMonthDayRe  = regexp.MustCompile(`^(\d{1,2})[/-](\d{1,2})$`)
	typedNamedDateRe = regexp.MustCompile(`^(\d{1,2})\s+([a-z]+)(?:\s+(\d{4}))?$`)
)

// TypedDate reads a Jalali date: "1405/07/04", "07/04", "4 Mehr 1405",
// "4 Mehr", or «امروز» / «دیروز» / «پریروز» (today, yesterday). Without a
// year it is the current Jalali year, or last year if that date is still to
// come. Dates after today (in Asia/Tehran) are refused.
func TypedDate(s string, now time.Time) (jalali.Date, bool) {
	s = strings.ToLower(strings.TrimSpace(Normalize(s)))
	now = now.In(clock.Tehran())
	today := jalali.FromTime(now)
	switch s {
	case "امروز", "today":
		return today, true
	case "دیروز", "yesterday":
		return jalali.FromTime(now.AddDate(0, 0, -1)), true
	case "پریروز":
		return jalali.FromTime(now.AddDate(0, 0, -2)), true
	}

	var d jalali.Date
	hasYear := false
	switch {
	case typedFullDateRe.MatchString(s):
		p := typedFullDateRe.FindStringSubmatch(s)
		d, hasYear = jalali.Date{Year: atoi(p[1]), Month: atoi(p[2]), Day: atoi(p[3])}, true
	case typedMonthDayRe.MatchString(s):
		p := typedMonthDayRe.FindStringSubmatch(s)
		d = jalali.Date{Month: atoi(p[1]), Day: atoi(p[2])}
	case typedNamedDateRe.MatchString(s):
		p := typedNamedDateRe.FindStringSubmatch(s)
		d = jalali.Date{Month: monthNumber(p[2]), Day: atoi(p[1])}
		if p[3] != "" {
			d.Year, hasYear = atoi(p[3]), true
		}
	default:
		return jalali.Date{}, false
	}
	if !hasYear {
		d.Year = today.Year
		if today.Before(d) {
			d.Year--
		}
	}
	if !d.Valid() || today.Before(d) {
		return jalali.Date{}, false
	}
	return d, true
}

// monthNumber returns the Jalali month named name (any case), or 0.
func monthNumber(name string) int {
	for m := 1; m <= 12; m++ {
		if strings.EqualFold(jalali.MonthName(m), name) {
			return m
		}
	}
	return 0
}
