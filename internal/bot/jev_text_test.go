package bot_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
)

// Spec #30: Jev's input is that bank message's text plus the shared note.
func TestEachBatchTransactionSendsOnlyItsOwnMessageToJev(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(case16Reading())

	h.SendText(smsCase16)

	calls := h.Categorizer.Calls()
	if len(calls) != 2 {
		t.Fatalf("categorizer called %d times, want 2", len(calls))
	}
	for i, c := range calls {
		own, other := "1,500,000", "2,300,000"
		if i == 1 {
			own, other = other, own
		}
		if !strings.Contains(c.Text, own) || strings.Contains(c.Text, other) {
			t.Errorf("call %d text = %q, want only the %s message", i, c.Text, own)
		}
		if !strings.Contains(c.Text, "هديه تولد مامان") {
			t.Errorf("call %d text = %q, want the shared note", i, c.Text)
		}
	}
	for _, id := range confirmedIDs(t, h.LastSent()) {
		if got := storedTransaction(t, h, id).RawText; got != smsCase16 {
			t.Errorf("raw_text = %q, want the full Input", got)
		}
	}
}

func TestTheBackgroundRetryAlsoSendsOnlyTheOwnMessage(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(case16Reading())
	h.Categorizer.Fail(errors.New("503"))
	h.Categorizer.Fail(errors.New("503"))
	h.Categorizer.Choose("Gifts", 0.9)
	h.Categorizer.Choose("Shopping", 0.9)

	h.SendText(smsCase16)
	h.Clock.Advance(11 * time.Minute)
	if err := h.Bot.RecategorizeDue(t.Context()); err != nil {
		t.Fatal(err)
	}

	calls := h.Categorizer.Calls()
	if len(calls) != 4 {
		t.Fatalf("categorizer called %d times, want 4", len(calls))
	}
	for i, c := range calls[2:] {
		other := "2,300,000"
		if i == 1 {
			other = "1,500,000"
		}
		if strings.Contains(c.Text, other) {
			t.Errorf("retry %d text = %q, holds the other message", i, c.Text)
		}
	}
}
