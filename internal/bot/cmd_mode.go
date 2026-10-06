package bot

import (
	"context"
	"strings"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/config"
)

// modePrefix is the callback prefix of the /mode switch button:
// "mode:webhook" or "mode:polling" (the mode to switch to).
const modePrefix = "mode"

func init() {
	RegisterCommand(Command{Name: "mode", Help: "show or switch how updates arrive (webhook or polling)", Order: 900, Run: runMode})
	RegisterCallback(Callback{Prefix: modePrefix, Run: tapMode})
}

const modeUnavailable = "Switching the update mode is not available."

func runMode(ctx context.Context, b *Bot, m *bale.Message, _ string) error {
	if b.Updates == nil {
		return b.Reply(ctx, m, modeUnavailable, nil)
	}
	if mode, _ := b.Updates.Mode(); mode == "" {
		return b.Reply(ctx, m, "The update source is not running yet.", nil)
	}
	text, markup := b.modeView("")
	return b.Reply(ctx, m, text, markup)
}

func tapMode(ctx context.Context, b *Bot, q *bale.CallbackQuery) (string, error) {
	if b.Updates == nil {
		return modeUnavailable, nil
	}
	_, target, _ := strings.Cut(q.Data, ":")
	if target != config.ModePolling && target != config.ModeWebhook {
		return "This button no longer works.", nil
	}
	toast := "Switched to " + target + "."
	var problem string
	if current, _ := b.Updates.Mode(); current == target {
		toast = "Already on " + target + "."
	} else if err := b.Updates.SwitchMode(ctx, target); err != nil {
		toast = "Switch failed."
		problem = "Could not switch to " + target + ": " + err.Error()
	}
	text, markup := b.modeView(problem)
	if q.Message == nil {
		_, err := b.Send(ctx, q.From.ID, text, markup)
		return toast, err
	}
	err := b.Bale.EditMessageText(ctx, bale.EditMessageTextParams{
		ChatID: q.Message.Chat.ID, MessageID: q.Message.MessageID, Text: text, ReplyMarkup: markup,
	})
	return toast, err
}

// modeView renders the /mode message: the active mode, why it differs from
// the saved choice after a fallback, an optional error, and the switch button.
func (b *Bot) modeView(problem string) (string, *bale.InlineKeyboardMarkup) {
	mode, note := b.Updates.Mode()
	other := config.ModeWebhook
	if mode == config.ModeWebhook {
		other = config.ModePolling
	}
	var sb strings.Builder
	if problem != "" {
		sb.WriteString(problem + "\n\n")
	}
	sb.WriteString("Updates arrive by " + mode + ".")
	if note != "" {
		sb.WriteString("\n(" + note + ")")
	}
	markup := &bale.InlineKeyboardMarkup{InlineKeyboard: [][]bale.InlineKeyboardButton{{
		{Text: "Switch to " + other, CallbackData: modePrefix + ":" + other},
	}}}
	return sb.String(), markup
}
