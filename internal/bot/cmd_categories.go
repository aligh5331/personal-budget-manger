package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// /categories (#42): list, add, rename, archive and unarchive Categories.
// Nothing hard-deletes a Category, and the built-in Uncategorized can't be
// changed. Callback data (prefix cm):
//
//	cm:list            the active list
//	cm:archived        the archived list
//	cm:add             ask for a new Category (a reply to the prompt adds it)
//	cm:<id>:ren        ask for the new name (a reply renames)
//	cm:<id>:arc        archive
//	cm:<id>:unarc      unarchive

const (
	categoriesPrefix     = "cm"
	maxCategoryNameRunes = 40
	categoryExample      = "Gym | باشگاه | expense"
	categoryFormatHelp   = "Expected one line: name | Persian hint | expense or income.\nFor example: " + categoryExample + "\nThe hint is optional (Gym | expense)."
	categoryGoneToast    = "That Category no longer exists."
)

func init() {
	RegisterCommand(Command{Name: "categories", Help: "list, add, rename and archive Categories", Order: 600, Run: runCategories})
	RegisterCallback(Callback{Prefix: categoriesPrefix, Run: runCategoriesCallback})
	RegisterReplyHandler(handleCategoryReply)
}

func runCategories(ctx context.Context, b *Bot, m *bale.Message, _ string) error {
	text, markup, err := b.categoriesView(ctx, false)
	if err != nil {
		return err
	}
	return b.Reply(ctx, m, text, markup)
}

func (b *Bot) categoriesView(ctx context.Context, archived bool) (string, *bale.InlineKeyboardMarkup, error) {
	if archived {
		return b.archivedView(ctx)
	}
	expense, err := b.Categories.ActiveCategories(ctx, storage.KindExpense)
	if err != nil {
		return "", nil, err
	}
	income, err := b.Categories.ActiveCategories(ctx, storage.KindIncome)
	if err != nil {
		return "", nil, err
	}
	text := "Categories\n\nExpense: " + categoryNames(expense) + "\nIncome: " + categoryNames(income) +
		"\n\nUncategorized is built in and can't be renamed or archived."
	var rows [][]bale.InlineKeyboardButton
	for _, c := range append(expense, income...) {
		id := strconv.FormatInt(c.ID, 10)
		rows = append(rows, []bale.InlineKeyboardButton{
			{Text: "Rename " + c.Name, CallbackData: BuildCallbackData(categoriesPrefix, id, "ren")},
			{Text: "Archive " + c.Name, CallbackData: BuildCallbackData(categoriesPrefix, id, "arc")},
		})
	}
	rows = append(rows, []bale.InlineKeyboardButton{
		{Text: "Add", CallbackData: BuildCallbackData(categoriesPrefix, "add")},
		{Text: "Show archived", CallbackData: BuildCallbackData(categoriesPrefix, "archived")},
	})
	return text, &bale.InlineKeyboardMarkup{InlineKeyboard: rows}, nil
}

func (b *Bot) archivedView(ctx context.Context) (string, *bale.InlineKeyboardMarkup, error) {
	cs, err := b.Categories.ArchivedCategories(ctx)
	if err != nil {
		return "", nil, err
	}
	text := "Archived Categories\n\nThey stay on old Transactions but are not offered for new ones."
	if len(cs) == 0 {
		text = "No archived Categories."
	}
	var rows [][]bale.InlineKeyboardButton
	for _, c := range cs {
		rows = append(rows, []bale.InlineKeyboardButton{
			{Text: "Unarchive " + c.Name + " (" + c.Kind + ")", CallbackData: BuildCallbackData(categoriesPrefix, c.ID, "unarc")},
		})
	}
	rows = append(rows, []bale.InlineKeyboardButton{{Text: "Back", CallbackData: BuildCallbackData(categoriesPrefix, "list")}})
	return text, &bale.InlineKeyboardMarkup{InlineKeyboard: rows}, nil
}

func categoryNames(cs []storage.Category) string {
	if len(cs) == 0 {
		return "none"
	}
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return strings.Join(out, ", ")
}

func runCategoriesCallback(ctx context.Context, b *Bot, q *bale.CallbackQuery) (string, error) {
	data := ParseCallbackData(q.Data)
	switch {
	case data.Len() == 2 && data.Part(1) == "list":
		return "", b.redrawCategories(ctx, q.Message, false)
	case data.Len() == 2 && data.Part(1) == "archived":
		return "", b.redrawCategories(ctx, q.Message, true)
	case data.Len() == 2 && data.Part(1) == "add":
		return "", b.askCategory(ctx, q.Message, storage.CategoryPromptAdd, 0,
			"Send the new Category as one line: name | Persian hint | expense or income.\nFor example: "+categoryExample+"\nThe hint is optional. Reply to this message.")
	case data.Len() == 3:
		id, ok := data.Int(1)
		if !ok {
			return staleButtonToast, nil
		}
		return b.categoryAction(ctx, q, id, data.Part(2))
	}
	return staleButtonToast, nil
}

func (b *Bot) categoryAction(ctx context.Context, q *bale.CallbackQuery, id int64, action string) (string, error) {
	c, ok, err := b.Categories.CategoryByID(ctx, id)
	if err != nil {
		return "Something went wrong, try again.", err
	}
	if !ok {
		return categoryGoneToast, nil
	}
	if c.BuiltIn {
		return "Uncategorized can't be changed.", nil
	}
	switch action {
	case "ren":
		return "", b.askCategory(ctx, q.Message, storage.CategoryPromptRename, id,
			"New name for "+c.Name+"? Reply to this message.")
	case "arc", "unarc":
		archive := action == "arc"
		if _, err := b.Categories.SetCategoryArchived(ctx, id, archive); err != nil {
			return "Couldn't save that, try again.", err
		}
		if err := b.redrawCategories(ctx, q.Message, !archive); err != nil {
			b.Log.Warn("categories: redraw", "err", err)
		}
		if archive {
			return "Archived " + c.Name, nil
		}
		return "Unarchived " + c.Name, nil
	}
	return staleButtonToast, nil
}

// redrawCategories edits the tapped message to the active or archived list.
func (b *Bot) redrawCategories(ctx context.Context, host *bale.Message, archived bool) error {
	if host == nil {
		return nil
	}
	return b.redrawCategoriesAt(ctx, host.Chat.ID, host.MessageID, archived)
}

func (b *Bot) redrawCategoriesAt(ctx context.Context, chatID, messageID int64, archived bool) error {
	text, markup, err := b.categoriesView(ctx, archived)
	if err != nil {
		return err
	}
	return b.Bale.EditMessageText(ctx, bale.EditMessageTextParams{ChatID: chatID, MessageID: messageID, Text: text, ReplyMarkup: markup})
}

func (b *Bot) askCategory(ctx context.Context, host *bale.Message, action string, id int64, ask string) error {
	if host == nil {
		return nil
	}
	prompt, err := b.Send(ctx, host.Chat.ID, ask, nil)
	if err != nil {
		return err
	}
	return b.Categories.SaveCategoryPrompt(ctx, storage.CategoryPrompt{
		ChatID: host.Chat.ID, PromptMessageID: prompt.MessageID, Action: action,
		CategoryID: id, HostMessageID: host.MessageID, CreatedAt: b.Clock.Now(),
	})
}

// handleCategoryReply takes the Owner's reply to an add or rename prompt. A
// line that doesn't fit gets an error; the prompt stays open.
func handleCategoryReply(ctx context.Context, b *Bot, m *bale.Message) (bool, error) {
	p, ok, err := b.Categories.CategoryPromptByMessage(ctx, m.Chat.ID, m.ReplyToMessage.MessageID)
	if err != nil || !ok {
		return false, err
	}
	var done string
	switch p.Action {
	case storage.CategoryPromptAdd:
		done, err = b.addCategoryFromLine(ctx, m.Text)
	case storage.CategoryPromptRename:
		done, err = b.renameCategoryTo(ctx, p.CategoryID, m.Text)
	}
	var bad badCategoryInput
	if errors.As(err, &bad) {
		return true, b.Reply(ctx, m, string(bad), nil)
	}
	if err != nil {
		return true, err
	}
	if err := b.redrawCategoriesAt(ctx, p.ChatID, p.HostMessageID, false); err != nil {
		b.Log.Warn("categories: redraw", "err", err)
	}
	return true, b.Reply(ctx, m, done, nil)
}

// badCategoryInput is a message for the Owner about a reply that can't be used.
type badCategoryInput string

func (e badCategoryInput) Error() string { return string(e) }

func (b *Bot) addCategoryFromLine(ctx context.Context, line string) (string, error) {
	name, hint, kind, ok := parseCategoryLine(line)
	if !ok {
		return "", badCategoryInput("That isn't a Category line.\n" + categoryFormatHelp)
	}
	if utf8.RuneCountInString(name) > maxCategoryNameRunes {
		return "", badCategoryInput(fmt.Sprintf("That name is too long (at most %d characters).\n%s", maxCategoryNameRunes, categoryFormatHelp))
	}
	c, err := b.Categories.AddCategory(ctx, name, hint, kind)
	if errors.Is(err, storage.ErrCategoryNameTaken) {
		return "", badCategoryInput("There is already a Category called " + name + ". Reply with another name.")
	}
	if err != nil {
		return "", err
	}
	return "Added " + c.Name + " (" + c.Kind + ").", nil
}

func (b *Bot) renameCategoryTo(ctx context.Context, id int64, text string) (string, error) {
	name := strings.TrimSpace(text)
	if name == "" || strings.ContainsAny(name, "|\n") || utf8.RuneCountInString(name) > maxCategoryNameRunes {
		return "", badCategoryInput(fmt.Sprintf("Reply with a new name of at most %d characters, on one line.", maxCategoryNameRunes))
	}
	ok, err := b.Categories.RenameCategory(ctx, id, name)
	if errors.Is(err, storage.ErrCategoryNameTaken) {
		return "", badCategoryInput("There is already a Category called " + name + ". Reply with another name.")
	}
	if err != nil {
		return "", err
	}
	if !ok {
		return "", badCategoryInput("That Category can't be renamed.")
	}
	return "Renamed to " + name + ".", nil
}

// parseCategoryLine reads "name | Persian hint | kind". The hint may be
// left out ("name | kind" or "name | | kind") and then defaults to the name.
func parseCategoryLine(line string) (name, hint, kind string, ok bool) {
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	switch len(parts) {
	case 2:
		parts = []string{parts[0], "", parts[1]}
	case 3:
	default:
		return "", "", "", false
	}
	name, hint, kind = parts[0], parts[1], strings.ToLower(parts[2])
	if name == "" || strings.ContainsRune(name, '\n') || (kind != storage.KindExpense && kind != storage.KindIncome) {
		return "", "", "", false
	}
	if hint == "" {
		hint = name
	}
	return name, hint, kind, true
}
