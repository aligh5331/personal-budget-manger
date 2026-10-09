package inputrules

import (
	"regexp"
	"strings"
)

var (
	// dateTimeLineRe matches a bank date/time line ("زمان: *12:05 1405/07/04*",
	// "0704-19:30", "1405/07/01 - 18:20").
	dateTimeLineRe = regexp.MustCompile(`\d{1,2}:\d{2}|\d{2,4}/\d{1,2}/\d{1,2}`)
	// headerLineRe matches a bank header line ("بانک ملی ایران",
	// "اطلاع رسانی بانک ملی ایران: ...").
	headerLineRe = regexp.MustCompile(`^\*?(اطلاع رسانی\s+)?بانک\s`)
)

// description returns the Owner's own words, never invented text.
//
// The model's note is kept only if it appears in the Input (both
// normalized, so a note the model tidied from ي to ی still counts).
// Otherwise code cuts the note out: the lines after the last bank date/time
// line, or else the lines before the first bank header line. Otherwise the
// description is empty.
func description(d doc, text, modelNote string) string {
	if contains(text, modelNote) {
		return strings.TrimSpace(Normalize(modelNote))
	}
	last, first := -1, -1
	for i, l := range d.lines {
		if dateTimeLineRe.MatchString(l) {
			last = i
		}
		if first == -1 && headerLineRe.MatchString(l) {
			first = i
		}
	}
	if last >= 0 {
		if s := joinLines(d.lines[last+1:]); s != "" {
			return s
		}
	}
	if first > 0 {
		return joinLines(d.lines[:first])
	}
	return ""
}

// messageText returns the text the Categorizer reads for one draft: the
// original lines of the bank message that holds the amount plus the note
// lines, in Input order, unmodified. When that is every line of the Input
// (one message) the Input is returned as is, and so it is when the amount
// was not found.
func messageText(text string, d doc, tok numToken, found bool, note string) string {
	if !found {
		return text
	}
	orig := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	norm := strings.Split(Normalize(text), "\n")
	lead := 0
	for lead < len(norm) && strings.TrimSpace(norm[lead]) == "" {
		lead++
	}
	if len(orig) != len(norm) || lead+len(d.lines) > len(orig) {
		return text
	}
	var picked []string
	all := true
	for i, l := range d.lines {
		switch {
		case l == "":
		case d.para[i] == d.para[tok.line] || contains(note, l):
			picked = append(picked, orig[lead+i])
		default:
			all = false
		}
	}
	if all || len(picked) == 0 {
		return text
	}
	return strings.Join(picked, "\n")
}

func joinLines(lines []string) string {
	var out []string
	for _, l := range lines {
		if l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
