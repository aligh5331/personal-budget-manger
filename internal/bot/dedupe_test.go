package bot_test

import (
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
)

func TestRepeatedUpdateIsProcessedOnce(t *testing.T) {
	h := bottest.New(t)
	u := h.SendText("/help")
	h.Feed(u)

	if n := len(h.Sent()); n != 1 {
		t.Fatalf("sent %d replies, want 1", n)
	}
}

func TestOlderUpdateIsSkipped(t *testing.T) {
	h := bottest.New(t)
	h.Feed(bale.Update{UpdateID: 10, Message: h.OwnerMessage("/help")})
	h.Feed(bale.Update{UpdateID: 9, Message: h.OwnerMessage("/help")})

	if n := len(h.Sent()); n != 1 {
		t.Fatalf("sent %d replies, want 1", n)
	}
}

func TestFirstUpdateZeroIsProcessed(t *testing.T) {
	// Bale's update_id starts at 0.
	h := bottest.New(t)
	h.Feed(bale.Update{UpdateID: 0, Message: h.OwnerMessage("/help")})

	if n := len(h.Sent()); n != 1 {
		t.Fatalf("sent %d replies, want 1", n)
	}
}

func TestProcessedUpdateIsSkippedAfterRestart(t *testing.T) {
	h := bottest.New(t)
	u := h.SendText("/help")
	h.Restart()
	h.Feed(u)

	if n := len(h.Sent()); n != 1 {
		t.Fatalf("sent %d replies, want 1", n)
	}
}

func TestDroppedUpdatesAlsoAdvanceTheHighWaterMark(t *testing.T) {
	h := bottest.New(t)
	h.Feed(bale.Update{UpdateID: 5, Message: h.MessageFrom(bottest.StrangerID, "hi")})
	h.Feed(bale.Update{UpdateID: 5, Message: h.OwnerMessage("/help")})

	if n := len(h.Sent()); n != 0 {
		t.Fatalf("sent %d replies to an already-seen update_id, want 0", n)
	}
}
