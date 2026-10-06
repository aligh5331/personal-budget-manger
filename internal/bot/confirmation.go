package bot

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/jalali"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// ConfirmationButton is a button on every Transaction confirmation. Its
// callback_data is TransactionData(id, Action); the matching handler is
// registered with RegisterTransactionAction.
type ConfirmationButton struct {
	// Order sorts the buttons left to right; leave gaps (10, 20, ...).
	Order  int
	Label  string
	Action string
	// Shown, when set, decides per Transaction whether the button appears
	// (nil: always).
	Shown func(tx storage.Transaction) bool
}

var confirmationButtons []ConfirmationButton

// RegisterConfirmationButton adds a button to Transaction confirmations.
// Call it from init.
func RegisterConfirmationButton(c ConfirmationButton) {
	confirmationButtons = append(confirmationButtons, c)
	sort.SliceStable(confirmationButtons, func(i, j int) bool {
		return confirmationButtons[i].Order < confirmationButtons[j].Order
	})
}

// sendConfirmation tells the Owner what was saved.
func (b *Bot) sendConfirmation(ctx context.Context, chatID int64, tx storage.Transaction) error {
	msg, err := b.Send(ctx, chatID, b.confirmationText(ctx, tx), confirmationMarkupFor(tx))
	if err != nil {
		return err
	}
	return b.queueRecategorize(ctx, tx, msg)
}

// confirmationText shows amount, Direction, Category, description (or the
// bank's label, marked as bank text, when the Owner wrote nothing) and the
// Jalali date.
func (b *Bot) confirmationText(ctx context.Context, tx storage.Transaction) string {
	var sb strings.Builder
	if tx.Flagged {
		fmt.Fprintf(&sb, "Saved as flagged (%s), not counted until fixed.\n", flagReasonText(tx.FlagReason))
	} else {
		sb.WriteString("Saved.\n")
	}
	amount := "amount unknown"
	if tx.AmountToman != nil {
		amount = FormatToman(*tx.AmountToman) + " toman"
	}
	fmt.Fprintf(&sb, "%s · %s\n", amount, directionText(tx))
	fmt.Fprintf(&sb, "Category: %s\n", b.categoryText(ctx, tx))
	switch {
	case tx.Description != "":
		fmt.Fprintf(&sb, "Note: %s\n", tx.Description)
	case tx.BankLabel != "":
		fmt.Fprintf(&sb, "Bank text: «%s»\n", tx.BankLabel)
	}
	fmt.Fprintf(&sb, "Date: %s", FormatDate(tx.OccurredAt))
	return sb.String()
}

func directionText(tx storage.Transaction) string {
	if tx.Flagged && tx.FlagReason == storage.FlagDirectionAmbiguous {
		return "expense or income?"
	}
	switch tx.Direction {
	case storage.DirectionIn:
		return "Income"
	case storage.DirectionInternal:
		return "Internal transfer"
	default:
		return "Expense"
	}
}

func flagReasonText(reason string) string {
	switch reason {
	case storage.FlagAmountMissing:
		return "amount missing"
	case storage.FlagDirectionAmbiguous:
		return "expense or income unclear"
	case storage.FlagFollowupTimeout:
		return "no answer"
	case storage.FlagFollowupUnparsed:
		return "answer not understood"
	}
	return reason
}

// confirmationMarkupFor is the confirmation buttons that apply to tx.
func confirmationMarkupFor(tx storage.Transaction) *bale.InlineKeyboardMarkup {
	return confirmationRow(tx.ID, func(c ConfirmationButton) bool { return c.Shown == nil || c.Shown(tx) })
}

func confirmationRow(id int64, keep func(ConfirmationButton) bool) *bale.InlineKeyboardMarkup {
	row := make([]bale.InlineKeyboardButton, 0, len(confirmationButtons))
	for _, c := range confirmationButtons {
		if keep(c) {
			row = append(row, bale.InlineKeyboardButton{Text: c.Label, CallbackData: TransactionData(id, c.Action)})
		}
	}
	if len(row) == 0 {
		return nil
	}
	return &bale.InlineKeyboardMarkup{InlineKeyboard: [][]bale.InlineKeyboardButton{row}}
}

// FormatToman renders whole toman with thousands separators: 1,240,000.
func FormatToman(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var out []byte
	for i := range len(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// FormatDate renders t in Tehran as "4 Mehr 1405, 12:05", leaving the time
// out at midnight (a bank date with no time).
func FormatDate(t time.Time) string {
	t = t.In(clock.Tehran())
	s := jalali.Format(t)
	if t.Hour() != 0 || t.Minute() != 0 {
		s += t.Format(", 15:04")
	}
	return s
}
