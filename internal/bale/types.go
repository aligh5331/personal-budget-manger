package bale

// Wire types for the Bale Bot API (Telegram-shaped). Only the fields the bot
// reads are declared; add fields here as features need them.

// Update is one incoming event. At most one of the pointer fields is set.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	EditedMessage *Message       `json:"edited_message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

// User is a Bale account.
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// Chat types.
const (
	ChatPrivate = "private"
)

// Chat is where a message lives.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// Message is a chat message.
type Message struct {
	MessageID      int64                 `json:"message_id"`
	From           *User                 `json:"from,omitempty"`
	Date           int64                 `json:"date"`
	Chat           Chat                  `json:"chat"`
	Text           string                `json:"text,omitempty"`
	Caption        string                `json:"caption,omitempty"`
	ForwardFrom    *User                 `json:"forward_from,omitempty"`
	ForwardDate    int64                 `json:"forward_date,omitempty"`
	ReplyToMessage *Message              `json:"reply_to_message,omitempty"`
	EditDate       int64                 `json:"edit_date,omitempty"`
	Voice          *File                 `json:"voice,omitempty"`
	Audio          *File                 `json:"audio,omitempty"`
	Document       *File                 `json:"document,omitempty"`
	Sticker        *File                 `json:"sticker,omitempty"`
	Video          *File                 `json:"video,omitempty"`
	Photo          []File                `json:"photo,omitempty"`
	ReplyMarkup    *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

// File is any file-like attachment (voice, photo size, sticker, document).
type File struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
	FilePath     string `json:"file_path,omitempty"`
}

// CallbackQuery is a tap on an inline button.
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data,omitempty"`
}

// InlineKeyboardMarkup is a grid of inline buttons.
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// InlineKeyboardButton is one inline button. CallbackData must be 1-64 bytes.
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// WebhookInfo is the result of getWebhookInfo.
type WebhookInfo struct {
	URL                string `json:"url"`
	PendingUpdateCount int    `json:"pending_update_count,omitempty"`
	LastErrorDate      int64  `json:"last_error_date,omitempty"`
	LastErrorMessage   string `json:"last_error_message,omitempty"`
}

// SendMessageParams are the arguments of sendMessage.
type SendMessageParams struct {
	ChatID           int64                 `json:"chat_id"`
	Text             string                `json:"text"`
	ReplyToMessageID int64                 `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

// EditMessageTextParams are the arguments of editMessageText.
type EditMessageTextParams struct {
	ChatID      int64                 `json:"chat_id"`
	MessageID   int64                 `json:"message_id"`
	Text        string                `json:"text"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

// AnswerCallbackQueryParams are the arguments of answerCallbackQuery.
type AnswerCallbackQueryParams struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
	ShowAlert       bool   `json:"show_alert,omitempty"`
}

// SendDocumentParams are the arguments of sendDocument (multipart upload).
type SendDocumentParams struct {
	ChatID   int64
	FileName string
	Content  []byte
	Caption  string
}

// GetUpdatesParams are the arguments of getUpdates.
type GetUpdatesParams struct {
	Offset  int64 `json:"offset,omitempty"`
	Limit   int   `json:"limit,omitempty"`
	Timeout int   `json:"timeout,omitempty"` // seconds of long poll
}
