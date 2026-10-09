// Package sourcecheck holds repo-wide checks on the source files themselves.
package sourcecheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

// invisible reports runes that render as nothing in an editor but change
// meaning: zero-width and bidi marks, the BOM, the soft hyphen. A ZWNJ in
// Persian text belongs in source as an escape (\u200c) or a hex constant,
// never as the literal character.
func invisible(r rune) bool {
	switch {
	case r >= 0x200B && r <= 0x200F, // zero-width space/joiners, LRM, RLM
		r >= 0x202A && r <= 0x202E, // bidi embeddings and overrides
		r >= 0x2060 && r <= 0x2064, // word joiner, invisible operators
		r >= 0x2066 && r <= 0x2069, // bidi isolates
		r == 0xFEFF,                // BOM / zero-width no-break space
		r == 0x00AD:                // soft hyphen
		return true
	}
	return false
}

func TestNoInvisibleCharactersInGoSource(t *testing.T) {
	root := repoRoot(t)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == ".claude" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		line := 1
		for rest := src; len(rest) > 0; {
			r, size := utf8.DecodeRune(rest)
			if r == '\n' {
				line++
			}
			if invisible(r) {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s:%d: invisible character U+%04X; write it as an escape or hex constant", rel, line, r)
			}
			rest = rest[size:]
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInvisibleCatchesTheCharactersThatBrokeABuild(t *testing.T) {
	for _, r := range []rune{0x200C, 0x200F, 0xFEFF, 0x202E, 0x2066} {
		if !invisible(r) {
			t.Errorf("U+%04X not flagged", r)
		}
	}
	for _, r := range []rune{'a', ' ', '\n', 0x0645} { // 0x0645 is Persian meem
		if invisible(r) {
			t.Errorf("U+%04X flagged", r)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}
