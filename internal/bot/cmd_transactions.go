package bot

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/jalali"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// /transactions (#40): this month's Transactions, 10 per page, newest first,
// with filter buttons. A row opens the record view (cb_list.go). The page
// and filter a list message shows live in memory, keyed by the message
// (listStore); after a restart its buttons answer "This list expired".

func init() {
	RegisterCommand(Command{Name: "transactions", Help: "browse your Transactions by month, Category or flag", Order: 30, Run: runTransactions})
}

const (
	listPageSize  = 10
	maxListStates = 200
	maxRowDesc    = 24
)

// List filters. A filter is a code in callback_data ("l:f:<code>").
const (
	filterThisMonth = "m"
	filterLastMonth = "l"
	filterAll       = "a"
	filterFlagged   = "x"
	filterCategory  = "c" // with listState.categoryID
)

// listState is what one list message shows.
type listState struct {
	filter     string
	categoryID int64
	page       int
}

type listKey struct{ chatID, messageID int64 }

// listStore keeps the most recent maxListStates list messages.
type listStore struct {
	states map[listKey]*listState
	order  []listKey
}

func (s *listStore) get(chatID, messageID int64) (*listState, bool) {
	if s == nil {
		return nil, false
	}
	st, ok := s.states[listKey{chatID, messageID}]
	return st, ok
}

func (s *listStore) put(chatID, messageID int64, st *listState) {
	k := listKey{chatID, messageID}
	if _, ok := s.states[k]; !ok {
		s.order = append(s.order, k)
		if len(s.order) > maxListStates {
			delete(s.states, s.order[0])
			s.order = s.order[1:]
		}
	}
	s.states[k] = st
}

func (b *Bot) listStates() *listStore {
	if b.lists == nil {
		b.lists = &listStore{states: map[listKey]*listState{}}
	}
	return b.lists
}

func runTransactions(ctx context.Context, b *Bot, m *bale.Message, _ string) error {
	st := &listState{filter: filterThisMonth}
	text, markup, err := b.renderList(ctx, st)
	if err != nil {
		return err
	}
	sent, err := b.Send(ctx, m.Chat.ID, text, markup)
	if err != nil {
		return err
	}
	b.listStates().put(m.Chat.ID, sent.MessageID, st)
	return nil
}

// listFilter turns the state into a storage filter and a title.
func (b *Bot) listFilter(ctx context.Context, st *listState) (storage.TransactionFilter, string, error) {
	loc := clock.Tehran()
	y, m := jalali.MonthContaining(b.Clock.Now().In(loc))
	switch st.filter {
	case filterLastMonth:
		y, m = jalali.AddMonths(y, m, -1)
		fallthrough
	case filterThisMonth:
		from, to := jalali.MonthRange(y, m, loc)
		title := "This month"
		if st.filter == filterLastMonth {
			title = "Last month"
		}
		return storage.TransactionFilter{From: from, To: to}, fmt.Sprintf("%s (%s %d)", title, jalali.MonthName(m), y), nil
	case filterFlagged:
		return storage.TransactionFilter{FlaggedOnly: true}, "Flagged", nil
	case filterCategory:
		c, ok, err := b.Categories.CategoryByID(ctx, st.categoryID)
		if err != nil {
			return storage.TransactionFilter{}, "", err
		}
		name := "unknown"
		if ok {
			name = c.Name
		}
		id := st.categoryID
		return storage.TransactionFilter{CategoryID: &id}, "Category: " + name, nil
	}
	return storage.TransactionFilter{}, "All", nil
}

// renderList returns the list message for st, clamping st.page to the pages
// that exist.
func (b *Bot) renderList(ctx context.Context, st *listState) (string, *bale.InlineKeyboardMarkup, error) {
	f, title, err := b.listFilter(ctx, st)
	if err != nil {
		return "", nil, err
	}
	p, err := b.listPage(ctx, f, listPageSize, st.page)
	if err != nil {
		return "", nil, err
	}
	st.page = p.Page

	var sb strings.Builder
	fmt.Fprintf(&sb, "Transactions · %s\n", title)
	if p.Total == 0 {
		sb.WriteString("Nothing here.")
	} else {
		fmt.Fprintf(&sb, "%d in all · page %d of %d", p.Total, p.Page+1, p.Pages)
	}

	var rows [][]bale.InlineKeyboardButton
	for _, tx := range p.Txs {
		rows = append(rows, []bale.InlineKeyboardButton{{
			Text:         b.rowLabel(ctx, tx),
			CallbackData: BuildCallbackData(listPrefix, "r", tx.ID),
		}})
	}
	nav := p.navRow(func(page int) string { return BuildCallbackData(listPrefix, "p", page) })
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	mark := func(label, code string) bale.InlineKeyboardButton {
		if st.filter == code {
			label = currentMarker + label
		}
		data := BuildCallbackData(listPrefix, "f", code)
		if code == filterCategory {
			data = BuildCallbackData(listPrefix, "c")
		}
		return bale.InlineKeyboardButton{Text: label, CallbackData: data}
	}
	rows = append(rows,
		[]bale.InlineKeyboardButton{mark("This month", filterThisMonth), mark("Last month", filterLastMonth), mark("All", filterAll)},
		[]bale.InlineKeyboardButton{mark("Flagged", filterFlagged), mark("Category", filterCategory)},
	)
	return sb.String(), &bale.InlineKeyboardMarkup{InlineKeyboard: rows}, nil
}

// rowLabel is the button text of a list row: "12 Mehr · 450,000 · Food ·
// Snacks". Income amounts get a plus sign; a Flagged row ends with "flagged".
func (b *Bot) rowLabel(ctx context.Context, tx storage.Transaction) string {
	loc := clock.Tehran()
	d := jalali.FromTime(tx.OccurredAt.In(loc))
	date := fmt.Sprintf("%d %s", d.Day, jalali.MonthName(d.Month))
	if nowY, _ := jalali.MonthContaining(b.Clock.Now().In(loc)); d.Year != nowY {
		date += fmt.Sprintf(" %d", d.Year)
	}
	amount := "?"
	if tx.AmountToman != nil {
		amount = FormatToman(*tx.AmountToman)
	}
	if tx.Direction == storage.DirectionIn {
		amount = "+" + amount
	}
	parts := []string{date, amount}
	if tx.Direction == storage.DirectionInternal {
		parts = append(parts, "Internal")
	} else {
		parts = append(parts, b.categoryText(ctx, tx))
	}
	if tx.Description != "" {
		parts = append(parts, truncateRunes(tx.Description, maxRowDesc))
	}
	if tx.Flagged {
		parts = append(parts, "flagged")
	}
	return strings.Join(parts, " · ")
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
