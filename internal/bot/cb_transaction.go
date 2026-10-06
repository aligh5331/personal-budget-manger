package bot

import (
	"context"
	"fmt"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
)

// Transaction buttons share one callback prefix, "t", with the form
// t:<id>:<action>. Features add an action (undo, edit, category, ...) with
// RegisterTransactionAction instead of registering their own prefix.

// TransactionAction handles one action on Transaction id. It returns the
// toast for answerCallbackQuery.
type TransactionAction func(ctx context.Context, b *Bot, q *bale.CallbackQuery, id int64) (toast string, err error)

var transactionActions = map[string]TransactionAction{}

const transactionPrefix = "t"

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
	return BuildCallbackData(transactionPrefix, id, action)
}

func init() {
	RegisterCallback(Callback{Prefix: transactionPrefix, Run: runTransactionCallback})
}

func runTransactionCallback(ctx context.Context, b *Bot, q *bale.CallbackQuery) (string, error) {
	data := ParseCallbackData(q.Data)
	id, ok := data.Int(1)
	if data.Len() < 3 || !ok || id <= 0 {
		return staleButtonToast, nil
	}
	fn, ok := transactionActions[data.Part(2)]
	if !ok {
		return staleButtonToast, nil
	}
	return fn(ctx, b, q, id)
}

// TransactionArg returns the <arg> of t:<id>:<action>:<arg> ("" when none).
func TransactionArg(q *bale.CallbackQuery) string {
	return ParseCallbackData(q.Data).Rest(3)
}
