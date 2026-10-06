package bot

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitMessage(t *testing.T) {
	t.Run("short text stays whole", func(t *testing.T) {
		got := splitMessage("a\nb", 10)
		if len(got) != 1 || got[0] != "a\nb" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("breaks at line ends and keeps every line", func(t *testing.T) {
		var lines []string
		for i := range 300 {
			lines = append(lines, strings.Repeat("x", 20)+string(rune('a'+i%26)))
		}
		text := strings.Join(lines, "\n")
		pages := splitMessage(text, 4096)
		if len(pages) < 2 {
			t.Fatalf("pages = %d, want a split", len(pages))
		}
		for i, p := range pages {
			if n := utf8.RuneCountInString(p); n > 4096 {
				t.Errorf("page %d has %d chars", i, n)
			}
		}
		if joined := strings.Join(pages, "\n"); joined != text {
			t.Errorf("pages do not add up to the text")
		}
	})
	t.Run("counts characters not bytes", func(t *testing.T) {
		line := strings.Repeat("غ", 6)
		pages := splitMessage(line+"\n"+line, 7)
		if len(pages) != 2 || pages[0] != line || pages[1] != line {
			t.Errorf("got %q", pages)
		}
	})
	t.Run("one overlong line is cut", func(t *testing.T) {
		pages := splitMessage(strings.Repeat("y", 25), 10)
		if len(pages) != 3 || utf8.RuneCountInString(pages[2]) != 5 {
			t.Errorf("got %q", pages)
		}
	})
}
