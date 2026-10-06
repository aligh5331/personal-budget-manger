package main_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The release workflow and the Dockerfile bake the tag into the binary with
// this -X flag; the bot reports it in /help and with -version.
const versionLDFlag = "-X github.com/aligh5331/personal-budget-manger/internal/version.Version="

func TestBinaryReportsVersionBakedInWithLDFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := filepath.Join(t.TempDir(), "bot")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-ldflags", versionLDFlag+"v9.8.7", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	out, err := exec.Command(bin, "-version").Output()
	if err != nil {
		t.Fatalf("bot -version: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "v9.8.7" {
		t.Errorf("bot -version printed %q, want %q", got, "v9.8.7")
	}
}
