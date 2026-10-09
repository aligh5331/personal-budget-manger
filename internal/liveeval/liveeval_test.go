package liveeval_test

import (
	"strings"
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/liveeval"
)

// A tiny invented fixture in the samples file format (not real data).
const fixture = `header line, ignored
Today when written: 1405/07/13

=== 01 ===
اطلاعرسانی بانک ملّی ایران: *بانک ملّی ایران*
*#خرید*
مبلغ: *۱,۰۰۰,۰۰۰-* ریال
مانده: *۹,۰۰۰,۰۰۰* ریال
زمان: *۱۰:۳۰ ۱۴۰۵/۰۷/۱۰*
ناهار
expect: 1000000 | out | food | 1405/07/10

=== 02 ===
۲۵۰ تومن نان
expect: 2500000 | out | groceries | 1405/07/13   (note only)

=== 03 ===
بانك ملي ايران
برداشت:500,000-
مانده:9,000,000
0713-10:00

بانك ملي ايران
برداشت:700,000+
مانده:9,000,000
0713-10:01
expect: 500000 + 700000 | in | (x) | 1405/07/13

=== 04 ===
بانك ملي ايران
مانده:9,000,000
0713-10:00
expect: UNKNOWN amount (line missing) | out? | health | 1405/07/13
`

func TestParseSamplesReadsCasesAndDropsExpect(t *testing.T) {
	cases := liveeval.ParseSamples(fixture)
	if len(cases) != 4 {
		t.Fatalf("got %d cases, want 4", len(cases))
	}
	if cases[0].N != 1 || cases[3].N != 4 {
		t.Errorf("case numbers %d, %d", cases[0].N, cases[3].N)
	}
	if strings.Contains(cases[0].Input, "expect:") || strings.HasSuffix(cases[0].Input, "\n") {
		t.Errorf("input not cleaned: %q", cases[0].Input)
	}
	if !strings.HasSuffix(cases[0].Input, "ناهار") {
		t.Errorf("input lost its note: %q", cases[0].Input)
	}
	if cases[1].Expect != "2500000 | out | groceries | 1405/07/13   (note only)" {
		t.Errorf("expect = %q", cases[1].Expect)
	}
}

func TestParseSamplesToleratesCRLF(t *testing.T) {
	cases := liveeval.ParseSamples(strings.ReplaceAll(fixture, "\n", "\r\n"))
	if len(cases) != 4 || cases[1].Input != "۲۵۰ تومن نان" {
		t.Fatalf("crlf parse: %d cases, %q", len(cases), cases[1].Input)
	}
}

func TestParseWantConvertsRialToTomanAndReadsDate(t *testing.T) {
	cases := liveeval.ParseSamples(fixture)
	w, err := liveeval.ParseWant(cases[2])
	if err != nil {
		t.Fatal(err)
	}
	if len(w.AmountsToman) != 2 || *w.AmountsToman[0] != 50000 || *w.AmountsToman[1] != 70000 {
		t.Errorf("amounts = %v", w.AmountsToman)
	}
	if w.Date != "1405/07/13" || w.Directions[0] != "in" {
		t.Errorf("date %q directions %v", w.Date, w.Directions)
	}
}

func TestParseWantUnknownAmountAndUncertainDirection(t *testing.T) {
	w, err := liveeval.ParseWant(liveeval.ParseSamples(fixture)[3])
	if err != nil {
		t.Fatal(err)
	}
	if len(w.AmountsToman) != 1 || w.AmountsToman[0] != nil {
		t.Errorf("amounts = %v", w.AmountsToman)
	}
	if len(w.Directions) != 2 {
		t.Errorf("directions = %v, want out or ambiguous", w.Directions)
	}
}

func TestParseWantRejectsAShortExpectLine(t *testing.T) {
	if _, err := liveeval.ParseWant(liveeval.Case{N: 1, Expect: "1000 | out"}); err == nil {
		t.Error("want an error")
	}
}

func TestExpectedNoteIsEverythingOutsideBankMessages(t *testing.T) {
	cases := liveeval.ParseSamples(fixture)
	want := []string{"ناهار", "250 تومن نان", "", ""}
	for i, c := range cases {
		got := liveeval.ExpectedNote(c.Input)
		if got != want[i] {
			t.Errorf("case %d: note %q", c.N, got)
		}
	}
	if got := liveeval.ExpectedNote("ورزش\nاطلاعرسانی بانک x\nمبلغ: 1\nزمان: 2"); got != "ورزش" {
		t.Errorf("note before the message: %q", got)
	}
}

func i64(n int64) *int64 { return &n }

func TestScoreMarksEachField(t *testing.T) {
	want := liveeval.Want{
		AmountsToman: []*int64{i64(100000)},
		Directions:   []string{"out"},
		Date:         "1405/07/10",
		Categories:   []string{"food", "snacks"},
		Description:  "ناهار",
	}
	good := liveeval.Got{
		IsTransaction: true, AmountsToman: []*int64{i64(100000)}, Directions: []string{"out"},
		Dates: []string{"1405/07/10"}, Categories: []string{"food"}, Descriptions: []string{"ناهار  با دوستان"},
	}
	if v := liveeval.Score(want, good); !v.OK() || !v.Description || len(v.Fails) != 0 {
		t.Fatalf("good run failed: %+v", v)
	}

	bad := good
	bad.AmountsToman = []*int64{i64(10000)}
	bad.Directions = []string{"in"}
	bad.Dates = []string{"1405/07/11"}
	bad.Categories = []string{"fuel"}
	bad.Descriptions = []string{""}
	v := liveeval.Score(want, bad)
	if v.Amount || v.Direction || v.Date || v.Category || v.Description || len(v.Fails) != 5 {
		t.Errorf("bad run: %+v", v)
	}
}

func TestScoreNilAmountMatchesUnknownOnly(t *testing.T) {
	want := liveeval.Want{AmountsToman: []*int64{nil}, Directions: []string{"out", "ambiguous"}, Date: "d"}
	got := liveeval.Got{IsTransaction: true, AmountsToman: []*int64{nil}, Directions: []string{"ambiguous"}, Dates: []string{"d"}, Descriptions: []string{""}}
	if v := liveeval.Score(want, got); !v.OK() {
		t.Errorf("unknown amount should match: %+v", v)
	}
	got.AmountsToman = []*int64{i64(5)}
	if v := liveeval.Score(want, got); v.Amount {
		t.Error("a produced amount must not match UNKNOWN")
	}
}

func TestScoreEmptyRunFails(t *testing.T) {
	want := liveeval.Want{AmountsToman: []*int64{i64(1)}, Directions: []string{"out"}, Date: "d"}
	if v := liveeval.Score(want, liveeval.Got{}); v.OK() {
		t.Error("an empty run must fail")
	}
}
