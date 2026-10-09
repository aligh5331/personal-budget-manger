package bot

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/inputrules"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// Follow-ups (#37). When an Input lacks an amount, or its Direction is
// ambiguous, the draft is not saved yet: it waits in the persisted
// followups table while the bot asks one question.
//
//   - amount: free text, answered by replying to the question;
//   - Direction: [Expense] [Income] [Internal] buttons (a reply with one of
//     those words also works).
//
// Whatever happens, the Input is never lost. If the Owner does not answer
// within FollowUpTTL, sends a new Input, or answers in a way that can't be
// read, the draft is saved as a Flagged transaction. A batch Input queues
// its Follow-ups and asks them one at a time. Expired rows are closed by
// ExpireFollowUps, which runs at startup and from RunFollowUpTimer.
//
// #38 [Fix] reuses the engine for an existing Flagged transaction through
// OpenFollowUp.

// FollowUpTTL is how long a question stays open.
const FollowUpTTL = 30 * time.Minute

const (
	followUpPrefix = "f"
	toastClosed    = "This question is closed."
)

const (
	leadTimeout  = "No answer, so I saved it as flagged. It stays out of totals until it is fixed."
	leadMovedOn  = "You moved on, so I saved your earlier message as flagged. It stays out of totals until it is fixed."
	leadUnparsed = "I couldn't read that, so I saved it as flagged. It stays out of totals until it is fixed."
)

func init() {
	RegisterReplyHandler(handleFollowUpReply)
	RegisterCallback(Callback{Prefix: followUpPrefix, Run: runFollowUpButton})
}

// queueFollowUp stores a draft Transaction that needs one more answer. tx
// is the Flagged placeholder saved if the Owner never answers. The question
// is asked by askNextFollowUp.
func (b *Bot) queueFollowUp(ctx context.Context, chatID int64, tx storage.Transaction, field string, hasTime, directionUnclear bool) error {
	b.followMu.Lock()
	defer b.followMu.Unlock()
	_, err := b.FollowUps.SaveFollowUp(ctx, storage.FollowUp{
		ChatID: chatID, Field: field, Draft: tx, HasTime: hasTime,
		DirectionUnclear: directionUnclear, CreatedAt: b.Clock.Now(),
	})
	return err
}

// AskNextFollowUp asks the next queued question of the chat unless one is
// already open.
func (b *Bot) askNextFollowUp(ctx context.Context, chatID int64) error {
	b.followMu.Lock()
	defer b.followMu.Unlock()
	return b.askNextLocked(ctx, chatID)
}

func (b *Bot) askNextLocked(ctx context.Context, chatID int64) error {
	open, err := b.FollowUps.OpenFollowUps(ctx, chatID)
	if err != nil {
		return err
	}
	var next *storage.FollowUp
	for i, f := range open {
		if f.TransactionID != 0 {
			continue
		}
		if f.Asked() {
			return nil
		}
		if next == nil {
			next = &open[i]
		}
	}
	if next == nil {
		return nil
	}
	return b.askLocked(ctx, *next)
}

// askLocked sends the question of f and records its message id.
func (b *Bot) askLocked(ctx context.Context, f storage.FollowUp) error {
	text, markup := b.followUpQuestion(ctx, f)
	msg, err := b.Send(ctx, f.ChatID, text, markup)
	if err != nil {
		return err
	}
	return b.FollowUps.MarkFollowUpAsked(ctx, f.ID, msg.MessageID, b.Clock.Now().Add(FollowUpTTL))
}

// OpenFollowUp opens a Follow-up for an existing Flagged transaction (the
// [Fix] button of /flagged, #38): it asks for the missing field (the amount
// if there is none, else the Direction) and, once answered, clears the flag,
// runs the Category step and sends the confirmation. An unreadable answer or
// no answer leaves the Transaction as it is. ok is false when there is
// nothing to ask: the Transaction is gone, not Flagged, or already has an
// open Follow-up.
func (b *Bot) OpenFollowUp(ctx context.Context, chatID, transactionID int64) (ok bool, err error) {
	b.followMu.Lock()
	defer b.followMu.Unlock()
	tx, found, err := b.Transactions.TransactionByID(ctx, transactionID)
	if err != nil || !found || !tx.Flagged {
		return false, err
	}
	if _, open, err := b.FollowUps.FollowUpForTransaction(ctx, transactionID); err != nil || open {
		return false, err
	}
	field := storage.FollowUpDirection
	if tx.AmountToman == nil {
		field = storage.FollowUpAmount
	}
	f := storage.FollowUp{ChatID: chatID, Field: field, TransactionID: transactionID, Draft: tx, CreatedAt: b.Clock.Now()}
	if f.ID, err = b.FollowUps.SaveFollowUp(ctx, f); err != nil {
		return false, err
	}
	return true, b.askLocked(ctx, f)
}

// followUpQuestion words the question: the field, what the bot understood
// and what reply it expects.
func (b *Bot) followUpQuestion(_ context.Context, f storage.FollowUp) (string, *bale.InlineKeyboardMarkup) {
	tx := f.Draft
	understood := understoodText(tx)
	if f.Field == storage.FollowUpAmount {
		return fmt.Sprintf("I read %s but couldn't find the amount. How much, in toman? "+
			"Reply to this message with a number like 450,000 or ۴۵۰ هزار.", understood), nil
	}
	summary := ""
	if tx.AmountToman != nil {
		summary = FormatToman(*tx.AmountToman) + " toman, "
	}
	text := fmt.Sprintf("I read %s%s (%s) but can't tell whether the money went out or came in. "+
		"Tap one: Expense (spent), Income (received) or Internal (between your own accounts).",
		summary, FormatDate(tx.OccurredAt), understood)
	row := make([]bale.InlineKeyboardButton, 0, len(directionChoices))
	for _, c := range directionChoices {
		row = append(row, bale.InlineKeyboardButton{Text: c.label, CallbackData: BuildCallbackData(followUpPrefix, f.ID, string(c.dir))})
	}
	return text, &bale.InlineKeyboardMarkup{InlineKeyboard: [][]bale.InlineKeyboardButton{row}}
}

// understoodText is what the bot read of the Input: the Owner's note, else
// the bank's label, else the start of the raw text.
func understoodText(tx storage.Transaction) string {
	s := tx.Description
	if s == "" {
		s = tx.BankLabel
	}
	if s == "" {
		s = strings.Join(strings.Fields(tx.RawText), " ")
		if utf8.RuneCountInString(s) > 60 {
			s = string([]rune(s)[:60]) + "..."
		}
	}
	return "«" + s + "»"
}

// handleFollowUpReply takes a reply to a Follow-up question as its answer.
func handleFollowUpReply(ctx context.Context, b *Bot, m *bale.Message) (bool, error) {
	if b.FollowUps == nil {
		return false, nil
	}
	f, ok, err := b.FollowUps.FollowUpByPrompt(ctx, m.Chat.ID, m.ReplyToMessage.MessageID)
	if err != nil || !ok {
		return false, err
	}
	b.followMu.Lock()
	defer b.followMu.Unlock()
	// Re-read: the timer may have closed it while we waited for the lock.
	f, ok, err = b.FollowUps.FollowUpByID(ctx, f.ID)
	if err != nil || !ok {
		return true, err
	}
	if !b.Clock.Now().Before(f.ExpiresAt) {
		return true, b.giveUpLocked(ctx, f, storage.FlagFollowupTimeout, leadTimeout)
	}
	switch f.Field {
	case storage.FollowUpAmount:
		if amount, ok := inputrules.TypedAmount(m.Text); ok {
			return true, b.completeLocked(ctx, f, &amount, "")
		}
	case storage.FollowUpDirection:
		if dir, ok := parseDirectionReply(m.Text); ok {
			return true, b.completeLocked(ctx, f, nil, dir)
		}
	}
	return true, b.giveUpLocked(ctx, f, storage.FlagFollowupUnparsed, leadUnparsed)
}

// parseDirectionReply reads a typed answer to a Direction question.
func parseDirectionReply(text string) (storage.Direction, bool) {
	switch strings.ToLower(strings.TrimSpace(inputrules.Normalize(text))) {
	case "expense", "spent", "out", "هزینه", "خرج", "پرداخت":
		return storage.DirectionOut, true
	case "income", "received", "in", "درآمد", "دریافت", "واریز":
		return storage.DirectionIn, true
	case "internal", "داخلی":
		return storage.DirectionInternal, true
	}
	return "", false
}

// runFollowUpButton handles f:<id>:<direction>.
func runFollowUpButton(ctx context.Context, b *Bot, q *bale.CallbackQuery) (string, error) {
	data := ParseCallbackData(q.Data)
	id, ok := data.Int(1)
	if data.Len() < 3 || !ok || id <= 0 {
		return staleButtonToast, nil
	}
	dir := storage.Direction(data.Rest(2))
	if dir != storage.DirectionOut && dir != storage.DirectionIn && dir != storage.DirectionInternal {
		return staleButtonToast, nil
	}
	b.followMu.Lock()
	defer b.followMu.Unlock()
	f, ok, err := b.FollowUps.FollowUpByID(ctx, id)
	if err != nil || !ok || f.Field != storage.FollowUpDirection {
		return toastClosed, err
	}
	if !b.Clock.Now().Before(f.ExpiresAt) {
		return "Too late", b.giveUpLocked(ctx, f, storage.FlagFollowupTimeout, leadTimeout)
	}
	if err := b.completeLocked(ctx, f, nil, dir); err != nil {
		return "Couldn't save it, try again.", err
	}
	return "Saved", nil
}

// completeLocked applies the Owner's answer (an amount or a Direction),
// saves the Transaction, confirms it and asks the next queued question.
func (b *Bot) completeLocked(ctx context.Context, f storage.FollowUp, amount *int64, direction storage.Direction) error {
	tx := f.Draft
	if f.TransactionID == 0 {
		tx.CreatedAt = b.Clock.Now()
	} else {
		stored, ok, err := b.Transactions.TransactionByID(ctx, f.TransactionID)
		if err != nil {
			return err
		}
		if !ok {
			return b.closeLocked(ctx, f) // deleted meanwhile
		}
		tx = stored
	}
	if amount != nil {
		tx.AmountToman = amount
	}
	if direction != "" {
		tx.Direction = direction
	}
	tx.Flagged, tx.FlagReason = false, ""
	if f.DirectionUnclear && amount != nil {
		// At most one Follow-up per Transaction: the Direction stays open.
		tx.Flagged, tx.FlagReason = true, storage.FlagDirectionAmbiguous
	}
	if f.TransactionID != 0 {
		if err := b.categorizeTransaction(ctx, &tx); err != nil {
			return err
		}
		if updated, err := b.Transactions.UpdateTransaction(ctx, tx); err != nil {
			return err
		} else if !updated {
			return b.closeLocked(ctx, f)
		}
		if err := b.sendConfirmations(ctx, f.ChatID, []storage.Transaction{tx}); err != nil {
			return err
		}
		return b.closeLocked(ctx, f)
	}
	saved, held, err := b.saveDraft(ctx, tx, f.HasTime)
	if err != nil {
		return err
	}
	if held {
		err = b.replyDuplicate(ctx, f.ChatID, saved)
	} else {
		err = b.sendConfirmations(ctx, f.ChatID, []storage.Transaction{saved})
	}
	if err != nil {
		return err
	}
	return b.closeLocked(ctx, f)
}

// giveUpLocked ends a Follow-up the Owner did not answer. A draft is saved
// Flagged with reason; an existing Flagged transaction is left as it is.
func (b *Bot) giveUpLocked(ctx context.Context, f storage.FollowUp, reason storage.FlagReason, lead string) error {
	if f.TransactionID != 0 {
		if err := b.closeLocked(ctx, f); err != nil {
			return err
		}
		_, err := b.Send(ctx, f.ChatID, "Left it flagged, nothing changed.", nil)
		return err
	}
	tx, err := b.saveFlaggedDraft(ctx, f, reason)
	if err != nil {
		return err
	}
	if err := b.sendFlagged(ctx, f.ChatID, lead, []storage.Transaction{tx}); err != nil {
		return err
	}
	return b.closeLocked(ctx, f)
}

// saveFlaggedDraft saves f's draft as a Flagged transaction.
func (b *Bot) saveFlaggedDraft(ctx context.Context, f storage.FollowUp, reason storage.FlagReason) (storage.Transaction, error) {
	tx := f.Draft
	tx.Flagged, tx.FlagReason = true, reason
	tx.CreatedAt = b.Clock.Now()
	saved, _, err := b.saveDraft(ctx, tx, f.HasTime)
	return saved, err
}

// sendFlagged sends the confirmation of Flagged transactions with lead on top.
func (b *Bot) sendFlagged(ctx context.Context, chatID int64, lead string, txs []storage.Transaction) error {
	text, markup := b.renderConfirmations(ctx, txs)
	_, err := b.Send(ctx, chatID, lead+"\n\n"+text, markup)
	return err
}

// closeLocked removes f and takes the buttons off its question.
func (b *Bot) closeLocked(ctx context.Context, f storage.FollowUp) error {
	if _, err := b.FollowUps.CloseFollowUp(ctx, f.ID); err != nil {
		return err
	}
	if f.Asked() && f.Field == storage.FollowUpDirection {
		text, _ := b.followUpQuestion(ctx, f)
		if err := b.Bale.EditMessageText(ctx, bale.EditMessageTextParams{
			ChatID: f.ChatID, MessageID: f.PromptMessageID, Text: text,
		}); err != nil {
			b.Log.Warn("follow-up: could not remove buttons", "err", err)
		}
	}
	return b.askNextLocked(ctx, f.ChatID)
}

// closeOpenFollowUps is the "new Input" step: every open Follow-up of the
// chat, asked or queued, ends as a Flagged transaction with its original
// reason, reported in one message. Questions about existing Transactions
// are dropped.
func (b *Bot) closeOpenFollowUps(ctx context.Context, chatID int64) error {
	b.followMu.Lock()
	defer b.followMu.Unlock()
	open, err := b.FollowUps.OpenFollowUps(ctx, chatID)
	if err != nil || len(open) == 0 {
		return err
	}
	var flagged []storage.Transaction
	for _, f := range open {
		if f.TransactionID == 0 {
			tx, err := b.saveFlaggedDraft(ctx, f, f.Draft.FlagReason)
			if err != nil {
				return err
			}
			flagged = append(flagged, tx)
		}
		// Close without asking the next question: nothing is left to ask.
		if _, err := b.FollowUps.CloseFollowUp(ctx, f.ID); err != nil {
			return err
		}
		if f.Asked() && f.Field == storage.FollowUpDirection {
			text, _ := b.followUpQuestion(ctx, f)
			if err := b.Bale.EditMessageText(ctx, bale.EditMessageTextParams{
				ChatID: f.ChatID, MessageID: f.PromptMessageID, Text: text,
			}); err != nil {
				b.Log.Warn("follow-up: could not remove buttons", "err", err)
			}
		}
	}
	if len(flagged) == 0 {
		return nil
	}
	return b.sendFlagged(ctx, chatID, leadMovedOn, flagged)
}

// ExpireFollowUps turns every Follow-up past its 30 minutes into a Flagged
// transaction and asks the next queued question where none is open. The app
// calls it at startup (rows that expired during downtime) and from
// RunFollowUpTimer.
func (b *Bot) ExpireFollowUps(ctx context.Context) error {
	b.followMu.Lock()
	defer b.followMu.Unlock()
	expired, err := b.FollowUps.ExpiredFollowUps(ctx, b.Clock.Now())
	if err != nil {
		return err
	}
	var first error
	keep := func(err error) {
		if err != nil && first == nil {
			first = err
		}
	}
	for _, f := range expired {
		keep(b.giveUpLocked(ctx, f, storage.FlagFollowupTimeout, leadTimeout))
	}
	queued, err := b.FollowUps.NextQueuedFollowUps(ctx)
	keep(err)
	for _, f := range queued {
		keep(b.askLocked(ctx, f))
	}
	return first
}

// RunFollowUpTimer expires Follow-ups every 30 seconds until ctx ends.
func (b *Bot) RunFollowUpTimer(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := b.ExpireFollowUps(ctx); err != nil {
				b.Log.Warn("expire follow-ups", "err", err)
			}
		}
	}
}
