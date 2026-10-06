# Restore a database backup

The bot takes a snapshot of its SQLite database at startup and every day at 03:00 Tehran time. Snapshots are kept in `DATA_DIR/backups` (`/data/backups` in the container) and each one is also sent to the Owner's Bale chat as a file.

Snapshots are named `bot-YYYYMMDD-HHMMSS.db`, in Tehran time. The folder keeps the newest copy of each of the last 14 days and of each of the last 8 weeks (Saturday to Friday). Older ones are deleted automatically.

A snapshot holds everything, including the `raw_text` of every Input.

Restoring replaces the whole database. Anything recorded after the snapshot was taken is lost.

## 1. Pick the snapshot

Take it from `DATA_DIR/backups` on the host, or download the file from the bot's message in your Bale chat. Use a file named `bot-*.db`, never one ending in `.tmp` (a snapshot that was still being written).

## 2. Stop the container

```sh
docker compose stop bot        # or: docker stop <container>
```

The bot must not be running while you swap the file.

## 3. Replace the database file

In the host folder mounted at `/data` (called `DATA_DIR` below; `./data` next to `compose.yml` with `compose.example.yml`):

```sh
cd DATA_DIR
mkdir -p broken && mv bot.db bot.db-wal bot.db-shm broken/ 2>/dev/null
cp backups/bot-20261006-030000.db bot.db
chown 10001:10001 bot.db       # the container runs as uid 10001
```

Move the `-wal` and `-shm` files away too. They belong to the old database, and SQLite would otherwise try to apply them to the restored one.

Keep `broken/` until you have checked the restore, then delete it.

## 4. Start the container

```sh
docker compose start bot       # or: docker start <container>
```

On startup the bot applies any migrations the snapshot is missing, so a snapshot from an older version works. It then takes a fresh snapshot and sends it to your chat.

## 5. Check

- `docker compose ps` shows the bot as `healthy` after about 30 seconds (the healthcheck calls `/healthz`).
- `/help` answers in Bale.
- `/transactions` shows the Transactions you expect up to the snapshot's time.

## Outside Docker

With `go run`, stop the process with Ctrl+C, do step 3 in your `DATA_DIR` (default `./data`, no `chown` needed), and start it again.
