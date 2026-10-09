package storage

import (
	"context"
	"errors"
	"time"
)

// Category kinds. Expense Categories are offered for money out, income ones
// for money in. The built-in Uncategorized has KindAny and fits both.
const (
	KindExpense = "expense"
	KindIncome  = "income"
	KindAny     = "any"
)

// Category is one of the Owner's Categories.
type Category struct {
	ID int64
	// Name is the English name shown to the Owner.
	Name string
	// Hint is the Persian hint used for classification (defaults to Name).
	Hint string
	// Kind is KindExpense, KindIncome or KindAny (Uncategorized only).
	Kind string
	// Archived Categories stay on old Transactions but are never offered.
	Archived bool
	// BuiltIn is true only for Uncategorized, which cannot be edited.
	BuiltIn bool
}

// CategoryKindFor returns the Category kind matching a Transaction's
// Direction. ok is false for internal transfers, which get no Category.
func CategoryKindFor(direction Direction) (kind string, ok bool) {
	switch direction {
	case DirectionOut:
		return KindExpense, true
	case DirectionIn:
		return KindIncome, true
	}
	return "", false
}

// Categories stores the Owner's Categories. Categories are never
// hard-deleted, only archived.
type Categories interface {
	// Uncategorized returns the built-in fallback Category.
	Uncategorized(ctx context.Context) (Category, error)
	// ActiveCategories returns the non-archived Categories of kind in list
	// order, without the built-in Uncategorized.
	ActiveCategories(ctx context.Context, kind string) ([]Category, error)
	// CategoryByID returns a Category, archived or not; ok is false if
	// there is none.
	CategoryByID(ctx context.Context, id int64) (c Category, ok bool, err error)

	// ArchivedCategories returns the archived Categories in list order.
	ArchivedCategories(ctx context.Context) ([]Category, error)
	// AddCategory adds an active Category. ErrCategoryNameTaken if any
	// Category (archived or not) already has that name, ignoring case.
	AddCategory(ctx context.Context, name, hint, kind string) (Category, error)
	// RenameCategory renames a Category (its hint follows when it was the
	// same as the old name). ok is false for an unknown or built-in
	// Category. ErrCategoryNameTaken as for AddCategory.
	RenameCategory(ctx context.Context, id int64, name string) (ok bool, err error)
	// SetCategoryArchived archives or unarchives a Category. ok is false
	// for an unknown or built-in Category.
	SetCategoryArchived(ctx context.Context, id int64, archived bool) (ok bool, err error)
	// SaveCategoryPrompt records a question the bot sent while managing
	// Categories (#42).
	SaveCategoryPrompt(ctx context.Context, p CategoryPrompt) error
	// CategoryPromptByMessage returns the prompt sent as message messageID
	// in chatID; ok is false if that message is not a Category prompt.
	CategoryPromptByMessage(ctx context.Context, chatID, messageID int64) (p CategoryPrompt, ok bool, err error)
}

// ErrCategoryNameTaken is returned when a Category name is already used.
var ErrCategoryNameTaken = errors.New("category name already used")

// Category prompt actions.
const (
	CategoryPromptAdd    = "add"
	CategoryPromptRename = "rename"
)

// CategoryPrompt is a question the bot sent in /categories ("Send the new
// Category as one line ..."). The Owner's reply to PromptMessageID answers
// it. They never expire.
type CategoryPrompt struct {
	ChatID          int64
	PromptMessageID int64
	// Action is CategoryPromptAdd or CategoryPromptRename.
	Action string
	// CategoryID is the Category being renamed (0 for add).
	CategoryID int64
	// HostMessageID is the /categories list, redrawn after the answer.
	HostMessageID int64
	CreatedAt     time.Time
}
