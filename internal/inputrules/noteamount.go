package inputrules

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/aligh5331/personal-budget-manger/internal/extract"
)

// colloquialLimit: a bare note amount below it is in thousands of toman.
const colloquialLimit = 10000

// multipliers are the words the Owner writes after a note amount, checked
// in order.
var multipliers = []struct {
	word string
	mult int64
}{
	{"هزار", 1000},
	{"میلیون", 1000000},
	{"ملیون", 1000000},
	{"k", 1000},
	{"K", 1000},
}

// rialWords mark a note amount as rial.
var rialWords = []string{"ریال", "IRR"}

// readAmount finds the item's amount in the Input and converts it to toman.
// tok is the printed number, used for the Direction's sign.
//
// Bank amounts are rial, always: the model's unit is ignored and the
// colloquial rule never applies. An Input with no bank message (no bank
// header and no balance line) is all the Owner's note, and its amount is a
// note amount (see noteToman).
//
// With a bank message, the amount is looked up on the bank's lines first.
// If the model read it from the note instead, the bank amount wins: when
// the bank lines print exactly one amount, that one is used. Only when the
// bank prints none is the note amount kept.
func readAmount(d doc, note string, it extract.Item, nth int) (tok numToken, toman int64, ok bool) {
	if !hasBankMessage(d) {
		all := func(int) bool { return true }
		if tok, ok := d.noteToken(all, it.Amount); ok {
			return tok, d.noteToman(tok), true
		}
		return numToken{}, 0, false
	}

	isNote := func(line int) bool { return contains(note, d.lines[line]) }
	if tok, ok := d.bankToken(isNote, it.Amount, nth); ok {
		return tok, rialToToman(it.Amount), true
	}
	tok, ok = d.noteToken(isNote, it.Amount)
	if !ok {
		return numToken{}, 0, false
	}
	if b, ok := d.soleBankAmount(isNote); ok {
		v, _ := strconv.ParseInt(b.value, 10, 64)
		return b, rialToToman(v), true
	}
	return tok, d.noteToman(tok), true
}

// hasBankMessage reports whether the Input holds a bank message.
func hasBankMessage(d doc) bool {
	for _, l := range d.lines {
		if headerLineRe.MatchString(l) || isBalanceLine(l) {
			return true
		}
	}
	return false
}

func rialToToman(rial int64) int64 { return (rial + 5) / 10 }

// bankToken finds the nth printed occurrence of amount on the bank's lines
// (not the balance, not the note), or the last one if there are fewer.
func (d doc) bankToken(isNote func(int) bool, amount int64, nth int) (numToken, bool) {
	if amount <= 0 {
		return numToken{}, false
	}
	want := strconv.FormatInt(amount, 10)
	var hits []numToken
	for _, t := range d.tokens() {
		if t.value == want && !isNote(t.line) && !isBalanceLine(d.lines[t.line]) {
			hits = append(hits, t)
		}
	}
	if len(hits) == 0 {
		return numToken{}, false
	}
	return hits[min(nth, len(hits)-1)], true
}

// soleBankAmount returns the bank's amount when its lines print exactly one
// number besides the balance, the date and time and the header.
func (d doc) soleBankAmount(isNote func(int) bool) (numToken, bool) {
	var hits []numToken
	for _, t := range d.tokens() {
		l := d.lines[t.line]
		if isNote(t.line) || isBalanceLine(l) || dateTimeLineRe.MatchString(l) || headerLineRe.MatchString(l) || t.value == "" {
			continue
		}
		hits = append(hits, t)
	}
	if len(hits) != 1 {
		return numToken{}, false
	}
	return hits[0], true
}

// noteToken finds the model's note amount on the note's lines. The model may
// give the number as printed (250), with the multiplier applied (450 هزار
// as 450000) or in toman after the colloquial rule (250 as 250000); all
// three match the same printed number.
func (d doc) noteToken(isNote func(int) bool, amount int64) (numToken, bool) {
	if amount <= 0 {
		return numToken{}, false
	}
	for _, t := range d.tokens() {
		if !isNote(t.line) {
			continue
		}
		v, mult, _, ok := d.noteNumber(t)
		if ok && (amount == v || amount == v*mult || amount == d.noteToman(t)) {
			return t, true
		}
	}
	return numToken{}, false
}

// noteToman converts a note amount to toman:
//
//   - an explicit multiplier (هزار, میلیون, k) applies as written;
//   - «ریال» on the line makes it rial, read literally (÷10, rounded);
//   - otherwise it is toman, and a bare amount under 10,000 is in
//     thousands (۲۵۰ is 250,000, ۱۵۰۰ is 1,500,000, ۱۲۰۰۰ stays 12,000).
//     Always thousands, never millions.
func (d doc) noteToman(t numToken) int64 {
	v, mult, rial, _ := d.noteNumber(t)
	switch {
	case rial:
		return rialToToman(v * mult)
	case mult == 1 && v < colloquialLimit:
		return v * 1000
	}
	return v * mult
}

// noteNumber reads a printed note number with the multiplier written after
// it and whether its line says rial.
func (d doc) noteNumber(t numToken) (v, mult int64, rial, ok bool) {
	v, err := strconv.ParseInt(t.value, 10, 64)
	if err != nil || v <= 0 || v > 1e12 {
		return 0, 0, false, false
	}
	line := d.lines[t.line]
	return v, multiplierAfter(line[t.end:]), anyIn(line, rialWords...), true
}

// multiplierAfter returns the multiplier written right after a number, or 1.
func multiplierAfter(rest string) int64 {
	rest = strings.TrimLeft(rest, " ")
	for _, m := range multipliers {
		if !strings.HasPrefix(rest, m.word) {
			continue
		}
		next, _ := utf8.DecodeRuneInString(rest[len(m.word):])
		if (m.word == "k" || m.word == "K") && unicode.IsLetter(next) {
			continue // "80 kg", not 80k
		}
		return m.mult
	}
	return 1
}
