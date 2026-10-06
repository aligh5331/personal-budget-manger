// Package categorize picks a Category for a Transaction. The bot core
// depends only on the Categorizer interface; Jev is the production
// implementation and categorizefake the test double.
//
// The caller decides which Categories are offered (those matching the
// Direction, plus Uncategorized as the "none" option) and what to do with a
// low-confidence pick. A Categorizer only asks the question.
package categorize

import "context"

// Categorizer picks one of options for an Input text.
type Categorizer interface {
	// Categorize returns the option the text belongs to. text is the bank
	// message plus the Owner's note, unmodified. An error means the
	// service failed (after its own retry); the caller falls back.
	Categorize(ctx context.Context, text string, options []Option) (Pick, error)
}

// Option is one Category offered to the Categorizer.
type Option struct {
	// ID is the Category id; Pick.ID is one of the offered IDs.
	ID int64
	// Name is the English name shown to the Owner.
	Name string
	// Hint is the Persian hint used for classification.
	Hint string
	// None marks the "nothing fits" option (Uncategorized).
	None bool
}

// Pick is the Categorizer's answer.
type Pick struct {
	// ID is the picked option's ID.
	ID int64
	// Confidence is 0..1.
	Confidence float64
}
