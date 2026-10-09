package bot_test

import (
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
)

const dropLog = "dropped update from non-owner or non-private chat"

func TestStrangerIsIgnoredSilently(t *testing.T) {
	h := bottest.New(t)
	h.SendMessage(h.MessageFrom(bottest.StrangerID, "/help"))
	h.SendMessage(h.MessageFrom(bottest.StrangerID, "خرید ۲۵۰۰۰۰ ریال"))

	if calls := h.Bale.Calls(); len(calls) != 0 {
		t.Fatalf("bot called Bale for a stranger: %+v", calls)
	}
}

func TestOwnerInGroupIsIgnored(t *testing.T) {
	h := bottest.New(t)
	m := h.OwnerMessage("/help")
	m.Chat = bale.Chat{ID: -100, Type: "group"}
	h.SendMessage(m)

	if calls := h.Bale.Calls(); len(calls) != 0 {
		t.Fatalf("bot answered in a group: %+v", calls)
	}
}

func TestStrangerButtonTapIsNotAnswered(t *testing.T) {
	h := bottest.New(t)
	h.TapAs(bottest.StrangerID, bale.Message{MessageID: 5, Chat: bale.Chat{ID: bottest.StrangerID, Type: bale.ChatPrivate}}, "t:1:undo")

	if calls := h.Bale.Calls(); len(calls) != 0 {
		t.Fatalf("bot answered a stranger's tap: %+v", calls)
	}
}

func TestDroppedSendersAreLoggedOncePerMinute(t *testing.T) {
	h := bottest.New(t)
	h.SendMessage(h.MessageFrom(bottest.StrangerID, "hi"))
	h.Clock.Advance(30 * time.Second)
	h.SendMessage(h.MessageFrom(bottest.StrangerID, "hi again"))
	h.SendMessage(h.MessageFrom(bottest.StrangerID+1, "someone else"))

	if n := h.Logs.Count(dropLog); n != 2 {
		t.Fatalf("drop log lines = %d, want 2 (one per sender within a minute)", n)
	}

	h.Clock.Advance(31 * time.Second)
	h.SendMessage(h.MessageFrom(bottest.StrangerID, "a minute later"))
	if n := h.Logs.Count(dropLog); n != 3 {
		t.Fatalf("drop log lines = %d, want 3 after a minute passed", n)
	}
}

func TestOwnerEditedMessageIsIgnored(t *testing.T) {
	h := bottest.New(t)
	m := h.OwnerMessage("/help")
	m.EditDate = h.Clock.Now().Unix()
	h.Feed(bale.Update{UpdateID: h.NextUpdateID(), EditedMessage: m})

	if calls := h.Bale.Calls(); len(calls) != 0 {
		t.Fatalf("bot reacted to an edit: %+v", calls)
	}
}

func TestOwnerButtonTapIsAlwaysAnswered(t *testing.T) {
	h := bottest.New(t)
	h.Tap(bale.Message{MessageID: 5, Chat: bale.Chat{ID: bottest.OwnerID, Type: bale.ChatPrivate}}, "nosuch:1")

	if n := len(h.Bale.Answers()); n != 1 {
		t.Fatalf("answered %d times, want 1", n)
	}
}
