package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// /flagged (#38): Flagged transactions, newest first, 5 per page, each with
// its raw text and reason. [Fix] re-asks the missing field through
// OpenFollowUp; [Delete] hard-deletes. The buttons carry everything they
// need, so they keep working after a restart:
//
//	fl:p:<page>        go to a page
//	fl:x:<id>          [Fix]
//	fl:d:<id>:<page>   [Delete], then redraw that page

const (
	flaggedPrefix   = "fl"
	flaggedPageSize = 5
	maxRawShown     = 300
)

func init() {
	RegisterCommand(Command{Name: "flagged", Help: "review Transactions that need fixing", Order: 35, Run: runFlagged})
	RegisterCallback(Callback{Prefix: flaggedPrefix, Run: runFlaggedCallback})
}

func runFlagged(ctx context.Context, b *Bot, m *bale.Message, _ string) error {
	text, markup, err := b.renderFlagged(ctx, 0)
	if err != nil {
		return err
	}
	_, err = b.Send(ctx, m.Chat.ID, text, markup)
	return err
}

// renderFlagged draws one page, clamped to the pages that exist.
func (b *Bot) renderFlagged(ctx context.Context, page int) (string, *bale.InlineKeyboardMarkup, error) {
	f := storage.TransactionFilter{FlaggedOnly: true}
	page = max(page, 0)
	txs, total, err := b.Transactions.ListTransactions(ctx, f, flaggedPageSize, page*flaggedPageSize)
	if err != nil {
		return "", nil, err
	}
	pages := (total + flaggedPageSize - 1) / flaggedPageSize
	if page > 0 && page >= pages {
		page = max(pages-1, 0)
		if txs, total, err = b.Transactions.ListTransactions(ctx, f, flaggedPageSize, page*flaggedPageSize); err != nil {
			return "", nil, err
		}
	}
	if total == 0 {
		return "Nothing is flagged.", nil, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Flagged · %d in all · page %d of %d\nThey stay out of totals until fixed.", total, page+1, pages)
	var rows [][]bale.InlineKeyboardButton
	for i, tx := range txs {
		fmt.Fprintf(&sb, "\n\n%d. %s", i+1, flaggedLine(tx))
		rows = append(rows, []bale.InlineKeyboardButton{
			{Text: fmt.Sprintf("Fix %d", i+1), CallbackData: fmt.Sprintf("%s:x:%d", flaggedPrefix, tx.ID)},
			{Text: fmt.Sprintf("Delete %d", i+1), CallbackData: fmt.Sprintf("%s:d:%d:%d", flaggedPrefix, tx.ID, page)},
		})
	}
	var nav []bale.InlineKeyboardButton
	if page > 0 {
		nav = append(nav, bale.InlineKeyboardButton{Text: "Newer", CallbackData: fmt.Sprintf("%s:p:%d", flaggedPrefix, page-1)})
	}
	if page+1 < pages {
		nav = append(nav, bale.InlineKeyboardButton{Text: "Older", CallbackData: fmt.Sprintf("%s:p:%d", flaggedPrefix, page+1)})
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	return sb.String(), &bale.InlineKeyboardMarkup{InlineKeyboard: rows}, nil
}

func flaggedLine(tx storage.Transaction) string {
	amount := "amount unknown"
	if tx.AmountToman != nil {
		amount = FormatToman(*tx.AmountToman) + " toman"
	}
	return fmt.Sprintf("%s · %s\nReason: %s\n«%s»",
		FormatDate(tx.OccurredAt), amount, flagReasonText(tx.FlagReason),
		truncateRunes(tx.RawText, maxRawShown))
}

func runFlaggedCallback(ctx context.Context, b *Bot, q *bale.CallbackQuery) (string, error) {
	if q.Message == nil {
		return "", nil
	}
	parts := strings.Split(q.Data, ":")
	if len(parts) < 3 {
		return "", nil
	}
	n, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", nil
	}
	switch parts[1] {
	case "p":
		return "", b.showFlagged(ctx, q, int(n))
	case "x":
		ok, err := b.OpenFollowUp(ctx, q.Message.Chat.ID, n)
		if err != nil {
			return "Something went wrong, try again.", err
		}
		if !ok {
			return "Nothing to fix here, or I'm already asking.", nil
		}
		return "Question sent", nil
	case "d":
		page := 0
		if len(parts) > 3 {
			page, _ = strconv.Atoi(parts[3])
		}
		if _, err := b.Transactions.DeleteTransaction(ctx, n); err != nil {
			return "Couldn't delete it, try again.", err
		}
		return "Deleted", b.showFlagged(ctx, q, page)
	}
	return "", nil
}

func (b *Bot) showFlagged(ctx context.Context, q *bale.CallbackQuery, page int) error {
	text, markup, err := b.renderFlagged(ctx, page)
	if err != nil {
		return err
	}
	b.editMessage(ctx, q, text, markup)
	return nil
}
