// Package bot is the bot core: it takes one Bale update at a time and decides
// what to do with it. It is wired to its outside world only through
// interfaces (bale.Client, clock.Clock, storage.*), which is what makes it
// testable through package bottest.
package bot

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/categorize"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/extract"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// Deps are the bot core's collaborators. Tickets that add a collaborator add
// one field here and wire it in internal/app and bottest.Harness.Deps.
type Deps struct {
	Bale     bale.Client
	Clock    clock.Clock
	Settings storage.Settings
	Log      *slog.Logger
	OwnerID  int64
	Version  string
	// Updates is how updates reach the bot (webhook or polling); /mode
	// shows and switches it. Nil disables /mode.
	Updates UpdateSource

	// Extractor reads Inputs (#32).
	Extractor extract.Extractor
	// Transactions stores Transactions (#32).
	Transactions storage.Transactions
	// EditPrompts stores the questions of the [Edit] flow (#39).
	EditPrompts storage.EditPrompts

	// Categorizer picks a Transaction's Category (#35).
	Categorizer categorize.Categorizer
	// Categories stores the Owner's Categories (#35).
	Categories storage.Categories
	// Retries is the background re-categorize queue (#36).
	Retries storage.CategorizeRetries
}

// UpdateSource is the switchable update source (internal/updates.Manager).
type UpdateSource interface {
	// Mode returns the active mode ("polling" or "webhook") and, when it
	// differs from the Owner's saved choice after a fallback, why.
	Mode() (mode, note string)
	// SwitchMode switches to mode and saves the choice. On failure the
	// previous mode stays active and the error says why.
	SwitchMode(ctx context.Context, mode string) error
}

// Bot is the bot core. HandleUpdate must be called from one goroutine at a
// time (the in-order worker); Bot is not safe for concurrent HandleUpdate.
type Bot struct {
	Deps

	dropLogged map[int64]time.Time // sender -> last "dropped" log line
	lists      *listStore          // /transactions list state, by message (#40)
}

// New validates deps and returns a Bot.
func New(d Deps) (*Bot, error) {
	if d.Bale == nil || d.Clock == nil || d.Settings == nil {
		return nil, errors.New("bot: Bale, Clock and Settings are required")
	}
	if d.Extractor == nil || d.Transactions == nil {
		return nil, errors.New("bot: Extractor and Transactions are required")
	}
	if d.Categorizer == nil || d.Categories == nil {
		return nil, errors.New("bot: Categorizer and Categories are required")
	}
	if d.Retries == nil {
		return nil, errors.New("bot: Retries is required")
	}
	if d.OwnerID == 0 {
		return nil, errors.New("bot: OwnerID is required")
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Bot{Deps: d, dropLogged: map[int64]time.Time{}}, nil
}

// HandleUpdate processes one update. Updates at or below the highest
// update_id already processed are skipped. The high-water mark is saved after
// the update is handled, even if handling failed, so a failing update is not
// retried forever and never saves a Transaction twice.
func (b *Bot) HandleUpdate(ctx context.Context, u bale.Update) error {
	last, seen, err := b.Settings.LastUpdateID(ctx)
	if err != nil {
		return err
	}
	if seen && u.UpdateID <= last {
		return nil
	}
	handleErr := b.dispatch(ctx, u)
	if err := b.Settings.SaveLastUpdateID(ctx, u.UpdateID); err != nil {
		return errors.Join(handleErr, err)
	}
	return handleErr
}

func (b *Bot) dispatch(ctx context.Context, u bale.Update) error {
	switch {
	case u.Message != nil:
		if !b.fromOwner(u.Message.From, &u.Message.Chat) {
			return nil
		}
		return b.handleMessage(ctx, u.Message)
	case u.CallbackQuery != nil:
		q := u.CallbackQuery
		var chat *bale.Chat
		if q.Message != nil {
			chat = &q.Message.Chat
		}
		if !b.fromOwner(&q.From, chat) {
			return nil
		}
		return b.handleCallback(ctx, q)
	default:
		// edited_message (the Owner's edits are ignored by design) and
		// update kinds the bot does not use.
		return nil
	}
}

// fromOwner reports whether an update comes from the Owner in a private
// chat. Other senders are dropped with one log line per sender per minute.
// chat may be nil for a callback on a message too old to be included.
func (b *Bot) fromOwner(from *bale.User, chat *bale.Chat) bool {
	var sender int64
	if from != nil {
		sender = from.ID
	}
	if sender == b.OwnerID && (chat == nil || chat.Type == bale.ChatPrivate) {
		return true
	}
	now := b.Clock.Now()
	if len(b.dropLogged) > 1000 {
		for id, t := range b.dropLogged {
			if now.Sub(t) >= time.Minute {
				delete(b.dropLogged, id)
			}
		}
	}
	if last, ok := b.dropLogged[sender]; !ok || now.Sub(last) >= time.Minute {
		b.dropLogged[sender] = now
		chatType := ""
		if chat != nil {
			chatType = chat.Type
		}
		b.Log.Info("dropped update from non-owner or non-private chat", "sender", sender, "chat_type", chatType)
	}
	return false
}
