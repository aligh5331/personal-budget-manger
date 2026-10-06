package storage

import "context"

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
func CategoryKindFor(direction string) (kind string, ok bool) {
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
}
