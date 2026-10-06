package bot_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/jalali"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// seed describes one stored Transaction for the list tests.
type seed struct {
	year, month, day int // Jalali; the time of day is 10:00
	amount           int64
	dir              string
	category         string // "" = Uncategorized, ignored for internal
	desc             string
	flagged          bool
}

func seedTx(t *testing.T, h *bottest.Harness, s seed) int64 {
	t.Helper()
	ctx := context.Background()
	if s.dir == "" {
		s.dir = storage.DirectionOut
	}
	amount := s.amount
	tx := storage.Transaction{
		CreatedAt:   h.Clock.Now(),
		OccurredAt:  jalali.Date{Year: s.year, Month: s.month, Day: s.day}.At(10, 0, clock.Tehran()),
		AmountToman: &amount,
		Direction:   s.dir,
		Description: s.desc,
		RawText:     fmt.Sprintf("raw text %d", s.amount),
		InputID:     fmt.Sprintf("seed:%d", s.amount),
		Flagged:     s.flagged,
	}
	if s.flagged {
		tx.FlagReason = storage.FlagFollowupTimeout
	}
	if s.dir != storage.DirectionInternal {
		name := s.category
		if name == "" {
			name = "Uncategorized"
		}
		id := categoryNamed(t, h, name).ID
		tx.CategoryID = &id
	}
	id, err := h.Store.SaveTransaction(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// listView follows one /transactions message as the Owner taps through it.
type listView struct {
	t    *testing.T
	h    *bottest.Harness
	msg  bale.Message
	seen int // edits already folded into msg
}

func openList(t *testing.T, h *bottest.Harness) *listView {
	t.Helper()
	h.SendText("/transactions")
	sent := h.LastSent()
	return &listView{
		t: t, h: h,
		msg: bale.Message{
			MessageID:   1000 + int64(len(h.Sent())),
			Chat:        bale.Chat{ID: sent.ChatID, Type: bale.ChatPrivate},
			Text:        sent.Text,
			ReplyMarkup: sent.ReplyMarkup,
		},
	}
}

// tap taps the button labelled label and folds the resulting edit in.
func (v *listView) tap(label string) {
	v.t.Helper()
	v.tapData(markupData(v.t, v.msg.ReplyMarkup, label))
}

func (v *listView) tapData(data string) {
	v.t.Helper()
	v.h.Tap(v.msg, data)
	edits := v.h.Bale.Edits()
	for _, e := range edits[v.seen:] {
		if e.MessageID != v.msg.MessageID {
			continue
		}
		v.msg.Text, v.msg.ReplyMarkup = e.Text, e.ReplyMarkup
	}
	v.seen = len(edits)
}

// rows returns the transaction row buttons (the ones that open a record).
func (v *listView) rows() []string {
	var out []string
	if v.msg.ReplyMarkup == nil {
		return out
	}
	for _, row := range v.msg.ReplyMarkup.InlineKeyboard {
		for _, b := range row {
			if strings.HasPrefix(b.CallbackData, "l:r:") {
				out = append(out, b.Text)
			}
		}
	}
	return out
}

func (v *listView) lastToast() string {
	v.t.Helper()
	a := v.h.Bale.Answers()
	if len(a) == 0 {
		v.t.Fatal("no callback answered")
	}
	return a[len(a)-1].Text
}

func hasLabel(m *bale.InlineKeyboardMarkup, label string) bool {
	for _, l := range labels(m) {
		if l == label || l == "• "+label {
			return true
		}
	}
	return false
}

func TestTransactionsOpensOnThisMonthNewestFirst(t *testing.T) {
	h := bottest.New(t)
	seedTx(t, h, seed{year: 1405, month: 7, day: 2, amount: 100_000, category: "Transport"})
	seedTx(t, h, seed{year: 1405, month: 7, day: 12, amount: 450_000, category: "Food", desc: "Snacks"})
	seedTx(t, h, seed{year: 1405, month: 7, day: 5, amount: 2_000_000, dir: storage.DirectionIn, category: "Salary"})
	seedTx(t, h, seed{year: 1405, month: 6, day: 30, amount: 999_000, category: "Rent"}) // last month

	v := openList(t, h)

	want := []string{
		"12 Mehr · 450,000 · Food · Snacks",
		"5 Mehr · +2,000,000 · Salary",
		"2 Mehr · 100,000 · Transport",
	}
	got := v.rows()
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
	if !strings.Contains(v.msg.Text, "This month (Mehr 1405)") {
		t.Errorf("header = %q", v.msg.Text)
	}
	for _, l := range []string{"This month", "Last month", "All", "Flagged", "Category"} {
		if !hasLabel(v.msg.ReplyMarkup, l) {
			t.Errorf("no [%s] filter button in %v", l, labels(v.msg.ReplyMarkup))
		}
	}
	if hasLabel(v.msg.ReplyMarkup, "Older") || hasLabel(v.msg.ReplyMarkup, "Newer") {
		t.Errorf("one page should have no paging buttons: %v", labels(v.msg.ReplyMarkup))
	}
}

func TestTransactionsMonthBoundaryIsJalaliInTehran(t *testing.T) {
	h := bottest.New(t)
	loc := clock.Tehran()
	// 00:00 on 1 Mehr is in; the last minute of 30 Shahrivar is not.
	in := storage.Transaction{OccurredAt: jalali.Date{Year: 1405, Month: 7, Day: 1}.At(0, 0, loc)}
	out := storage.Transaction{OccurredAt: jalali.Date{Year: 1405, Month: 7, Day: 1}.At(0, 0, loc).Add(-time.Minute)}
	for i, tx := range []storage.Transaction{in, out} {
		amount := int64(1000 * (i + 1))
		tx.AmountToman, tx.Direction, tx.RawText, tx.InputID = &amount, storage.DirectionOut, "raw", fmt.Sprintf("b:%d", i)
		tx.CreatedAt = h.Clock.Now()
		if _, err := h.Store.SaveTransaction(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
	}
	v := openList(t, h)
	if got := v.rows(); len(got) != 1 || !strings.Contains(got[0], "1,000") {
		t.Errorf("this month = %v, want only the 00:00 1 Mehr one", got)
	}
	v.tap("Last month")
	if got := v.rows(); len(got) != 1 || !strings.Contains(got[0], "2,000") {
		t.Errorf("last month = %v, want only the 23:59 30 Shahrivar one", got)
	}
}

func TestTransactionsPaging(t *testing.T) {
	h := bottest.New(t)
	for i := range 23 {
		seedTx(t, h, seed{year: 1405, month: 7, day: 1 + i%14, amount: int64(1000 + i), category: "Food"})
	}
	// Day 14 is the newest day; amounts 1013 and 1027 don't exist, so check by count and order.
	v := openList(t, h)
	if n := len(v.rows()); n != 10 {
		t.Fatalf("page 1 has %d rows, want 10", n)
	}
	if !strings.Contains(v.msg.Text, "page 1 of 3") || !strings.Contains(v.msg.Text, "23 in all") {
		t.Errorf("header = %q", v.msg.Text)
	}
	if hasLabel(v.msg.ReplyMarkup, "Newer") || !hasLabel(v.msg.ReplyMarkup, "Older") {
		t.Errorf("page 1 buttons = %v", labels(v.msg.ReplyMarkup))
	}
	page1 := v.rows()

	v.tap("Older")
	if n := len(v.rows()); n != 10 || !strings.Contains(v.msg.Text, "page 2 of 3") {
		t.Fatalf("page 2: %d rows, header %q", n, v.msg.Text)
	}
	if v.rows()[0] == page1[0] {
		t.Error("page 2 starts with page 1's first row")
	}
	if !hasLabel(v.msg.ReplyMarkup, "Newer") || !hasLabel(v.msg.ReplyMarkup, "Older") {
		t.Errorf("page 2 buttons = %v", labels(v.msg.ReplyMarkup))
	}
	v.tap("Older")
	if n := len(v.rows()); n != 3 || !strings.Contains(v.msg.Text, "page 3 of 3") {
		t.Fatalf("page 3: %d rows, header %q", n, v.msg.Text)
	}
	if hasLabel(v.msg.ReplyMarkup, "Older") {
		t.Errorf("last page offers Older: %v", labels(v.msg.ReplyMarkup))
	}
	v.tap("Newer")
	v.tap("Newer")
	if got := v.rows(); len(got) != 10 || got[0] != page1[0] {
		t.Errorf("back on page 1 = %v, want %v", got, page1)
	}
	if len(h.Sent()) != 1 {
		t.Errorf("paging sent %d messages, want it to edit the one list message", len(h.Sent()))
	}
}

func TestTransactionsNewestFirstWithinADayByTime(t *testing.T) {
	h := bottest.New(t)
	seedTx(t, h, seed{year: 1405, month: 7, day: 3, amount: 111})
	seedTx(t, h, seed{year: 1405, month: 7, day: 9, amount: 222})
	seedTx(t, h, seed{year: 1405, month: 7, day: 6, amount: 333})
	got := openList(t, h).rows()
	if len(got) != 3 || !strings.Contains(got[0], "222") || !strings.Contains(got[1], "333") || !strings.Contains(got[2], "111") {
		t.Errorf("rows = %v, want by occurred_at, newest first", got)
	}
}

func TestTransactionsFilters(t *testing.T) {
	h := bottest.New(t)
	seedTx(t, h, seed{year: 1405, month: 7, day: 10, amount: 11_000, category: "Food"})
	seedTx(t, h, seed{year: 1405, month: 6, day: 20, amount: 22_000, category: "Food"})
	seedTx(t, h, seed{year: 1404, month: 2, day: 5, amount: 33_000, category: "Rent"})
	seedTx(t, h, seed{year: 1405, month: 7, day: 3, amount: 44_000, flagged: true})
	seedTx(t, h, seed{year: 1405, month: 6, day: 1, amount: 55_000, dir: storage.DirectionInternal})

	v := openList(t, h)
	has := func(rows []string, amounts ...string) {
		t.Helper()
		if len(rows) != len(amounts) {
			t.Fatalf("rows = %v, want amounts %v", rows, amounts)
		}
		for i, a := range amounts {
			if !strings.Contains(rows[i], a) {
				t.Fatalf("rows = %v, want amounts %v", rows, amounts)
			}
		}
	}

	has(v.rows(), "11,000", "44,000")

	v.tap("Last month")
	has(v.rows(), "22,000", "55,000")
	if !strings.Contains(v.msg.Text, "Last month (Shahrivar 1405)") {
		t.Errorf("header = %q", v.msg.Text)
	}

	v.tap("All")
	has(v.rows(), "11,000", "44,000", "22,000", "55,000", "33,000")
	if rows := v.rows(); !strings.Contains(rows[4], "1404") {
		t.Errorf("a row from another year shows it: %q", rows[4])
	}
	if !strings.Contains(v.rows()[4], "Rent") || !strings.Contains(v.rows()[3], "Internal") {
		t.Errorf("rows = %v", v.rows())
	}

	v.tap("Flagged")
	has(v.rows(), "44,000")
	if !strings.HasSuffix(v.rows()[0], "flagged") {
		t.Errorf("flagged row = %q", v.rows()[0])
	}

	v.tap("Category")
	for _, want := range []string{"Food", "Salary", "Uncategorized", "Back"} {
		if !hasLabel(v.msg.ReplyMarkup, want) {
			t.Errorf("category picker lacks [%s]: %v", want, labels(v.msg.ReplyMarkup))
		}
	}
	v.tap("Food")
	has(v.rows(), "11,000", "22,000")
	if !strings.Contains(v.msg.Text, "Category: Food") {
		t.Errorf("header = %q", v.msg.Text)
	}
	if !hasLabel(v.msg.ReplyMarkup, "Category") {
		t.Errorf("buttons = %v", labels(v.msg.ReplyMarkup))
	}

	v.tap("This month")
	has(v.rows(), "11,000", "44,000")
}

func TestTransactionsCategoryPickerBackKeepsTheList(t *testing.T) {
	h := bottest.New(t)
	seedTx(t, h, seed{year: 1405, month: 7, day: 10, amount: 11_000})
	v := openList(t, h)
	v.tap("Category")
	v.tap("Back")
	if got := v.rows(); len(got) != 1 {
		t.Errorf("rows after Back = %v", got)
	}
}

func TestTransactionsEmptyList(t *testing.T) {
	h := bottest.New(t)
	v := openList(t, h)
	if !strings.Contains(v.msg.Text, "Nothing here") || len(v.rows()) != 0 {
		t.Errorf("text %q rows %v", v.msg.Text, v.rows())
	}
	if !hasLabel(v.msg.ReplyMarkup, "All") {
		t.Error("filters missing on an empty list")
	}
}

func TestTransactionsRecordView(t *testing.T) {
	h := bottest.New(t)
	id := seedTx(t, h, seed{year: 1405, month: 7, day: 12, amount: 450_000, category: "Food", desc: "Snacks", flagged: true})
	v := openList(t, h)

	v.tap("12 Mehr · 450,000 · Food · Snacks · flagged")

	for _, want := range []string{"450,000 toman", "Category: Food", "Date: 12 Mehr 1405", "Note: Snacks", "Flagged: no answer", "raw text 450000"} {
		if !strings.Contains(v.msg.Text, want) {
			t.Errorf("record %q lacks %q", v.msg.Text, want)
		}
	}
	if got, want := labels(v.msg.ReplyMarkup), []string{"Edit", "Category", "Delete", "Back to list"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("buttons = %v, want %v", got, want)
	}
	if d := markupData(t, v.msg.ReplyMarkup, "Delete"); !strings.HasPrefix(d, fmt.Sprintf("t:%d:", id)) {
		t.Errorf("Delete data = %q", d)
	}
	if len(h.Sent()) != 1 {
		t.Errorf("opening a record sent a new message")
	}
}

func TestTransactionsRecordOfInternalTransferHasNoCategoryButton(t *testing.T) {
	h := bottest.New(t)
	seedTx(t, h, seed{year: 1405, month: 7, day: 12, amount: 70_000, dir: storage.DirectionInternal})
	v := openList(t, h)
	v.tap(v.rows()[0])
	if hasLabel(v.msg.ReplyMarkup, "Category") {
		t.Errorf("buttons = %v", labels(v.msg.ReplyMarkup))
	}
}

func TestTransactionsBackToListKeepsPageAndFilter(t *testing.T) {
	h := bottest.New(t)
	for i := range 12 {
		seedTx(t, h, seed{year: 1405, month: 6, day: 1 + i, amount: int64(1000 + i), category: "Food"})
	}
	v := openList(t, h)
	v.tap("Last month")
	v.tap("Older")
	before := v.rows()
	if len(before) != 2 {
		t.Fatalf("page 2 rows = %v", before)
	}

	v.tap(before[0])
	v.tap("Back to list")

	if got := v.rows(); strings.Join(got, "|") != strings.Join(before, "|") {
		t.Errorf("after Back rows = %v, want %v", got, before)
	}
	if !strings.Contains(v.msg.Text, "Last month") || !strings.Contains(v.msg.Text, "page 2 of 2") {
		t.Errorf("after Back text = %q", v.msg.Text)
	}
}

func TestTransactionsEditFromTheRecord(t *testing.T) {
	h := bottest.New(t)
	id := seedTx(t, h, seed{year: 1405, month: 7, day: 12, amount: 450_000, category: "Food"})
	v := openList(t, h)
	v.tap(v.rows()[0])

	v.tap("Edit")
	if got := labels(v.msg.ReplyMarkup); strings.Join(got, ",") != "Amount,Direction,Date,Description,Back" {
		t.Fatalf("sub-menu = %v", got)
	}
	v.tap("Direction")
	v.tap("Income")

	tx, _, _ := h.Store.TransactionByID(context.Background(), id)
	if tx.Direction != storage.DirectionIn {
		t.Errorf("direction = %s", tx.Direction)
	}
	if !strings.Contains(v.msg.Text, "Income") || !hasLabel(v.msg.ReplyMarkup, "Back to list") {
		t.Errorf("record after edit: %q %v", v.msg.Text, labels(v.msg.ReplyMarkup))
	}
	v.tap("Back to list")
	if got := v.rows(); len(got) != 1 || !strings.Contains(got[0], "+450,000") {
		t.Errorf("list after edit = %v", got)
	}
}

func TestTransactionsRecategorizeFromTheRecord(t *testing.T) {
	h := bottest.New(t)
	id := seedTx(t, h, seed{year: 1405, month: 7, day: 12, amount: 450_000, category: "Food"})
	v := openList(t, h)
	v.tap(v.rows()[0])

	v.tap("Category")
	v.tap("Rent")

	tx, _, _ := h.Store.TransactionByID(context.Background(), id)
	if tx.CategoryID == nil || *tx.CategoryID != categoryNamed(t, h, "Rent").ID {
		t.Fatalf("category = %v", tx.CategoryID)
	}
	if !strings.Contains(v.msg.Text, "Category: Rent") || !hasLabel(v.msg.ReplyMarkup, "Back to list") {
		t.Errorf("record after pick: %q %v", v.msg.Text, labels(v.msg.ReplyMarkup))
	}
	if strings.Contains(v.msg.Text, "Saved") {
		t.Errorf("record redrawn as a confirmation: %q", v.msg.Text)
	}
}

func TestTransactionsDeleteAsksThenHardDeletes(t *testing.T) {
	h := bottest.New(t)
	keep := seedTx(t, h, seed{year: 1405, month: 7, day: 10, amount: 11_000})
	del := seedTx(t, h, seed{year: 1405, month: 7, day: 12, amount: 22_000})
	v := openList(t, h)
	v.tap(v.rows()[0]) // newest: 22,000

	v.tap("Delete")
	if !strings.Contains(v.msg.Text, "Delete?") || strings.Join(labels(v.msg.ReplyMarkup), ",") != "Yes,No" {
		t.Fatalf("confirmation = %q %v", v.msg.Text, labels(v.msg.ReplyMarkup))
	}
	if _, ok, _ := h.Store.TransactionByID(context.Background(), del); !ok {
		t.Fatal("deleted before the Owner said Yes")
	}

	v.tap("No")
	if _, ok, _ := h.Store.TransactionByID(context.Background(), del); !ok {
		t.Fatal("deleted after No")
	}
	if !hasLabel(v.msg.ReplyMarkup, "Back to list") {
		t.Errorf("No should return to the record: %v", labels(v.msg.ReplyMarkup))
	}

	v.tap("Delete")
	v.tap("Yes")
	if _, ok, _ := h.Store.TransactionByID(context.Background(), del); ok {
		t.Error("still stored after Yes")
	}
	if _, ok, _ := h.Store.TransactionByID(context.Background(), keep); !ok {
		t.Error("deleted the wrong Transaction")
	}
	if got := v.rows(); len(got) != 1 || !strings.Contains(got[0], "11,000") {
		t.Errorf("list after delete = %v", got)
	}
	if v.lastToast() != "Deleted" {
		t.Errorf("toast = %q", v.lastToast())
	}
}

func TestTransactionsRowOfADeletedTransactionRefreshesTheList(t *testing.T) {
	h := bottest.New(t)
	id := seedTx(t, h, seed{year: 1405, month: 7, day: 12, amount: 22_000})
	seedTx(t, h, seed{year: 1405, month: 7, day: 10, amount: 11_000})
	v := openList(t, h)
	row := markupData(t, v.msg.ReplyMarkup, v.rows()[0])
	if _, err := h.Store.DeleteTransaction(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	v.tapData(row)

	if got := v.rows(); len(got) != 1 {
		t.Errorf("rows = %v", got)
	}
	if v.lastToast() != "This Transaction was removed." {
		t.Errorf("toast = %q", v.lastToast())
	}
}

func TestTransactionsDeletingTheLastRowOfAPageShowsAValidPage(t *testing.T) {
	h := bottest.New(t)
	for i := range 11 {
		seedTx(t, h, seed{year: 1405, month: 7, day: 1 + i, amount: int64(1000 + i)})
	}
	v := openList(t, h)
	v.tap("Older")
	v.tap(v.rows()[0])
	v.tap("Delete")
	v.tap("Yes")
	if got := v.rows(); len(got) != 10 || !strings.Contains(v.msg.Text, "page 1 of 1") {
		t.Errorf("after deleting the only row of page 2: %d rows, %q", len(got), v.msg.Text)
	}
}

func TestTransactionsStaleButtonsAnswerExpired(t *testing.T) {
	h := bottest.New(t)
	seedTx(t, h, seed{year: 1405, month: 7, day: 12, amount: 22_000})
	for i := range 11 {
		seedTx(t, h, seed{year: 1405, month: 7, day: 1 + i, amount: int64(5000 + i)})
	}
	v := openList(t, h)
	older := markupData(t, v.msg.ReplyMarkup, "Older")
	row := markupData(t, v.msg.ReplyMarkup, v.rows()[0])
	filter := markupData(t, v.msg.ReplyMarkup, "All")
	h.Restart()
	edits := len(h.Bale.Edits())

	for name, data := range map[string]string{"Older": older, "row": row, "filter": filter, "back": "l:b"} {
		v.tapData(data)
		if got := v.lastToast(); got != "This list expired, send /transactions" {
			t.Errorf("%s: toast = %q", name, got)
		}
	}
	if len(h.Bale.Edits()) != edits {
		t.Error("a stale button still edited the message")
	}
}

func TestTransactionsButtonOnAnotherMessageIsStale(t *testing.T) {
	h := bottest.New(t)
	seedTx(t, h, seed{year: 1405, month: 7, day: 12, amount: 22_000})
	v := openList(t, h)
	other := v.msg
	other.MessageID += 99
	h.Tap(other, "l:f:a")
	if a := h.Bale.Answers(); a[len(a)-1].Text != "This list expired, send /transactions" {
		t.Errorf("toast = %q", a[len(a)-1].Text)
	}
}

func TestTransactionsListsAreIndependentPerMessage(t *testing.T) {
	h := bottest.New(t)
	seedTx(t, h, seed{year: 1405, month: 7, day: 12, amount: 22_000})
	seedTx(t, h, seed{year: 1405, month: 6, day: 12, amount: 11_000})
	a := openList(t, h)
	b := openList(t, h)
	b.tap("Last month")
	if got := a.rows(); len(got) != 1 || !strings.Contains(got[0], "22,000") {
		t.Errorf("first list = %v", got)
	}
	a.tap("All")
	if got := b.rows(); len(got) != 1 || !strings.Contains(got[0], "11,000") {
		t.Errorf("second list = %v", got)
	}
}

func TestTransactionsListMovesWithTheMonth(t *testing.T) {
	h := bottest.New(t)
	seedTx(t, h, seed{year: 1405, month: 7, day: 12, amount: 22_000})
	h.Clock.Advance(20 * 24 * time.Hour) // into Aban
	v := openList(t, h)
	if len(v.rows()) != 0 || !strings.Contains(v.msg.Text, "Aban 1405") {
		t.Errorf("text %q rows %v", v.msg.Text, v.rows())
	}
	v.tap("Last month")
	if len(v.rows()) != 1 {
		t.Errorf("last month rows = %v", v.rows())
	}
}

func TestTransactionsIsListedInHelp(t *testing.T) {
	h := bottest.New(t)
	h.SendText("/help")
	if !strings.Contains(h.LastSent().Text, "/transactions") {
		t.Errorf("help = %q", h.LastSent().Text)
	}
}
