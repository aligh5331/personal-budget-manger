package bot

import (
	"context"
	"fmt"
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
	p, err := b.listPage(ctx, storage.TransactionFilter{FlaggedOnly: true}, flaggedPageSize, page)
	if err != nil {
		return "", nil, err
	}
	if p.Total == 0 {
		return "Nothing is flagged.", nil, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Flagged · %d in all · page %d of %d\nThey stay out of totals until fixed.", p.Total, p.Page+1, p.Pages)
	var rows [][]bale.InlineKeyboardButton
	for i, tx := range p.Txs {
		fmt.Fprintf(&sb, "\n\n%d. %s", i+1, flaggedLine(tx))
		rows = append(rows, []bale.InlineKeyboardButton{
			{Text: fmt.Sprintf("Fix %d", i+1), CallbackData: BuildCallbackData(flaggedPrefix, "x", tx.ID)},
			{Text: fmt.Sprintf("Delete %d", i+1), CallbackData: BuildCallbackData(flaggedPrefix, "d", tx.ID, p.Page)},
		})
	}
	nav := p.navRow(func(page int) string { return BuildCallbackData(flaggedPrefix, "p", page) })
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
	data := ParseCallbackData(q.Data)
	n, ok := data.Int(2)
	if !ok {
		return "", nil
	}
	switch data.Part(1) {
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
		page, _ := data.Int(3)
		if _, err := b.Transactions.DeleteTransaction(ctx, n); err != nil {
			return "Couldn't delete it, try again.", err
		}
		return "Deleted", b.showFlagged(ctx, q, int(page))
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
