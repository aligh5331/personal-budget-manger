package inputrules

import (
	"strings"
	"unicode"
)

// Code points Normalize treats specially. Written as numbers so invisible
// characters never hide in the source.
const (
	zwnj       = 0x200C
	zwj        = 0x200D
	nbsp       = 0x00A0
	lrm        = 0x200E
	rlm        = 0x200F
	alm        = 0x061C
	bom        = 0xFEFF
	tatweel    = 0x0640
	arabicYeh  = 0x064A // ي
	alefMaksur = 0x0649 // ى
	arabicKaf  = 0x0643 // ك
	persianYeh = 0x06CC // ی
	persianKaf = 0x06A9 // ک
	arabicSep  = 0x066C // ٬ thousands separator
)

// Normalize puts Persian text in one canonical form so that the model's
// copies of the Input can be compared with the Input itself:
//
//   - Persian (۰-۹) and Arabic-Indic (٠-٩) digits become ASCII;
//   - Arabic ي, ى and ك become Persian ی and ک;
//   - the Arabic thousands separator ٬ becomes ",";
//   - ZWNJ, NBSP and tabs become a space; bidi marks, tatweel and Arabic
//     diacritics (the shadda in ملّی ...) are dropped;
//   - spaces are collapsed, each line is trimmed, CRLF becomes LF.
//
// Line structure is kept: Normalize(s) has as many lines as s.
func Normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ReplaceAll(s, "\r\n", "\n") {
		switch {
		case r >= 0x06F0 && r <= 0x06F9: // Persian digits
			b.WriteRune('0' + (r - 0x06F0))
		case r >= 0x0660 && r <= 0x0669: // Arabic-Indic digits
			b.WriteRune('0' + (r - 0x0660))
		case r == arabicYeh || r == alefMaksur:
			b.WriteRune(persianYeh)
		case r == arabicKaf:
			b.WriteRune(persianKaf)
		case r == arabicSep:
			b.WriteRune(',')
		case r == zwnj || r == nbsp || r == '\t' || r == '\r':
			b.WriteRune(' ')
		case dropped(r):
		default:
			b.WriteRune(r)
		}
	}
	lines := strings.Split(b.String(), "\n")
	for i, l := range lines {
		lines[i] = strings.Join(strings.Fields(l), " ")
	}
	return strings.Join(lines, "\n")
}

func dropped(r rune) bool {
	switch {
	case r == lrm || r == rlm || r == alm || r == zwj || r == bom || r == tatweel:
		return true
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true // bidi embeddings and isolates
	case r >= 0x064B && r <= 0x0652, r == 0x0670:
		return true // Arabic diacritics
	}
	return r != '\n' && unicode.Is(unicode.Cf, r)
}

// contains reports whether needle appears in haystack once both are
// normalized and their line breaks are treated as spaces.
func contains(haystack, needle string) bool {
	n := oneLine(Normalize(needle))
	return n != "" && strings.Contains(oneLine(Normalize(haystack)), n)
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
