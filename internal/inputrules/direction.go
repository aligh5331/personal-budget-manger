package inputrules

import (
	"strings"

	"github.com/aligh5331/personal-budget-manger/internal/extract"
)

// MinConfidence is the model confidence below which a Direction read only
// by the model is treated as ambiguous.
const MinConfidence = 0.7

var (
	outKeywords      = []string{"برداشت", "خرید", "پرداخت"}
	inKeywords       = []string{"واریز"}
	transferKeywords = []string{"انتقال"}
)

// direction decides an item's Direction, first rule that gives an answer
// wins:
//
//  1. the sign printed next to the amount (- out, + in);
//  2. a keyword on the bank message's own lines (برداشت, خرید, پرداخت = out;
//     واریز = in; out and in together, or a bare انتقال, = ambiguous);
//  3. the same keywords in the Owner's note;
//  4. the model's reading, which also covers the note's meaning ("حقوق"):
//     ambiguous below MinConfidence;
//  5. no cue at all: out (expense). The model saying "ambiguous" with no
//     transfer word in the text counts as no cue.
func direction(d doc, tok numToken, found bool, note string, it extract.Item) Direction {
	if found {
		if s := sign(d.lines[tok.line], tok); s != "" {
			return s
		}
	}
	if k := keywordDirection(bankLines(d, tok, found, note)); k != "" {
		return k
	}
	if k := keywordDirection([]string{note}); k != "" {
		return k
	}
	switch it.Direction {
	case extract.DirOut, extract.DirIn:
		if it.Confidence < MinConfidence {
			return Ambiguous
		}
		return Direction(it.Direction)
	}
	return Out
}

// sign returns the +/- printed right after the amount (Melli: "63,000,000-"),
// or right before it ("-63,000").
func sign(line string, t numToken) Direction {
	after := strings.TrimLeft(line[t.end:], " ")
	switch {
	case strings.HasPrefix(after, "-"):
		return Out
	case strings.HasPrefix(after, "+"):
		return In
	}
	if t.start > 0 {
		c := line[t.start-1]
		if (c == '-' || c == '+') && (t.start == 1 || strings.ContainsRune(" :*", rune(line[t.start-2]))) {
			if c == '-' {
				return Out
			}
			return In
		}
	}
	return ""
}

// bankLines returns the lines of the bank message holding the amount (its
// blank-line paragraph), or of the whole Input when the amount was not
// found, minus the lines that make up the Owner's note.
func bankLines(d doc, tok numToken, found bool, note string) []string {
	lines := d.lines
	if found {
		lines = d.paragraph(d.para[tok.line])
	}
	var out []string
	for _, l := range lines {
		if !contains(note, l) {
			out = append(out, l)
		}
	}
	return out
}

func keywordDirection(lines []string) Direction {
	text := strings.Join(lines, "\n")
	has := func(words []string) bool {
		for _, w := range words {
			if strings.Contains(text, w) {
				return true
			}
		}
		return false
	}
	out, in := has(outKeywords), has(inKeywords)
	switch {
	case out && in:
		return Ambiguous
	case out:
		return Out
	case in:
		return In
	case has(transferKeywords):
		return Ambiguous
	}
	return ""
}
