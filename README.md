# Personal Budget Manager

A private [Bale](https://bale.ai) bot that records your spending. Forward it a bank message, add a few words of your own if you like, and it saves a Transaction with the amount in toman, the Direction (money out or in), a Category and a Jalali date. Cash payments work too: send only your note, like «نون ۲۵۰ تومن».

It is built for one person. Only the Owner (you) can use it, and everything runs in one Go container with a SQLite file.

> **Status:** v1 is feature-complete and passes its tests and CI, but it has not yet run against live Bale, Metis or Jev, and the 18-case live evaluation (`internal/liveeval`) has not been run. Expect to fix things on first use.

## What you get

| Command | What it does |
|---|---|
| (forward a bank message) | Saves one Transaction per bank message, up to 5 per Input. Your note after it becomes the description. |
| (send only a note) | Saves a cash Transaction. A bare amount under 10,000 is read as thousands of toman: «۲۵۰ تومن» is 250,000. |
| `/transactions` | This month's Transactions, 10 per page, with filters, edit, Category and delete. |
| `/report` | Spending and income by Category for this or last month, with totals and net. |
| `/flagged` | Transactions the bot couldn't complete, with [Fix] and [Delete]. |
| `/categories` | Add, rename and archive Categories. |
| `/export` | A CSV of every Transaction. |
| `/mode` | Switch between polling and webhook without redeploying. |
| `/help` | The commands, the Input convention and the version. |

Bot text is English, amounts are whole toman, and dates are Jalali. The bot asks at most one follow-up question per Transaction. If you don't answer, the Transaction is saved as flagged and kept out of totals until you fix it. A database snapshot is taken at startup and daily at 03:00 Tehran time, and each one is sent to your Bale chat.

Domain words (Owner, Input, Transaction, Flagged transaction, Follow-up) are defined in [`CONTEXT.md`](CONTEXT.md).

## What you need

- A Bale account and a bot of your own. Create one with Bale's BotFather and keep the token.
- A [Metis](https://metisai.ir) account and API key. The bot calls an extraction model (`gpt-4.1-mini`) and Jev, Metis's classifier, for Categories. The research estimated about $0.08 a month at 5 Inputs a day.
- Go 1.27 to run it from source, or Docker to run the published image.
- For webhook mode only: a public HTTPS URL on port 443 or 88 that reaches the bot. Polling needs no public address and is the default.

The bank-message handling was researched against Melli Bank messages (see `docs/research/bank-sms-formats.md`). Other Iranian banks follow similar conventions but are untested.

## Run it on your computer

```sh
git clone https://github.com/aligh5331/personal-budget-manger
cd personal-budget-manger
cp .env.example .env
```

Fill in `.env`:

| Variable | Meaning |
|---|---|
| `BALE_BOT_TOKEN` | Your bot's token. Required. |
| `OWNER_BALE_ID` | Your numeric Bale user id. Required. |
| `LLM_API_KEY` | Your Metis key. |
| `MODE_DEFAULT` | `polling` or `webhook`. Empty means polling. Used only until you switch with `/mode`. |
| `WEBHOOK_URL`, `WEBHOOK_SECRET_PATH` | Webhook mode only. The bot listens at `WEBHOOK_URL/bale/<secret>`; the secret must be 32+ characters. |
| `LISTEN_ADDR`, `DATA_DIR`, `TZ_REPORTS` | Defaults are `:8080`, `./data` and `Asia/Tehran`. |

**Finding your Bale id.** Set `OWNER_BALE_ID=1`, start the bot, send it any message from your account, and read the log line `dropped update from non-owner or non-private chat`: it prints your id as `sender`. Put that in `.env` and restart.

Then:

```sh
go run ./cmd/bot
```

Send `/start` to your bot. `http://localhost:8080/healthz` returns 200 when the process, the database and the update source are healthy, and 503 with JSON naming the failing part otherwise.

## Deploy with Docker

Tagged releases publish an image to `ghcr.io/aligh5331/personal-budget-manger` and attach a `docker save` tarball to the GitHub Release. If you fork the repo, change the image name in `deploy.sh` and `compose.example.yml`, then tag a release (`git tag v0.1.0 && git push --tags`) to build your own.

On the host:

```sh
mkdir bot && cd bot
cp /path/to/repo/deploy.sh /path/to/repo/compose.example.yml /path/to/repo/.env.example .
mv compose.example.yml compose.yml
mv .env.example .env                  # fill it in as above
mkdir -p data && sudo chown 10001:10001 data
docker network create reverse-proxy   # once, if you don't already have it
./deploy.sh                           # pulls :latest and runs docker compose up -d
```

`./deploy.sh v0.1.0` pins a version. `./deploy.sh bot.tar.gz` loads a Release tarball with no registry. A private ghcr.io package needs `docker login ghcr.io` first.

The example compose file publishes no host port and joins an external `reverse-proxy` network, so a reverse proxy in front of it handles TLS. For polling-only use you don't need the proxy.

To restore a backup, see [`docs/runbooks/restore-backup.md`](docs/runbooks/restore-backup.md).

## Make it yours

- **Categories** are yours to shape with `/categories`. Each has an English name and a Persian hint that Jev uses to match your notes. The seeded list lives in `internal/storage/sqlite/migrations/0035_categories.sql`.
- **Another bank or language:** the extraction prompt is in `internal/extract`, and the rules that turn its reading into a Transaction are pure code in `internal/inputrules` with table tests. Start there.
- **Check changes against real messages:** put forwarded messages in `samples/forwarded-messages.txt` (gitignored) and run the opt-in evaluation, which costs a little money:
  `LLM_API_KEY=... go test -tags live -run TestLiveEvaluation -v -timeout 20m ./internal/liveeval/`

## Develop

```sh
scripts/check.sh      # gofmt, go vet, go test, golangci-lint (what CI runs)
```

Where code goes: [`docs/agents/code-layout.md`](docs/agents/code-layout.md). Review rules: [`CODING_STANDARDS.md`](CODING_STANDARDS.md). The spec and tickets are GitHub issues #30–#47, and the research behind the design is in `docs/research/`.

## Privacy

The database and its backups hold the full text of every Input you send, including the bank message. Strip personal details such as card and account numbers before you forward. Messages and button taps from any account other than the Owner, and from groups, are dropped silently.
