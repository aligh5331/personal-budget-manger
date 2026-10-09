package bot

import (
	"strconv"
	"strings"
)

// callback_data is "<prefix>:<part>:<part>..." (at most 64 bytes). Every
// handler reads and builds it through these two helpers instead of
// splitting strings by hand.

// CallbackData is a parsed callback_data. Part 0 is the prefix.
type CallbackData []string

// ParseCallbackData splits callback_data at every ":".
func ParseCallbackData(data string) CallbackData {
	return strings.Split(data, ":")
}

// Len is the number of parts, the prefix included.
func (c CallbackData) Len() int { return len(c) }

// Part returns part i, or "" when there is none.
func (c CallbackData) Part(i int) string {
	if i < 0 || i >= len(c) {
		return ""
	}
	return c[i]
}

// Rest returns parts i and later joined with ":" again ("" when none), for
// an argument that may itself hold colons.
func (c CallbackData) Rest(i int) string {
	if i < 0 || i >= len(c) {
		return ""
	}
	return strings.Join(c[i:], ":")
}

// Int reads part i as an integer; ok is false when it is missing or not one.
func (c CallbackData) Int(i int) (n int64, ok bool) {
	if i < 0 || i >= len(c) {
		return 0, false
	}
	n, err := strconv.ParseInt(c[i], 10, 64)
	return n, err == nil
}

// BuildCallbackData joins a prefix and parts with ":". Parts may be strings
// or integers.
func BuildCallbackData(prefix string, parts ...any) string {
	var sb strings.Builder
	sb.WriteString(prefix)
	for _, p := range parts {
		sb.WriteByte(':')
		switch v := p.(type) {
		case string:
			sb.WriteString(v)
		case int:
			sb.WriteString(strconv.Itoa(v))
		case int64:
			sb.WriteString(strconv.FormatInt(v, 10))
		default:
			panic("bot: callback data part must be a string or an integer")
		}
	}
	return sb.String()
}
