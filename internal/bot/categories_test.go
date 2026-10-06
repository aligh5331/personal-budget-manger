package bot_test

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// openCategories sends /categories and returns the list as the message the
// Owner taps.
func openCategories(t *testing.T, h *bottest.Harness) bale.Message {
	t.Helper()
	h.SendText("/categories")
	s := h.LastSent()
	return bale.Message{MessageID: 7000, Chat: bale.Chat{ID: s.ChatID, Type: bale.ChatPrivate}, Text: s.Text, ReplyMarkup: s.ReplyMarkup}
}

// tapList taps the labelled button on the list message.
func tapList(t *testing.T, h *bottest.Harness, msg bale.Message, label string) {
	t.Helper()
	h.Tap(msg, markupData(t, msg.ReplyMarkup, label))
}

// redrawn returns the list message as last edited.
func redrawn(t *testing.T, h *bottest.Harness, msg bale.Message) bale.Message {
	t.Helper()
	e := lastEdit(t, h)
	msg.Text, msg.ReplyMarkup = e.Text, e.ReplyMarkup
	return msg
}

func activeNames(t *testing.T, h *bottest.Harness, kind string) []string {
	t.Helper()
	cs, err := h.Store.ActiveCategories(context.Background(), kind)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func TestCategoriesListsActiveWithButtons(t *testing.T) {
	h := bottest.New(t)
	msg := openCategories(t, h)

	assertContains(t, msg.Text, "Food", "Salary", "Uncategorized")
	got := labels(msg.ReplyMarkup)
	for _, want := range []string{"Rename Food", "Archive Food", "Rename Salary", "Add", "Show archived"} {
		if !slices.Contains(got, want) {
			t.Errorf("buttons %v lack [%s]", got, want)
		}
	}
	for _, l := range got {
		if strings.HasSuffix(l, "Uncategorized") {
			t.Errorf("Uncategorized has a button [%s]", l)
		}
	}
}

func TestAddACategory(t *testing.T) {
	h := bottest.New(t)
	msg := openCategories(t, h)

	tapList(t, h, msg, "Add")
	assertContains(t, h.LastSent().Text, "name | Persian hint | expense or income")
	reply(h, "Gym | باشگاه | expense")

	assertContains(t, h.LastSent().Text, "Added Gym")
	cs, err := h.Store.ActiveCategories(context.Background(), storage.KindExpense)
	if err != nil {
		t.Fatal(err)
	}
	last := cs[len(cs)-1]
	if last.Name != "Gym" || last.Hint != "باشگاه" {
		t.Fatalf("added %+v, want Gym with its hint", last)
	}
	assertContains(t, lastEdit(t, h).Text, "Gym")
}

func TestAddWithoutAHintUsesTheName(t *testing.T) {
	h := bottest.New(t)
	msg := openCategories(t, h)
	tapList(t, h, msg, "Add")
	reply(h, "Side income | income")

	cs, _ := h.Store.ActiveCategories(context.Background(), storage.KindIncome)
	last := cs[len(cs)-1]
	if last.Name != "Side income" || last.Hint != "Side income" {
		t.Errorf("added %+v, want hint defaulting to the name", last)
	}
}

func TestAMalformedLineShowsTheFormatAndKeepsTheQuestion(t *testing.T) {
	h := bottest.New(t)
	msg := openCategories(t, h)
	tapList(t, h, msg, "Add")
	before := len(activeNames(t, h, storage.KindExpense))
	promptID := 1000 + int64(len(h.Sent()))

	for _, bad := range []string{"Gym", "Gym | باشگاه | spending", "a | b | c | expense", " | | expense"} {
		m := h.OwnerMessage(bad)
		m.ReplyToMessage = &bale.Message{MessageID: promptID, Chat: m.Chat}
		h.SendMessage(m)
		assertContains(t, h.LastSent().Text, "name | Persian hint | expense or income", "Gym | باشگاه | expense")
	}
	if got := len(activeNames(t, h, storage.KindExpense)); got != before {
		t.Errorf("a malformed line changed the list: %d -> %d", before, got)
	}
	m := h.OwnerMessage("Gym | expense")
	m.ReplyToMessage = &bale.Message{MessageID: promptID, Chat: m.Chat}
	h.SendMessage(m)
	if !slices.Contains(activeNames(t, h, storage.KindExpense), "Gym") {
		t.Error("a correct line after errors did not add")
	}
}

func TestAddRejectsADuplicateName(t *testing.T) {
	h := bottest.New(t)
	msg := openCategories(t, h)
	tapList(t, h, msg, "Add")
	reply(h, "food | expense")
	assertContains(t, h.LastSent().Text, "already a Category called food")
}

func TestRenameACategory(t *testing.T) {
	h := bottest.New(t)
	msg := openCategories(t, h)
	food := categoryNamed(t, h, "Food")

	tapList(t, h, msg, "Rename Food")
	assertContains(t, h.LastSent().Text, "New name for Food")
	reply(h, "Meals")

	c, ok, err := h.Store.CategoryByID(context.Background(), food.ID)
	if err != nil || !ok || c.Name != "Meals" {
		t.Fatalf("category = %+v ok=%v err=%v, want renamed to Meals", c, ok, err)
	}
	if c.Hint != food.Hint {
		t.Errorf("hint changed to %q", c.Hint)
	}
	assertContains(t, lastEdit(t, h).Text, "Meals")
}

func TestRenameRejectsATakenName(t *testing.T) {
	h := bottest.New(t)
	msg := openCategories(t, h)
	tapList(t, h, msg, "Rename Food")
	reply(h, "Snacks")
	assertContains(t, h.LastSent().Text, "already a Category called Snacks")
	if !slices.Contains(activeNames(t, h, storage.KindExpense), "Food") {
		t.Error("Food was renamed")
	}
}

func TestRenameKeepsADefaultedHintInStepWithTheName(t *testing.T) {
	h := bottest.New(t)
	msg := openCategories(t, h)
	tapList(t, h, msg, "Add")
	reply(h, "Gym | expense")
	msg = redrawn(t, h, msg)
	tapList(t, h, msg, "Rename Gym")
	reply(h, "Fitness")
	if c := categoryNamed(t, h, "Fitness"); c.Hint != "Fitness" {
		t.Errorf("hint = %q, want it to follow the rename", c.Hint)
	}
}

func TestArchiveHidesFromPickersAndJevButKeepsOldTransactions(t *testing.T) {
	h := bottest.New(t)
	h.Extractor.Return(melliReading("تنقلات"))
	h.Categorizer.Choose("Snacks", 0.92)
	h.SendText(melliBale + "\nتنقلات")
	conf := h.LastSent()
	id := savedID(t, conf)
	snacks := categoryNamed(t, h, "Snacks")

	list := openCategories(t, h)
	tapList(t, h, list, "Archive Snacks")

	if slices.Contains(activeNames(t, h, storage.KindExpense), "Snacks") {
		t.Fatal("Snacks still active")
	}
	if e := lastEdit(t, h); slices.Contains(labels(e.ReplyMarkup), "Archive Snacks") || strings.Contains(e.Text, "Snacks") {
		t.Errorf("list still shows Snacks: %q %v", e.Text, labels(e.ReplyMarkup))
	}

	// Jev is not offered it.
	h.Extractor.Return(melliReading("ناهار"))
	h.SendText(melliBale + "\nناهار")
	calls := h.Categorizer.Calls()
	if opts := calls[len(calls)-1].OptionNames(); slices.Contains(opts, "Snacks") {
		t.Errorf("Jev options %v include archived Snacks", opts)
	}

	// The picker does not offer it, and the old Transaction keeps it.
	h.Tap(confirmationOf(conf, id), markupData(t, conf.ReplyMarkup, "Category"))
	if got := labels(lastEdit(t, h).ReplyMarkup); slices.Contains(got, "Snacks") || slices.Contains(got, "• Snacks") {
		t.Errorf("picker offers archived Snacks: %v", got)
	}
	tx := storedTransaction(t, h, id)
	if tx.CategoryID == nil || *tx.CategoryID != snacks.ID {
		t.Errorf("old Transaction category = %v, want Snacks kept", tx.CategoryID)
	}
	assertContains(t, h.LastSent().Text, "ناهار")
}

func TestShowArchivedAndUnarchive(t *testing.T) {
	h := bottest.New(t)
	list := openCategories(t, h)
	tapList(t, h, list, "Archive Rent")
	list = redrawn(t, h, list)

	tapList(t, h, list, "Show archived")
	list = redrawn(t, h, list)
	if got := labels(list.ReplyMarkup); !slices.Contains(got, "Unarchive Rent (expense)") {
		t.Fatalf("archived list buttons = %v", got)
	}
	tapList(t, h, list, "Unarchive Rent (expense)")

	if !slices.Contains(activeNames(t, h, storage.KindExpense), "Rent") {
		t.Error("Rent not active after unarchive")
	}
	assertContains(t, lastEdit(t, h).Text, "No archived Categories")
}

func TestUncategorizedCannotBeChangedAndNothingDeletes(t *testing.T) {
	h := bottest.New(t)
	ctx := context.Background()
	un, _ := h.Store.Uncategorized(ctx)

	if ok, err := h.Store.RenameCategory(ctx, un.ID, "Misc"); err != nil || ok {
		t.Errorf("rename Uncategorized ok=%v err=%v", ok, err)
	}
	if ok, err := h.Store.SetCategoryArchived(ctx, un.ID, true); err != nil || ok {
		t.Errorf("archive Uncategorized ok=%v err=%v", ok, err)
	}
	list := openCategories(t, h)
	h.Tap(list, "cm:"+strconv.FormatInt(un.ID, 10)+":arc")
	answers := h.Bale.Answers()
	assertContains(t, answers[len(answers)-1].Text, "can't be changed")
	if got, _ := h.Store.Uncategorized(ctx); got.Name != "Uncategorized" || got.Archived {
		t.Errorf("Uncategorized changed: %+v", got)
	}
}

func TestCategoryPromptSurvivesARestart(t *testing.T) {
	h := bottest.New(t)
	msg := openCategories(t, h)
	tapList(t, h, msg, "Add")
	h.Restart()
	reply(h, "Gym | expense")
	if !slices.Contains(activeNames(t, h, storage.KindExpense), "Gym") {
		t.Error("reply after a restart did not add")
	}
}
