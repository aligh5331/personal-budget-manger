package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
)

// Transaction buttons share one callback prefix, "t", with the form
// t:<id>:<action>. Features add an action (undo, edit, category, ...) with
// RegisterTransactionAction instead of registering their own prefix.

// TransactionAction handles one action on Transaction id. It returns the
// toast for answerCallbackQuery.
type TransactionAction func(ctx context.Context, b *Bot, q *bale.CallbackQuery, id int64) (toast string, err error)

var transactionActions = map[string]TransactionAction{}

const staleButtonToast = "This button no longer works."

// RegisterTransactionAction adds a t:<id>:<action> handler. It panics on a
// duplicate action; call it from init.
func RegisterTransactionAction(action string, fn TransactionAction) {
	if _, dup := transactionActions[action]; dup {
		panic(fmt.Sprintf("bot: transaction action %q registered twice", action))
	}
	transactionActions[action] = fn
}

// An action may carry an argument: t:<id>:<action>:<arg> runs the handler
// registered for <action>, which reads <arg> with TransactionArg.

// TransactionData builds the callback_data for an action on Transaction id.
// action may be "<action>:<arg>".
func TransactionData(id int64, action string) string {
	return "t:" + strconv.FormatInt(id, 10) + ":" + action
}

func init() {
	RegisterCallback(Callback{Prefix: "t", Run: runTransactionCallback})
}

func runTransactionCallback(ctx context.Context, b *Bot, q *bale.CallbackQuery) (string, error) {
	parts := strings.SplitN(q.Data, ":", 3)
	if len(parts) != 3 {
		return staleButtonToast, nil
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return staleButtonToast, nil
	}
	name, _, _ := strings.Cut(parts[2], ":")
	fn, ok := transactionActions[name]
	if !ok {
		return staleButtonToast, nil
	}
	return fn(ctx, b, q, id)
}

// TransactionArg returns the <arg> of t:<id>:<action>:<arg> ("" when none).
func TransactionArg(q *bale.CallbackQuery) string {
	parts := strings.SplitN(q.Data, ":", 4)
	if len(parts) < 4 {
		return ""
	}
	return parts[3]
}
