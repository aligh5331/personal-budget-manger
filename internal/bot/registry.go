package bot

import (
	"context"
	"fmt"
	"sort"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
)

// Features plug into the core by registering handlers from an init function
// in their own file (cmd_report.go, cb_transaction.go, ...), so tickets do not
// edit a shared switch statement.

// Command is a slash command.
type Command struct {
	// Name without the slash, e.g. "report".
	Name string
	// Help is the one-line description shown in /help.
	Help string
	// Order sorts /help; lower first. Leave gaps (10, 20, ...).
	Order int
	// Hidden commands work but are not listed in /help (e.g. /start).
	Hidden bool
	// Run handles the command. args is the text after the command word.
	Run func(ctx context.Context, b *Bot, m *bale.Message, args string) error
}

// Callback handles inline button taps whose callback_data starts with
// Prefix + ":" (or equals Prefix). Data is the full callback_data.
// The returned toast (may be empty) is sent with answerCallbackQuery, which
// the core always calls for Owner taps.
type Callback struct {
	Prefix string
	Run    func(ctx context.Context, b *Bot, q *bale.CallbackQuery) (toast string, err error)
}

// ReplyHandler handles a Text note sent as a reply to one of the bot's
// messages (an edit prompt, a Follow-up). It returns handled=false when the
// replied-to message is not one of its own, and the next handler (finally
// the Input pipeline) gets the message.
type ReplyHandler func(ctx context.Context, b *Bot, m *bale.Message) (handled bool, err error)

// TextHandler handles a plain text message (a Text note) from the Owner.
type TextHandler func(ctx context.Context, b *Bot, m *bale.Message) error

var (
	commands    = map[string]Command{}
	callbacks   = map[string]Callback{}
	textHandler TextHandler
)

var replyHandlers []ReplyHandler

// RegisterReplyHandler adds a handler for replies to the bot's messages.
// Call it from init.
func RegisterReplyHandler(h ReplyHandler) {
	replyHandlers = append(replyHandlers, h)
}

// RegisterCommand adds a command. It panics on a duplicate name; call it from init.
func RegisterCommand(c Command) {
	if _, dup := commands[c.Name]; dup {
		panic(fmt.Sprintf("bot: command /%s registered twice", c.Name))
	}
	commands[c.Name] = c
}

// RegisterCallback adds a button handler. It panics on a duplicate prefix.
func RegisterCallback(c Callback) {
	if _, dup := callbacks[c.Prefix]; dup {
		panic(fmt.Sprintf("bot: callback prefix %q registered twice", c.Prefix))
	}
	callbacks[c.Prefix] = c
}

// SetTextHandler installs the Text note handler. It panics if one is already
// set; only the Input pipeline should call it.
func SetTextHandler(h TextHandler) {
	if textHandler != nil {
		panic("bot: text handler set twice")
	}
	textHandler = h
}

// listedCommands returns the commands shown in /help, in display order.
func listedCommands() []Command {
	var out []Command
	for _, c := range commands {
		if !c.Hidden {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Name < out[j].Name
	})
	return out
}
