package bot

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/jalali"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

func init() {
	RegisterCommand(Command{Name: "export", Help: "get every Transaction as a CSV file", Order: 800, Run: runExport})
}

// utf8BOM makes spreadsheet apps read the CSV as UTF-8, so Persian survives.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// exportHeader lists the CSV columns: every stored column, with each UTC time
// followed by its Jalali date in Tehran, and the Category name after its id.
var exportHeader = []string{
	"id",
	"created_at_utc", "created_date_jalali",
	"occurred_at_utc", "occurred_date_jalali",
	"amount_toman", "direction",
	"category_id", "category",
	"description", "bank_label", "raw_text", "input_id",
	"flagged", "flag_reason", "categorize_pending",
}

func runExport(ctx context.Context, b *Bot, m *bale.Message, _ string) error {
	all, err := b.Transactions.AllTransactions(ctx)
	if err != nil {
		return err
	}
	content, err := b.exportCSV(ctx, all)
	if err != nil {
		return err
	}
	today := jalali.FromTime(b.Clock.Now().In(clock.Tehran()))
	_, err = b.Bale.SendDocument(ctx, bale.SendDocumentParams{
		ChatID:   m.Chat.ID,
		FileName: fmt.Sprintf("transactions-%04d-%02d-%02d.csv", today.Year, today.Month, today.Day),
		Content:  content,
		Caption:  fmt.Sprintf("%d transactions", len(all)),
	})
	return err
}

// exportCSV renders txs as UTF-8 CSV with a BOM and a header row.
func (b *Bot) exportCSV(ctx context.Context, txs []storage.Transaction) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write(utf8BOM)
	w := csv.NewWriter(&buf)
	if err := w.Write(exportHeader); err != nil {
		return nil, err
	}
	for _, tx := range txs {
		category, err := b.exportCategoryName(ctx, tx)
		if err != nil {
			return nil, err
		}
		if err := w.Write([]string{
			strconv.FormatInt(tx.ID, 10),
			utcTime(tx.CreatedAt), jalaliDate(tx.CreatedAt),
			utcTime(tx.OccurredAt), jalaliDate(tx.OccurredAt),
			optionalInt(tx.AmountToman), tx.Direction,
			optionalInt(tx.CategoryID), category,
			tx.Description, tx.BankLabel, tx.RawText, tx.InputID,
			boolCell(tx.Flagged), tx.FlagReason, boolCell(tx.CategorizePending),
		}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

// exportCategoryName is the Category name column of /export: the Category's
// name even when archived, empty for internal transfers.
func (b *Bot) exportCategoryName(ctx context.Context, tx storage.Transaction) (string, error) {
	if tx.CategoryID == nil {
		return "", nil
	}
	c, _, err := b.Categories.CategoryByID(ctx, *tx.CategoryID)
	return c.Name, err
}

func utcTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func jalaliDate(t time.Time) string { return jalali.FromTime(t.In(clock.Tehran())).String() }

func optionalInt(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

func boolCell(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
