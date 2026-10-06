package inputrules

import (
	"regexp"
	"strings"
)

// doc is the normalized Input split into lines and blank-line paragraphs
// (one bank message per paragraph).
type doc struct {
	lines []string
	para  []int // paragraph number of each line; -1 for blank lines
}

func parseDoc(text string) doc {
	lines := strings.Split(strings.TrimSpace(Normalize(text)), "\n")
	d := doc{lines: lines, para: make([]int, len(lines))}
	p := 0
	for i, l := range lines {
		if l == "" {
			d.para[i] = -1
			if i > 0 && d.para[i-1] != -1 {
				p++
			}
			continue
		}
		d.para[i] = p
	}
	return d
}

// numToken is a number printed in the text, separators included.
type numToken struct {
	line, start, end int // byte offsets in d.lines[line]
	value            string
}

var numRe = regexp.MustCompile(`\d[\d,]*`)

func (d doc) tokens() []numToken {
	var out []numToken
	for i, l := range d.lines {
		for _, m := range numRe.FindAllStringIndex(l, -1) {
			end := m[1]
			for end > m[0] && l[end-1] == ',' {
				end--
			}
			v := strings.TrimLeft(strings.ReplaceAll(l[m[0]:end], ",", ""), "0")
			out = append(out, numToken{line: i, start: m[0], end: end, value: v})
		}
	}
	return out
}

// isBalanceLine reports whether a line prints the account balance.
func isBalanceLine(l string) bool {
	return strings.Contains(l, "مانده") || strings.Contains(l, "موجودی")
}

// paragraph returns the lines of paragraph p.
func (d doc) paragraph(p int) []string {
	var out []string
	for i, l := range d.lines {
		if d.para[i] == p {
			out = append(out, l)
		}
	}
	return out
}

// joined returns the normalized text on one line.
func (d doc) joined() string { return oneLine(strings.Join(d.lines, " ")) }
