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
| `internal/backup` | Daily and startup `VACUUM INTO` snapshots in `DATA_DIR/backups`, retention, upload to the Owner. Restore: `docs/runbooks/restore-backup.md`. |
| `internal/health` | `/healthz` from a list of named checks. |
| `internal/clock` | `Clock` interface, `System`, `Fake`, `Tehran()`. Never call `time.Now()` in the core; use `b.Clock.Now()`. |
| `internal/version` | `Version`, set by `-ldflags` at build time. |

Pure rule packages (Input rules, Jalali dates) go in their own `internal/<name>` package with no I/O and table tests. Outside clients (Metis extractor, Jev categorizer) get an interface the core depends on, the HTTP implementation in `internal/<name>`, and a fake for `bottest`.

## Adding a feature to the bot core

Register handlers from an `init` in a new file instead of editing a shared switch:

- Command: `cmd_<name>.go` calls `RegisterCommand(Command{Name, Help, Order, Run})`. `Order` sorts `/help`; `/help` itself is 1000, so use 10, 20, ... below it.
- Buttons: `cb_<name>.go` calls `RegisterCallback(Callback{Prefix, Run})`. `callback_data` is `<prefix>:<id>:<action>`, at most 64 bytes (`t:<id>:undo` for Transactions). The core always calls `answerCallbackQuery` with the toast `Run` returns.
- Text notes: the Input pipeline calls `SetTextHandler` once. Until then plain text gets the help text.

A new collaborator (Extractor, Categorizer, another storage interface) is one field in `bot.Deps`, wired in `internal/app/app.go` and in `bottest.Harness.Deps`.

## Migrations

`internal/storage/sqlite/migrations/NNNN_name.sql`, where `NNNN` is your ticket's issue number (`0032_transactions.sql`). Several files for one ticket share the prefix (`0032_transactions.sql`, `0032_transactions_index.sql`). The runner applies every file not yet in `schema_migrations`, in name order, each in its own transaction, so a lower-numbered file merged later still runs. Never edit a migration after it merged; add a new one.

Portable SQL only: `INTEGER` ids, `TEXT`, `BIGINT` amounts and UTC unix seconds, 0/1 booleans, `ON CONFLICT` upserts. No SQLite-only types or functions.

## Tests

- Behaviour goes through `bottest`: feed updates (`h.SendText`, `h.SendMessage`, `h.Tap`, `h.Feed`), then assert on `h.Sent()`, `h.Bale.Edits()`, `h.Bale.Answers()`, and rows read back through the storage interfaces. Move time with `h.Clock.Advance`. `h.Restart()` reopens the DB and rebuilds the bot. `h.Bale.Fail(method, err)` makes a Bale call fail.
- One test file per feature (`report_test.go`), package `bot_test`.
- HTTP edges (Bale polling, webhook, Metis, Jev, `/healthz`) use `httptest`.

## Checks

`go vet ./...`, `go test ./...` and `golangci-lint run` (v2, default linters) must pass; CI also builds the Docker image. A golangci-lint built with an older Go than `go.mod` fails to load; build it with the current toolchain (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`).
