package bot

import (
	"context"
	"errors"
	"strings"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
)

const textOnlyReply = "Text only for now. Send a bank message, with an optional note."

// handleMessage routes an Owner message to a command or the Text note handler.
func (b *Bot) handleMessage(ctx context.Context, m *bale.Message) error {
	if strings.TrimSpace(m.Text) == "" {
		// Voice, photo, sticker, file... v1 takes Text notes only (ADR-0001).
		return b.Reply(ctx, m, textOnlyReply, nil)
	}
	if name, args, ok := parseCommand(m.Text); ok {
		c, found := commands[name]
		if !found {
			return runUnknownCommand(ctx, b, m)
		}
		return c.Run(ctx, b, m, args)
	}
	if m.ReplyToMessage != nil {
		for _, h := range replyHandlers {
			if handled, err := h(ctx, b, m); handled || err != nil {
				return err
			}
		}
	}
	if textHandler != nil {
		return textHandler(ctx, b, m)
	}
	return b.Reply(ctx, m, b.helpText(), nil)
}

// handleCallback dispatches an Owner button tap by its callback_data prefix
// and always answers it, so the button never stays in its loading state.
func (b *Bot) handleCallback(ctx context.Context, q *bale.CallbackQuery) error {
	prefix, _, _ := strings.Cut(q.Data, ":")
	var toast string
	var runErr error
	if c, ok := callbacks[prefix]; ok {
		toast, runErr = c.Run(ctx, b, q)
	} else {
		toast = "This button no longer works."
	}
	ansErr := b.Bale.AnswerCallbackQuery(ctx, bale.AnswerCallbackQueryParams{CallbackQueryID: q.ID, Text: toast})
	return errors.Join(runErr, ansErr)
}

// parseCommand splits "/name@bot args" into ("name", "args").
func parseCommand(text string) (name, args string, ok bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	word, rest, _ := strings.Cut(text[1:], " ")
	word, _, _ = strings.Cut(word, "@")
	if word == "" {
		return "", "", false
	}
	return strings.ToLower(word), strings.TrimSpace(rest), true
}

// Reply sends text to the chat m came from.
func (b *Bot) Reply(ctx context.Context, m *bale.Message, text string, markup *bale.InlineKeyboardMarkup) error {
	_, err := b.Send(ctx, m.Chat.ID, text, markup)
	return err
}

// Send sends text to chatID and returns the sent message.
func (b *Bot) Send(ctx context.Context, chatID int64, text string, markup *bale.InlineKeyboardMarkup) (bale.Message, error) {
	return b.Bale.SendMessage(ctx, bale.SendMessageParams{ChatID: chatID, Text: text, ReplyMarkup: markup})
}
