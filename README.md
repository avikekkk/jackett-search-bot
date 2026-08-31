# JackettSearchBot

Telegram bot for searching Jackett releases with in-chat pagination.

Written in Go on [gotd/td](https://github.com/gotd/td) (MTProto), with persistent authorization,
host stats, and log upload.

## Prerequisites

- Go 1.26+
- Telegram API credentials (`api_id` / `api_hash`) from https://my.telegram.org and a bot token
- Jackett running and reachable from this machine

## Setup

1. Clone the repository and enter it.
```bash
git clone <your-repo-url>
cd JackettSearchBot
```

2. Create `.env` from the template and fill in the values.
```bash
cp .env.example .env
```

```env
TELEGRAM_API_ID=123456
TELEGRAM_API_HASH=your_api_hash_here
TELEGRAM_BOT_TOKEN=your_bot_token_here
OWNER_ID=123456789
AUTHORIZED_CHAT_IDS=-1001234567890

JACKETT_URL=http://localhost:9117
JACKETT_API_KEY=your_jackett_api_key_here

MAX_RESULTS=10
SEARCH_REDACT_SECONDS=300
DATABASE_PATH=jackettbot.db
```

Notes:
- `TELEGRAM_API_ID`, `TELEGRAM_API_HASH`, `TELEGRAM_BOT_TOKEN`, `OWNER_ID`, and `JACKETT_API_KEY`
  are required; the rest have defaults.
- `AUTHORIZED_CHAT_IDS` is optional bootstrap data: a comma-separated list of group or chat IDs
  whose members may run `/r`. Admin commands stay owner-only.
- `MAX_RESULTS` is the page size of a result message, `SEARCH_REDACT_SECONDS` is how long results
  stay visible (`0` keeps them until `CLOSE` is tapped).
- `DATABASE_PATH` is the SQLite file holding IDs authorized at runtime with `/auth`. Those grants
  survive a restart; `AUTHORIZED_CHAT_IDS` entries stay in `.env`.
- Logs are written to `logs/jackett_bot.log`, size-rotated, and uploaded by `/logs`.

## Run

```bash
./start.sh
```

Or build and run manually:

```bash
go build -o bin/jackettbot ./cmd/jackettbot
./bin/jackettbot
```

Missing or invalid configuration stops startup with a `Configuration error:` line naming the value.
If Telegram returns a startup flood wait, the bot exits with a clear message and expected wait time.

## Bot commands

- `/r <query>` : Search releases, paginated with `PREV`/`NEXT`, hidden with `CLOSE`.
  The query is free text, an IMDb ID (`tt0133093`), or an IMDb link.
- Result flags, usable anywhere in the command:
  - `--ptp` : PassThePopcorn only.
  - `--btn` : BroadcasTheNet only.
  - `--gp` : Golden Popcorn releases only. Requires `--ptp`, since the label is PassThePopcorn's.

  With neither `--ptp` nor `--btn`, every configured indexer is searched. Given together they
  mean both.
- `/help`, `/start` : Command list.
- `/server` : Host stats and bot uptime.
- `/logs` : Owner-only. Uploads the current log file.
- `/auth [id]`, `/unauth [id]` : Owner-only. Target is the replied-to user, an explicit ID, or the
  current chat, in that order.
- `/unauthall` : Owner-only. Clears every runtime authorization; `.env` and owner access remain.

Results come back 1080p first, then 2160p, smallest first within each group.

Only the requester (or the owner) can page or close a result message, and only in the chat the
search was run in.

Authorization rules:
- Access is granted if any one applies: owner, ID in `AUTHORIZED_CHAT_IDS`, or ID authorized at
  runtime with `/auth`.
- Because of that, removing one grant may still leave another grant active.
- `/unauthall` clears only runtime grants; `.env` IDs and owner access remain.

Rate limit behavior:
- Telegram `FloodWait` during normal operation is handled gracefully (send/edit/delete/callback).
- Temporary rate-limit responses are returned to users instead of crashing handler execution.

## Project structure

- `cmd/jackettbot/main.go` : Entry point, logging, signal handling.
- `internal/config/config.go` : Environment-based configuration and validation.
- `internal/bot/bot.go` : Client setup, command routing, authorization, help text.
- `internal/bot/search.go` : `/r`, pagination sessions, redaction, callbacks.
- `internal/bot/admin.go` : `/logs`, `/auth`, `/unauth`, `/unauthall`.
- `internal/bot/stats.go` : `/server` host stats.
- `internal/jackett/jackett.go` : Torznab query, feed parsing, size formatting.
- `internal/jackett/indexer.go` : Indexer/filter registry and search options.
- `internal/jackett/ptp.go`, `btn.go` : One file per tracker, holding its ID, flags, and quirks.
- `internal/store/store.go` : SQLite-backed authorizations.

### Adding an indexer

Copy `internal/jackett/ptp.go`. Declare the indexer with its Jackett ID, its `/r` flag, a short
label for the results header, and the tracker's full name:

```go
var HDB = registerIndexer(&Indexer{
	ID:    "hdbits",
	Flag:  "--hdb",
	Label: "HDB",
	Name:  "HDBits",
})
```

That is the whole change. Flag parsing, the `/help` list, the results header, and endpoint scoping
all read the registry, so nothing else needs editing. Two optional extras live in the same file
when a tracker has its own quirks:

- `SplitTitle` on the indexer, if it appends something to release names the way PTP appends
  `[1080p / Blu-ray / ...]`. Only that tracker's releases go through it.
- `registerFilter`, for a flag that only makes sense for this tracker, like `--gp`. Set its
  `Indexer` field and the flag is rejected unless that indexer was asked for too.

## Best Practices

- Keep secrets only in a local `.env`; do not commit tokens or API keys.
- Run `go mod tidy` after adding or removing imports.
- Run the bot with a process manager in production (for example: `systemd`, Docker restart policy, or PM2).
- Rotate credentials immediately if they are ever exposed.
