# Code layout

Where things go, for agents implementing tickets of spec #30. Domain words come from `CONTEXT.md`.

## Packages

| Path | What lives there |
|---|---|
| `cmd/bot` | `main`: loads config, handles signals, calls `app.Run`. Keep it thin. |
| `internal/app` | Production wiring: config, storage, Bale client, bot core, worker, update source, HTTP mux. The only place that names concrete types. |
| `internal/config` | Env vars only. Add a variable here, to `.env.example` and to `LogValue` (redact secrets). |
| `internal/bale` | `Client` interface, wire types, plain `net/http` client. `balefake` is the recording fake for tests. |
| `internal/updates` | Update sources and the single in-order `Worker`. `poller.go` is long polling; the webhook handler and mode switch go here too. |
| `internal/bot` | The bot core. `HandleUpdate` does the Owner check, `update_id` dedupe and routing. Features are files in this package. |
| `internal/bot/bottest` | The test seam: `bottest.New(t)` gives a bot wired to the fake Bale client, a fixed Asia/Tehran clock (`bottest.Start`, 14 Mehr 1405 12:00) and a real temp SQLite DB. |
| `internal/storage` | One small interface per aggregate, one file each (`settings.go`, later `transactions.go`, `categories.go`, ...). Methods named by intent. |
| `internal/storage/sqlite` | The implementation, one file per aggregate, plus `migrations/`. |
| `internal/extract` | `Extractor` interface, the `Result`/`Item` the model returns, and `Metis` (OpenAI-compatible chat, strict json_schema, one retry on `gpt-5-mini`). `extractfake` is the scripted fake (`h.Extractor` in `bottest`). |
| `internal/categorize` | `Categorizer` interface (`Categorize(ctx, text, []Option) (Pick, error)`) and `Jev` (Metis TypeSafe Choice question, one retry on 5xx, timeout or unreadable reply). Option keys are slugs of the name (`c<id>` fallback). `categorizefake` is the scripted fake (`h.Categorizer.Choose(name, conf)`, `.Fail(err)`, `.Calls()`; unscripted it picks Uncategorized). |
| `internal/inputrules` | The Input rules, pure: `Apply(Input{Text, Extraction, Now}) Outcome` turns the model's reading into `Draft`s, each carrying the one `FollowUp` field it still needs. One file per rule family (`normalize.go`, `direction.go`, `date.go`, `reldate.go`, `note.go`, `noteamount.go`); add rules there, behind `Apply`. |
| `internal/liveeval` | Opt-in live evaluation (#47). `liveeval.go` parses the samples file and scores a run (unit-tested); `live_test.go` is behind `//go:build live` and sends the 18 cases through real Metis and Jev: `LLM_API_KEY=... go test -tags live -run TestLiveEvaluation -v -timeout 20m ./internal/liveeval/` (costs money; `LIVE_SAMPLES`, `LIVE_RUNS` optional; skips without the key or the samples file). |
| `internal/jalali` | Jalali/Gregorian conversion (`FromTime`, `Date.At`), `Format` ("14 Mehr 1405"), `MonthName`, `DaysInMonth`, `MonthRange(y, m, loc)` (half-open instant range of a Jalali month), `AddMonths`. |
| `internal/backup` | Daily and startup `VACUUM INTO` snapshots in `DATA_DIR/backups`, retention, upload to the Owner. Restore: `docs/runbooks/restore-backup.md`. |
| `internal/health` | `/healthz` from a list of named checks. |
| `internal/clock` | `Clock` interface, `System`, `Fake`, `Tehran()`. Never call `time.Now()` in the core; use `b.Clock.Now()`. |
| `internal/version` | `Version`, set by `-ldflags` at build time. |

Pure rule packages (Input rules, Jalali dates) go in their own `internal/<name>` package with no I/O and table tests. Outside clients (Metis extractor, Jev categorizer) get an interface the core depends on, the HTTP implementation in `internal/<name>`, and a fake for `bottest`.

## Adding a feature to the bot core

Register handlers from an `init` in a new file instead of editing a shared switch:

- Command: `cmd_<name>.go` calls `RegisterCommand(Command{Name, Help, Order, Run})`. `Order` sorts `/help`; `/help` itself is 1000, so use 10, 20, ... below it.
- Buttons: `cb_<name>.go` calls `RegisterCallback(Callback{Prefix, Run})`. `callback_data` is `<prefix>:<id>:<action>`, at most 64 bytes (`t:<id>:undo` for Transactions). The core always calls `answerCallbackQuery` with the toast `Run` returns.
- Text notes: `input.go` is the Input pipeline (extract, `inputrules.Apply`, save, confirm), one function per step. Extend a step there; don't add a second text handler.
- Transaction buttons all share prefix `t`. Add an action with `RegisterTransactionAction("edit", fn)` in `cb_<name>.go`, and a button on every confirmation with `RegisterConfirmationButton(ConfirmationButton{Order, Label, Action})` (Undo is 10). `TransactionData(id, action)` builds the data.
- `confirmation.go` renders the confirmation. `FormatToman` and `FormatDate` are the shared display helpers. A `ConfirmationButton` may set `Shown(tx)` to hide itself for some Transactions (the [Category] button is hidden on internal transfers). `confirmation_batch.go` sends one confirmation for all Transactions of an Input (numbered, with `Undo 1`, `Undo 2` rows when more than one) and redraws it after an Undo; send confirmations through `sendConfirmations`. After a Category or Edit tap on a batch message the message shows just that Transaction (the others stay saved). `duplicate.go` holds the duplicate check and the `d:<id>:save` [Save anyway] button (held rows live in `held_duplicates`); Save anyway runs the Category step.
- `cmd_export.go` is `/export` (UTF-8 CSV with BOM, one row per Transaction). Its `exportCategoryName` reads the Category name (archived ones too); a new stored column goes into `exportHeader` and the row.
- Transaction action arguments: `t:<id>:<action>:<arg>`; the handler reads `<arg>` with `TransactionArg(q)`. `TransactionData(id, "cat:7")` builds one.

### Categories (#35)

- Storage: `storage.Categories` (`Uncategorized`, `ActiveCategories(kind)`, `CategoryByID`), `storage.Category{ID, Name, Hint, Kind, Archived, BuiltIn}`, `storage.CategoryKindFor(direction)`. Kinds: `KindExpense`, `KindIncome`, `KindAny` (Uncategorized only). Table and seed: `0035_categories.sql`. #42 added the write side (`AddCategory`, `RenameCategory`, `SetCategoryArchived`, `ArchivedCategories`, plus `SaveCategoryPrompt` for its typed replies). `cmd_categories.go` is /categories (callback prefix `cm`). Reports (#41) can join `transactions.category_id` to `categories` and must keep archived ones.
- `storage.Transactions.SetOwnerCategory(id, categoryID)` is the Owner's hand pick and clears `categorize_pending`. #36 should add a conditional update that applies a background result only while the row is still Uncategorized and pending.
- Category step: `category.go`, `(*Bot).categorizeTransaction(ctx, &tx)` runs before save in the Input pipeline. It sets `CategoryID` and `CategorizePending` (Categorizer failed after its retry). `MinCategoryConfidence` is 0.7. #37 calls it again once a Follow-up settles amount or Direction (`categorizable` skips Flagged ones). `categoryOptions` builds the option list; `categoryText` names a Category.
- `cmd_report.go` is `/report` (#41): `Transactions.ReportSummary(ReportRange)` totals by Category (archived kept), internal transfers and flagged count; buttons use callback prefix `rep` (`rep:140507`); `splitMessage` cuts at 4096 characters.
- Picker: `cb_category.go`, actions `cat`, `cat:<catID>`, `cat:back`. The picker works on any `TransactionView`: `registerCategoryPicker(viewKey)` adds `cat@<key>` actions. `editView(ctx, q, tx, viewKey)` / `editMessage` redraw a message in place (#36: `editView(..., "")` for a confirmation).
- `/transactions` (#40): `cmd_transactions.go` (list, filters, `listStore` in memory keyed by chat+message, 200 kept) and `cb_list.go` (callback prefix `l`: `l:p:<page>`, `l:f:<m|l|a|x>`, `l:c[:<id>]`, `l:r:<txid>`, `l:b`; record view `TransactionView` key `r`, delete actions `del@r`/`del.yes@r`/`del.no@r`). `Transactions.ListTransactions(TransactionFilter{From,To,FlaggedOnly,CategoryID}, limit, offset)`. Month filters use `jalali.MonthContaining` + `jalali.MonthRange`/`AddMonths`.

- Background re-categorize (#36): `recategorize.go`, `storage.CategorizeRetries` (table `categorize_retries`, `0036_`). `sendConfirmation` queues a pending Transaction with its confirmation message; `(*Bot).RecategorizeDue` (tests call it after `h.Clock.Advance`) retries every 10 min, 6 times max, and `RunRecategorize` is the production loop. `editMessageAt` edits a remembered message.

### Follow-ups and Flagged transactions (#37)

- `followup.go` is the engine. A draft missing its amount, or with an ambiguous Direction, is not saved: `queueFollowUp` stores it in the persisted `followups` table (`0037_followups.sql`, `storage.FollowUps`) as the Flagged placeholder that is saved if the Owner never answers. `askNextFollowUp` asks one question at a time per chat (batch Inputs queue). The amount is asked as free text (reply to the question; `inputrules.TypedAmount`), the Direction with `f:<id>:<out|in|internal>` buttons (a typed word also works).
- Every way out is in `followup.go`: an answer (`completeLocked`: duplicate check, Category step, save, confirmation), no answer in `FollowUpTTL` (30 min; `ExpireFollowUps`, run at startup and every 30 s by `RunFollowUpTimer`), a new Input (`closeOpenFollowUps`, first step of `handleInput`; reason stays `amount_missing` / `direction_ambiguous`), or an unreadable reply (`followup_unparsed`). Extraction failing twice saves the Input Flagged with its raw text (`saveUnreadInput`, reason `amount_missing`).
- `(*Bot).saveDraft` (input.go) is duplicate check, Category step and save for one complete draft; use it instead of calling `SaveTransaction` directly. `categorizable` skips every Flagged transaction.
- **For #38 [Fix]:** `(*Bot).OpenFollowUp(ctx, chatID, transactionID) (ok bool, err error)` opens a Follow-up for an existing Flagged transaction. It asks for the amount when there is none, else the Direction. When answered it clears the flag, runs the Category step, updates the row and sends a confirmation; an unreadable answer or no answer leaves the Transaction as it was. `ok` is false when the Transaction is gone, not Flagged, or already has an open Follow-up. A missing field is derived from the row (`AmountToman == nil` is the amount). The `Edit` flow's amount and Direction edits also clear the flag on any Flagged transaction.
- Tests: `h.ExpireFollowUps()` runs the sweep (move `h.Clock` first); `h.Restart()` runs it too, like startup. `extractfake` keeps its last scripted reading; `h.Extractor.Reset()` clears the script before queueing a new one after an Input was sent.

A new collaborator (Extractor, Categorizer, another storage interface) is one field in `bot.Deps`, wired in `internal/app/app.go` and in `bottest.Harness.Deps`.

## Migrations

`internal/storage/sqlite/migrations/NNNN_name.sql`, where `NNNN` is your ticket's issue number (`0032_transactions.sql`). Several files for one ticket share the prefix (`0032_transactions.sql`, `0032_transactions_index.sql`). The runner applies every file not yet in `schema_migrations`, in name order, each in its own transaction, so a lower-numbered file merged later still runs. Never edit a migration after it merged; add a new one.

Portable SQL only: `INTEGER` ids, `TEXT`, `BIGINT` amounts and UTC unix seconds, 0/1 booleans, `ON CONFLICT` upserts. No SQLite-only types or functions.

## Tests

- Behaviour goes through `bottest`: feed updates (`h.SendText`, `h.SendMessage`, `h.Tap`, `h.Feed`), then assert on `h.Sent()`, `h.Bale.Edits()`, `h.Bale.Answers()`, and rows read back through the storage interfaces. Move time with `h.Clock.Advance`. `h.Restart()` reopens the DB and rebuilds the bot. `h.Bale.Fail(method, err)` makes a Bale call fail. `h.StartUpdates()` boots the real update source (`updates.Manager`: polling or webhook, `/mode`) against the fake Bale client and routes `Feed` through the in-order worker; tests that don't call it never poll.
- One test file per feature (`report_test.go`), package `bot_test`.
- HTTP edges (Bale polling, webhook, Metis, Jev, `/healthz`) use `httptest`.

## Checks

`go vet ./...`, `go test ./...` and `golangci-lint run` (v2, default linters) must pass; CI also builds the Docker image. A golangci-lint built with an older Go than `go.mod` fails to load; build it with the current toolchain (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`).

## Releases

Pushing a `v*` tag runs `.github/workflows/release.yml`: image `ghcr.io/aligh5331/personal-budget-manger:vX.Y.Z` and `:latest`, plus a GitHub Release with the gzipped `docker save` tarball. The tag is baked in with `-ldflags -X .../internal/version.Version` and shows in `/help` and `bot -version`. On the lab, `deploy.sh` (pull or `docker load`, then `docker compose up -d`) runs next to `compose.yml` copied from `compose.example.yml`.
