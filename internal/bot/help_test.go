package bot_test

import (
	"strings"
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
)

func TestHelpShowsCommandsInputConventionAndVersion(t *testing.T) {
	for _, cmd := range []string{"/help", "/start", "/help@personal_bm_bot"} {
		t.Run(cmd, func(t *testing.T) {
			h := bottest.New(t)
			h.SendText(cmd)

			sent := h.Sent()
			if len(sent) != 1 {
				t.Fatalf("sent %d messages, want 1", len(sent))
			}
			got := sent[0]
			if got.ChatID != bottest.OwnerID {
				t.Errorf("sent to chat %d, want Owner", got.ChatID)
			}
			for _, want := range []string{"/help", "bank message", bottest.Version} {
				if !strings.Contains(got.Text, want) {
					t.Errorf("help text missing %q:\n%s", want, got.Text)
				}
			}
		})
	}
}

func TestUnknownCommandGetsCommandList(t *testing.T) {
	h := bottest.New(t)
	h.SendText("/nosuchthing")

	got := h.LastSent().Text
	if !strings.Contains(got, "/help") {
		t.Errorf("reply to unknown command lacks the command list:\n%s", got)
	}
}
