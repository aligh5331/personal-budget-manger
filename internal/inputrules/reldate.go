package inputrules

import (
	"strings"
	"time"
	"unicode"
)

// relativeDays maps a day word in the Owner's note to how many days before
// today it is.
var relativeDays = map[string]int{
	"امروز":  0,
	"دیروز":  1,
	"پریروز": 2,
}

// noteDate resolves a relative date in the note («امروز», «دیروز»,
// «پریروز», also written «پری روز») to midnight of that day in now's
// location (Asia/Tehran). The note has no time of day.
func noteDate(note string, now time.Time) (time.Time, bool) {
	words := strings.FieldsFunc(Normalize(note), func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	})
	for i, w := range words {
		if w == "پری" && i+1 < len(words) && words[i+1] == "روز" {
			w = "پریروز"
		}
		if n, ok := relativeDays[w]; ok {
			y, m, day := now.Date()
			return time.Date(y, m, day-n, 0, 0, 0, 0, now.Location()), true
		}
	}
	return time.Time{}, false
}
