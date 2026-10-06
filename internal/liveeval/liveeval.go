// Package liveeval holds the pure half of the opt-in live evaluation (#47):
// reading the gitignored samples file in the research harness's format and
// scoring a pipeline run against each case's expect: line. The tagged test
// in live_test.go supplies the real Extractor, Input rules and Categorizer.
package liveeval

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/aligh5331/personal-budget-manger/internal/inputrules"
)

// Today is the Jalali day the samples were written on; case 14 depends on it.
const Today = "1405/07/13"

// Case is one sample: the Input as the Owner would send it, and the
// expect: line (without its prefix).
type Case struct {
	N      int
	Input  string
	Expect string
}

var (
	delimRe = regexp.MustCompile(`(?m)^=== (\d+) ===[ \t]*\n`)
	digitRe = regexp.MustCompile(`\d{4,}`)
	dateRe  = regexp.MustCompile(`\d{4}/\d{2}/\d{2}`)
)

// ParseSamples reads the samples file format: free-text header, then
// "=== NN ===" delimited bodies, each ending in one "expect:" line. The
// Input is the body without the expect: line, trimmed.
func ParseSamples(text string) []Case {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	marks := delimRe.FindAllStringSubmatchIndex(text, -1)
	var cases []Case
	for i, m := range marks {
		end := len(text)
		if i+1 < len(marks) {
			end = marks[i+1][0]
		}
		n, _ := strconv.Atoi(text[m[2]:m[3]])
		var body []string
		expect := ""
		for _, ln := range strings.Split(text[m[1]:end], "\n") {
			if strings.HasPrefix(ln, "expect:") {
				expect = strings.TrimSpace(strings.TrimPrefix(ln, "expect:"))
			} else {
				body = append(body, ln)
			}
		}
		cases = append(cases, Case{N: n, Input: strings.TrimSpace(strings.Join(body, "\n")), Expect: expect})
	}
	return cases
}

// Want is what a case's expect: line asks for.
type Want struct {
	// AmountsToman has one entry per Transaction; a nil entry means the
	// amount is missing (UNKNOWN).
	AmountsToman []*int64
	// Directions are the acceptable Directions of every Transaction.
	Directions []string
	// Date is Jalali YYYY/MM/DD.
	Date string
	// Categories are the acceptable Category keys (lower-case, underscores);
	// the key "" means none (internal). Empty means no check.
	Categories []string
	// Description is the Owner's note, "" when there is none.
	Description string
}

// ParseWant reads "amount | direction | category | date" (rial amounts).
func ParseWant(c Case) (Want, error) {
	parts := strings.Split(c.Expect, "|")
	if len(parts) < 4 {
		return Want{}, fmt.Errorf("case %d: expect needs 4 fields, got %q", c.N, c.Expect)
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	w := Want{}
	if strings.Contains(parts[0], "UNKNOWN") {
		w.AmountsToman = []*int64{nil}
	} else {
		for _, m := range digitRe.FindAllString(parts[0], -1) {
			rial, _ := strconv.ParseInt(m, 10, 64)
			toman := rial / 10
			w.AmountsToman = append(w.AmountsToman, &toman)
		}
	}
	d := strings.TrimRight(parts[1], "?")
	w.Directions = []string{d}
	if strings.HasSuffix(parts[1], "?") && d == "out" {
		// Uncertain (case 13: no amount line): a Follow-up may be asked.
		w.Directions = []string{"out", "ambiguous"}
	}
	w.Date = dateRe.FindString(parts[3])
	if w.Date == "" {
		return Want{}, fmt.Errorf("case %d: no date in %q", c.N, parts[3])
	}
	w.Categories = AcceptableCategories[c.N]
	w.Description = ExpectedNote(c.Input)
	return w, nil
}

// AcceptableCategories is the research harness's per-case table (EXP_CAT).
// Field 2 of expect: is free text, so it is not parsed. "" is internal.
var AcceptableCategories = map[int][]string{
	1: {"snacks"}, 2: {"loan_installment"}, 3: {"food"}, 4: {"education", "entertainment"},
	5: {"uncategorized", "shopping", "other", "food", "snacks", "groceries"},
	6: {"cash_withdrawal"}, 7: {"salary"}, 8: {""}, 9: {"transport"}, 10: {"fuel"}, 11: {"food"},
	12: {"entertainment"}, 13: {"health"}, 14: {"groceries", "food"}, 15: {"bills"}, 16: {"gifts"},
	17: {"snacks", "food"}, 18: {"other", "uncategorized"},
}

// ExpectedNote derives the Owner's note from an Input: every line outside a
// bank message. A bank message starts at a Melli header line and ends at its
// time line (or a blank line). Lines are joined with a space.
func ExpectedNote(input string) string {
	var note []string
	inBank := false
	for _, ln := range strings.Split(input, "\n") {
		trim := strings.TrimSpace(inputrules.Normalize(ln))
		switch {
		case trim == "":
			inBank = false
		case isBankHeader(trim):
			inBank = true
		case inBank:
			if isTimeLine(trim) {
				inBank = false
			}
		default:
			note = append(note, trim)
		}
	}
	return strings.Join(note, " ")
}

var smsTimeRe = regexp.MustCompile(`^\d{4}-\d{2}:\d{2}$`)

func isBankHeader(s string) bool {
	s = strings.ReplaceAll(s, "\u200c", " ")
	return strings.HasPrefix(s, "بانک ملی") || strings.HasPrefix(s, "اطلاع")
}

func isTimeLine(s string) bool {
	return smsTimeRe.MatchString(s) || strings.HasPrefix(s, "زمان")
}

// Got is what the pipeline produced for one case, one entry per Transaction.
type Got struct {
	IsTransaction bool
	// AmountsToman: nil entry means no amount.
	AmountsToman []*int64
	Directions   []string
	Dates        []string
	// Categories are lower-case keys; "" is none (internal).
	Categories   []string
	Descriptions []string
}

// Verdict is the per-field outcome of one case; Fails explains each miss.
type Verdict struct {
	Amount, Direction, Date, Category, Description bool
	Fails                                          []string
}

// OK is true when the four hard fields pass (description is reported only).
func (v Verdict) OK() bool { return v.Amount && v.Direction && v.Date && v.Category }

// Score compares got with want.
func Score(want Want, got Got) Verdict {
	v := Verdict{Amount: true, Direction: true, Date: true, Category: true, Description: true}
	fail := func(f *bool, format string, a ...any) {
		*f = false
		v.Fails = append(v.Fails, fmt.Sprintf(format, a...))
	}
	if !got.IsTransaction {
		fail(&v.Amount, "is_transaction=false")
	}
	if !slices.EqualFunc(got.AmountsToman, want.AmountsToman, func(a, b *int64) bool {
		return (a == nil) == (b == nil) && (a == nil || *a == *b)
	}) {
		fail(&v.Amount, "amount %s != %s", fmtAmounts(got.AmountsToman), fmtAmounts(want.AmountsToman))
	}
	if len(got.Directions) == 0 || !subset(got.Directions, want.Directions) {
		fail(&v.Direction, "direction %v not in %v", got.Directions, want.Directions)
	}
	if len(got.Dates) == 0 || slices.ContainsFunc(got.Dates, func(d string) bool { return d != want.Date }) {
		fail(&v.Date, "date %v != %s", got.Dates, want.Date)
	}
	if len(want.Categories) > 0 {
		if len(got.Categories) == 0 {
			fail(&v.Category, "no category")
		}
		for _, c := range got.Categories {
			if !slices.Contains(want.Categories, c) {
				fail(&v.Category, "category %q not in %v", c, want.Categories)
			}
		}
	}
	if len(got.Descriptions) == 0 {
		fail(&v.Description, "no description")
	}
	for _, d := range got.Descriptions {
		if !describes(d, want.Description) {
			fail(&v.Description, "description %q vs note %q", d, want.Description)
		}
	}
	return v
}

func subset(got, ok []string) bool {
	for _, g := range got {
		if !slices.Contains(ok, g) {
			return false
		}
	}
	return true
}

// describes: the produced description contains the expected note (and is
// empty when there is none), ignoring spacing and Arabic letter variants.
func describes(got, want string) bool {
	g := squash(got)
	if want == "" {
		return g == ""
	}
	return strings.Contains(g, squash(want))
}

func squash(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(inputrules.Normalize(s), "\u200c", " ")), " ")
}

func fmtAmounts(a []*int64) string {
	parts := make([]string, len(a))
	for i, p := range a {
		if p == nil {
			parts[i] = "none"
		} else {
			parts[i] = strconv.FormatInt(*p, 10)
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}
