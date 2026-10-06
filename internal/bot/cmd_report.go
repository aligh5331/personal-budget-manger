package bot

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/jalali"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// reportPrefix is the callback prefix of the [Last month] / [This month]
// buttons; the data is "rep:<jalali year><2-digit month>", e.g. "rep:140507".
const reportPrefix = "rep"

// baleMessageLimit is Bale's maximum text length in characters.
const baleMessageLimit = 4096

func init() {
	RegisterCommand(Command{Name: "report", Help: "this month's spending and income by Category", Order: 30, Run: runReport})
	RegisterCallback(Callback{Prefix: reportPrefix, Run: tapReport})
}

func runReport(ctx context.Context, b *Bot, m *bale.Message, _ string) error {
	y, mo := b.currentJalaliMonth()
	pages, markup, err := b.reportPages(ctx, y, mo)
	if err != nil {
		return err
	}
	for i, p := range pages {
		var mk *bale.InlineKeyboardMarkup
		if i == len(pages)-1 {
			mk = markup
		}
		if err := b.Reply(ctx, m, p, mk); err != nil {
			return err
		}
	}
	return nil
}

// tapReport switches the Report to the month in the button. The tapped
// message becomes the first page; any further pages follow as new messages.
func tapReport(ctx context.Context, b *Bot, q *bale.CallbackQuery) (string, error) {
	y, mo, ok := parseReportData(q.Data)
	if !ok {
		return "Unknown month", nil
	}
	pages, markup, err := b.reportPages(ctx, y, mo)
	if err != nil {
		return "Couldn't build the Report, try again.", err
	}
	if q.Message == nil {
		return "", nil
	}
	first := bale.EditMessageTextParams{ChatID: q.Message.Chat.ID, MessageID: q.Message.MessageID, Text: pages[0]}
	if len(pages) == 1 {
		first.ReplyMarkup = markup
	}
	if err := b.Bale.EditMessageText(ctx, first); err != nil {
		return "", err
	}
	for i, p := range pages[1:] {
		var mk *bale.InlineKeyboardMarkup
		if i == len(pages)-2 {
			mk = markup
		}
		if _, err := b.Send(ctx, q.Message.Chat.ID, p, mk); err != nil {
			return "", err
		}
	}
	return "", nil
}

func (b *Bot) currentJalaliMonth() (year, month int) {
	d := jalali.FromTime(b.Clock.Now().In(clock.Tehran()))
	return d.Year, d.Month
}

func reportData(y, m int) string { return reportPrefix + ":" + fmt.Sprintf("%04d%02d", y, m) }

func parseReportData(data string) (y, m int, ok bool) {
	_, v, found := strings.Cut(data, ":")
	if !found || len(v) != 6 {
		return 0, 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, 0, false
	}
	y, m = n/100, n%100
	return y, m, y >= 1 && m >= 1 && m <= 12
}

// reportPages builds the Report of Jalali month m of year y as one or more
// messages, plus the month-switching buttons (relative to today).
func (b *Bot) reportPages(ctx context.Context, y, m int) ([]string, *bale.InlineKeyboardMarkup, error) {
	start, end := jalali.MonthRange(y, m, clock.Tehran())
	sum, err := b.Transactions.ReportSummary(ctx, storage.ReportRange{From: start, To: end})
	if err != nil {
		return nil, nil, err
	}
	cy, cm := b.currentJalaliMonth()
	ly, lm := jalali.AddMonths(cy, cm, -1)
	markup := &bale.InlineKeyboardMarkup{InlineKeyboard: [][]bale.InlineKeyboardButton{{
		{Text: "Last month", CallbackData: reportData(ly, lm)},
		{Text: "This month", CallbackData: reportData(cy, cm)},
	}}}
	return splitMessage(renderReport(y, m, sum), baleMessageLimit), markup, nil
}

// renderReport lays out one month: expenses by Category largest first, then
// income, the totals, and one line each for internal transfers and Flagged
// transactions, which stay out of the totals.
func renderReport(y, m int, sum storage.ReportSummary) string {
	title := fmt.Sprintf("%s %d", jalali.MonthName(m), y)
	if len(sum.Lines) == 0 && sum.InternalCount == 0 && sum.FlaggedCount == 0 {
		return "No transactions in " + title + "."
	}
	var out, in []storage.ReportLine
	var outTotal, inTotal int64
	for _, l := range sum.Lines {
		if l.CategoryName == "" {
			l.CategoryName = "Uncategorized"
		}
		if l.Direction == storage.DirectionIn {
			in = append(in, l)
			inTotal += l.Total
		} else {
			out = append(out, l)
			outTotal += l.Total
		}
	}
	var sb strings.Builder
	sb.WriteString("Report: " + title)
	section := func(name string, lines []storage.ReportLine, total int64) {
		if len(lines) == 0 {
			return
		}
		sort.Slice(lines, func(i, j int) bool {
			if lines[i].Total != lines[j].Total {
				return lines[i].Total > lines[j].Total
			}
			return lines[i].CategoryName < lines[j].CategoryName
		})
		sb.WriteString("\n\n" + name)
		for _, l := range lines {
			fmt.Fprintf(&sb, "\n%s: %s (%d%%)", l.CategoryName, FormatToman(l.Total), percent(l.Total, total))
		}
	}
	section("Expenses", out, outTotal)
	section("Income", in, inTotal)
	if len(sum.Lines) > 0 {
		fmt.Fprintf(&sb, "\n\nExpenses: %s\nIncome: %s\nNet: %s", FormatToman(outTotal), FormatToman(inTotal), FormatToman(inTotal-outTotal))
	}
	if sum.InternalCount > 0 {
		sb.WriteString("\n\nInternal transfers: " + FormatToman(sum.InternalToman))
	}
	if sum.FlaggedCount > 0 {
		fmt.Fprintf(&sb, "\n\n%d flagged, not counted: /flagged", sum.FlaggedCount)
	}
	return sb.String()
}

// percent is part/whole as a rounded whole percent.
func percent(part, whole int64) int64 {
	if whole <= 0 {
		return 0
	}
	return (part*200 + whole) / (2 * whole)
}

// splitMessage cuts text into pieces of at most limit characters, breaking
// at line ends (and inside a line only when one line is over the limit).
func splitMessage(text string, limit int) []string {
	var pages []string
	var cur strings.Builder
	curLen := 0
	flush := func() {
		if cur.Len() > 0 {
			pages = append(pages, strings.TrimRight(cur.String(), "\n"))
			cur.Reset()
			curLen = 0
		}
	}
	for line := range strings.SplitSeq(text, "\n") {
		for utf8.RuneCountInString(line) > limit {
			flush()
			r := []rune(line)
			pages = append(pages, string(r[:limit]))
			line = string(r[limit:])
		}
		n := utf8.RuneCountInString(line) + 1
		if curLen+n > limit+1 {
			flush()
		}
		cur.WriteString(line + "\n")
		curLen += n
	}
	flush()
	if len(pages) == 0 {
		return []string{""}
	}
	return pages
}
