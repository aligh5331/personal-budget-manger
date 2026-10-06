package bot

import (
	"context"

	"github.com/aligh5331/personal-budget-manger/internal/categorize"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// The Category step of the Input pipeline (#35). It runs once a
// Transaction's Direction is known:
//
//   - internal transfers get no Category and the Categorizer is not asked;
//   - a Transaction still waiting on its amount or Direction gets
//     Uncategorized without asking (#37 runs this step after the answer);
//   - otherwise the Categorizer picks among the active Categories of the
//     Direction's kind plus Uncategorized. Below MinCategoryConfidence the
//     pick is dropped for Uncategorized (no flag, no Follow-up);
//   - when the Categorizer fails (after its own retry) the Transaction is
//     saved Uncategorized with categorize_pending set, for the background
//     re-categorize queue (#36).

// MinCategoryConfidence is the lowest confidence at which a pick is kept.
const MinCategoryConfidence = 0.7

// categorizeTransaction fills tx.CategoryID (and CategorizePending) before
// tx is saved. Only storage errors are returned.
func (b *Bot) categorizeTransaction(ctx context.Context, tx *storage.Transaction) error {
	tx.CategoryID = nil
	kind, ok := storage.CategoryKindFor(tx.Direction)
	if !ok {
		return nil
	}
	un, err := b.Categories.Uncategorized(ctx)
	if err != nil {
		return err
	}
	tx.CategoryID = &un.ID
	if !categorizable(*tx) {
		return nil
	}
	options, err := b.categoryOptions(ctx, kind, un)
	if err != nil {
		return err
	}
	pick, err := b.Categorizer.Categorize(ctx, categorizerText(*tx), options)
	if err != nil {
		b.Log.Warn("categorize failed, saved Uncategorized", "input_id", tx.InputID, "err", err)
		tx.CategorizePending = true
		return nil
	}
	name := ""
	for _, o := range options {
		if o.ID == pick.ID {
			name = o.Name
		}
	}
	kept := name != "" && pick.Confidence >= MinCategoryConfidence
	b.Log.Info("category picked", "input_id", tx.InputID, "category", name,
		"confidence", pick.Confidence, "kept", kept)
	if kept {
		id := pick.ID
		tx.CategoryID = &id
	}
	return nil
}

// categorizable reports whether the Categorizer should be asked about tx:
// its amount and Direction must be settled.
func categorizable(tx storage.Transaction) bool {
	if !tx.Flagged {
		return true
	}
	return tx.FlagReason != storage.FlagAmountMissing && tx.FlagReason != storage.FlagDirectionAmbiguous
}

// categorizerText is what the Categorizer reads: the bank message plus the
// Owner's note, unmodified. For a one-message Input that is the whole Input.
func categorizerText(tx storage.Transaction) string {
	return tx.RawText
}

// categoryOptions lists the Categories offered for kind: the active ones,
// then Uncategorized as the "none" option.
func (b *Bot) categoryOptions(ctx context.Context, kind string, un storage.Category) ([]categorize.Option, error) {
	cs, err := b.Categories.ActiveCategories(ctx, kind)
	if err != nil {
		return nil, err
	}
	options := make([]categorize.Option, 0, len(cs)+1)
	for _, c := range cs {
		options = append(options, categorize.Option{ID: c.ID, Name: c.Name, Hint: c.Hint})
	}
	return append(options, categorize.Option{ID: un.ID, Name: un.Name, Hint: un.Hint, None: true}), nil
}

// categoryText names tx's Category for the confirmation.
func (b *Bot) categoryText(ctx context.Context, tx storage.Transaction) string {
	if tx.Direction == storage.DirectionInternal {
		return "none (internal transfer)"
	}
	if tx.CategoryID != nil {
		c, ok, err := b.Categories.CategoryByID(ctx, *tx.CategoryID)
		if err != nil {
			b.Log.Warn("read category", "id", *tx.CategoryID, "err", err)
		}
		if ok {
			return c.Name
		}
	}
	return "Uncategorized"
}
