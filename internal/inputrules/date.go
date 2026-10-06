package inputrules

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/extract"
	"github.com/aligh5331/personal-budget-manger/internal/jalali"
)

var (
	fullDateRe = regexp.MustCompile(`^(\d{2,4})/(\d{1,2})/(\d{1,2})$`)
	dayDateRe  = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})$`)
	mmddRe     = regexp.MustCompile(`^(\d{2})(\d{2})$`)
	timeRe     = regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)
)

// occurredAt resolves when the money moved.
//
// The bank date wins, but only if it is printed in the Input: the model's
// date is checked against the text. When the year is printed it is used as
// is. When only month and day are printed (Melli SMS "0704-19:30"), the year
// is the current Jalali year, or the one before if that date is still in the
// future. The time is kept only if printed. Without a bank date the
// Transaction gets now.
func occurredAt(d doc, it extract.Item, now time.Time) (time.Time, bool) {
	date, ok := bankDate(d.joined(), strings.TrimSpace(Normalize(it.Date)), jalali.FromTime(now))
	if !ok {
		return now, false
	}
	if h, m, ok := printedTime(d.joined(), strings.TrimSpace(Normalize(it.Time))); ok {
		return date.At(h, m, now.Location()), true
	}
	return date.At(0, 0, now.Location()), false
}

func bankDate(text, s string, today jalali.Date) (jalali.Date, bool) {
	var y, m, day int
	switch {
	case fullDateRe.MatchString(s):
		p := fullDateRe.FindStringSubmatch(s)
		y, m, day = atoi(p[1]), atoi(p[2]), atoi(p[3])
	case dayDateRe.MatchString(s):
		p := dayDateRe.FindStringSubmatch(s)
		m, day = atoi(p[1]), atoi(p[2])
	case mmddRe.MatchString(s):
		p := mmddRe.FindStringSubmatch(s)
		m, day = atoi(p[1]), atoi(p[2])
	default:
		return jalali.Date{}, false
	}

	if y >= 1000 && anyIn(text,
		fmt.Sprintf("%d/%02d/%02d", y, m, day),
		fmt.Sprintf("%d/%d/%d", y, m, day)) {
		dt := jalali.Date{Year: y, Month: m, Day: day}
		return dt, dt.Valid()
	}
	if !anyIn(text,
		fmt.Sprintf("%02d%02d", m, day),
		fmt.Sprintf("%02d/%02d", m, day),
		fmt.Sprintf("%d/%d", m, day)) {
		return jalali.Date{}, false // the model's date is not in the text
	}
	dt := jalali.Date{Year: today.Year, Month: m, Day: day}
	if today.Before(dt) {
		dt.Year--
	}
	return dt, dt.Valid()
}

func printedTime(text, s string) (int, int, bool) {
	p := timeRe.FindStringSubmatch(s)
	if p == nil {
		return 0, 0, false
	}
	h, m := atoi(p[1]), atoi(p[2])
	if h > 23 || m > 59 || !anyIn(text, fmt.Sprintf("%02d:%02d", h, m), fmt.Sprintf("%d:%02d", h, m)) {
		return 0, 0, false
	}
	return h, m, true
}

func anyIn(text string, subs ...string) bool {
	for _, s := range subs {
		if strings.Contains(text, s) {
			return true
		}
	}
	return false
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
