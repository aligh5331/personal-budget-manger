package bot_test

import (
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/bot"
)

func TestCallbackDataRoundTrips(t *testing.T) {
	data := bot.BuildCallbackData("t", int64(42), "cat:7")
	if data != "t:42:cat:7" {
		t.Fatalf("built %q", data)
	}
	c := bot.ParseCallbackData(data)
	if id, ok := c.Int(1); !ok || id != 42 {
		t.Errorf("Int(1) = %d, %v", id, ok)
	}
	if c.Part(0) != "t" || c.Part(2) != "cat" || c.Part(9) != "" {
		t.Errorf("parts = %v", c)
	}
	if got := c.Rest(2); got != "cat:7" {
		t.Errorf("Rest(2) = %q", got)
	}
	if _, ok := c.Int(2); ok {
		t.Error("Int on a word succeeded")
	}
	if bot.BuildCallbackData("l", "p", 3) != "l:p:3" {
		t.Error("int part not formatted")
	}
}
