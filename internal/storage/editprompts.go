package storage

import (
	"context"
	"time"
)

// Edit fields the Owner answers by typing (#39).
const (
	EditFieldAmount      = "amount"
	EditFieldDate        = "date"
	EditFieldDescription = "description"
)

// EditPrompt is a question the bot sent while the Owner edits a
// Transaction ("New amount ...?"). The Owner's reply to PromptMessageID is
// the new value of Field.
type EditPrompt struct {
	ChatID          int64
	PromptMessageID int64
	TransactionID   int64
	// Field is one of the EditField constants.
	Field string
	// View is the key of the view the edit started from ("" is the
	// confirmation); HostMessageID is that message, redrawn after the edit.
	View          string
	HostMessageID int64
	CreatedAt     time.Time
}

// EditPrompts stores open edit prompts. They never expire: a reply to an
// old prompt still edits its Transaction.
type EditPrompts interface {
	// SaveEditPrompt records a prompt the bot just sent.
	SaveEditPrompt(ctx context.Context, p EditPrompt) error
	// EditPromptByMessage returns the prompt sent as message messageID in
	// chatID; ok is false if that message is not an edit prompt.
	EditPromptByMessage(ctx context.Context, chatID, messageID int64) (p EditPrompt, ok bool, err error)
}
