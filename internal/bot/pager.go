package bot

import (
	"context"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// Paging shared by /transactions and /flagged: newest first, a fixed page
// size, "Newer" and "Older" buttons.

// transactionPage is one page of a filtered list.
type transactionPage struct {
	Txs []storage.Transaction
	// Page is the page shown: the one asked for, clamped to the pages that
	// exist (the last page after a delete empties the one asked for).
	Page  int
	Pages int
	Total int
}

// listPage fetches page of the Transactions matching f. A page past the end
// is refetched as the last page.
func (b *Bot) listPage(ctx context.Context, f storage.TransactionFilter, size, page int) (transactionPage, error) {
	p := transactionPage{Page: max(page, 0)}
	var err error
	p.Txs, p.Total, err = b.Transactions.ListTransactions(ctx, f, size, p.Page*size)
	if err != nil {
		return p, err
	}
	p.Pages = (p.Total + size - 1) / size
	if p.Page > 0 && p.Page >= p.Pages {
		p.Page = max(p.Pages-1, 0)
		if p.Txs, p.Total, err = b.Transactions.ListTransactions(ctx, f, size, p.Page*size); err != nil {
			return p, err
		}
	}
	return p, nil
}

// navRow returns the [Newer] [Older] buttons for the page, or nil when
// there is one page. data builds the callback_data that goes to a page.
func (p transactionPage) navRow(data func(page int) string) []bale.InlineKeyboardButton {
	var nav []bale.InlineKeyboardButton
	if p.Page > 0 {
		nav = append(nav, bale.InlineKeyboardButton{Text: "Newer", CallbackData: data(p.Page - 1)})
	}
	if p.Page+1 < p.Pages {
		nav = append(nav, bale.InlineKeyboardButton{Text: "Older", CallbackData: data(p.Page + 1)})
	}
	return nav
}
