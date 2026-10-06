package bot

import (
	"context"
	"strings"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
)

func init() {
	RegisterCommand(Command{Name: "help", Help: "show this help", Order: 1000, Run: runHelp})
	RegisterCommand(Command{Name: "start", Hidden: true, Run: runHelp})
}

const inputConvention = `To record money spent or received, forward the bank message to me, then optionally add a short note in your own words (e.g. "ناهار").
For cash, send just the note with the amount (e.g. "تاکسی ۸۰ تومن").`

// commandList is the "Commands:" block shared by /help and unknown commands.
func commandList() string {
	var sb strings.Builder
	sb.WriteString("Commands:")
	for _, c := range listedCommands() {
		sb.WriteString("\n/" + c.Name + " - " + c.Help)
	}
	return sb.String()
}

func (b *Bot) helpText() string {
	return inputConvention + "\n\n" + commandList() + "\n\nVersion " + b.Version
}

func runHelp(ctx context.Context, b *Bot, m *bale.Message, _ string) error {
	return b.Reply(ctx, m, b.helpText(), nil)
}

func runUnknownCommand(ctx context.Context, b *Bot, m *bale.Message) error {
	return b.Reply(ctx, m, "Unknown command.\n\n"+commandList(), nil)
}
