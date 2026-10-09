package bot_test

import (
	"strings"
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
)

func TestEachTransactionInABatchGetsItsOwnCategory(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(case16Reading())
	h.Categorizer.Choose("Gifts", 0.9)
	h.Categorizer.Choose("Shopping", 0.8)

	h.SendText(smsCase16)

	if n := len(h.Categorizer.Calls()); n != 2 {
		t.Fatalf("categorizer called %d times, want once per Transaction", n)
	}
	conf := h.LastSent()
	assertContains(t, conf.Text, "Category: Gifts", "Category: Shopping")
	ids := confirmedIDs(t, conf)
	assertCategory(t, h, storedTransaction(t, h, ids[0]), "Gifts")
	assertCategory(t, h, storedTransaction(t, h, ids[1]), "Shopping")
	// Every Transaction has its own Category and Edit buttons.
	for _, label := range []string{"Category 1", "Category 2", "Edit 1", "Edit 2"} {
		buttonData(t, conf, label)
	}
}

func TestAnInternalTransferInABatchIsNotCategorized(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(case8Reading())

	h.SendText(smsCase8)

	if n := len(h.Categorizer.Calls()); n != 0 {
		t.Errorf("categorizer called %d times for an internal transfer", n)
	}
	conf := h.LastSent()
	assertContains(t, conf.Text, "Internal transfer")
	for _, row := range conf.ReplyMarkup.InlineKeyboard {
		for _, b := range row {
			if strings.HasPrefix(b.Text, "Category") {
				t.Errorf("internal transfer has a %q button", b.Text)
			}
		}
	}
}

func TestCategoryButtonOnABatchOpensThePickerForThatTransaction(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(case16Reading())
	h.SendText(smsCase16)
	conf := h.LastSent()

	h.Tap(asReceived(conf, 800), buttonData(t, conf, "Category 2"))

	edits := h.Bale.Edits()
	if len(edits) != 1 || edits[0].MessageID != 800 {
		t.Fatalf("edits = %+v, want the message edited into the picker", edits)
	}
	assertContains(t, edits[0].Text, "230,000 toman")
}

func TestSaveAnywayRunsTheCategoryStep(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Repeat() // the same message is sent twice
	h.Extractor.Return(melliReading(""))
	h.Categorizer.Choose("Snacks", 0.9)
	h.Categorizer.Choose("Fuel", 0.9)
	h.SendText(melliBale)
	h.SendText(melliBale)
	dup := h.LastSent()
	if n := len(h.Categorizer.Calls()); n != 1 {
		t.Fatalf("categorizer called %d times before Save anyway, want 1 (not for the held duplicate)", n)
	}

	h.Tap(asReceived(dup, 700), buttonData(t, dup, "Save anyway"))

	e := h.Bale.Edits()[0]
	assertContains(t, e.Text, "Category: Fuel")
	id := savedID(t, bale.SendMessageParams{Text: e.Text, ReplyMarkup: e.ReplyMarkup})
	assertCategory(t, h, storedTransaction(t, h, id), "Fuel")
}
