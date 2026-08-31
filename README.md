# JackettSearchBot

Telegram bot for searching Jackett releases with in-chat pagination.

Two implementations live side by side: the Python bot (Pyrogram) described below, and a Go port
(gotd/td MTProto) under `cmd/` and `internal/`. See [Go build](#go-build).

## Prerequisites

- Python 3.10+
- `uv` installed (`https://docs.astral.sh/uv/`)
- A Telegram bot token
- Jackett running and reachable from this machine

## Setup

1. Clone the repository and enter it.
```bash
git clone <your-repo-url>
cd JackettSearchBot
```

2. Create the local environment and install dependencies with `uv`.
```bash
uv sync
```

Notes:
- `uv sync` creates a local `.venv` automatically.
- You usually do not need to activate it manually when using `uv run`.
- `rich` is used for colored, cleaner terminal logging.
- `tgcrypto` is installed automatically on Python 3.10-3.12; on 3.13+ it is skipped unless you have C++ build tools.

3. Create `config.env` from the template and fill values.
```bash
cp config.env.example config.env
```

```env
# Telegram Bot Configuration
TELEGRAM_TOKEN=your_bot_token_here
API_ID=123456
API_HASH=your_api_hash_here

# Jackett Configuration
JACKETT_API_KEY=your_jackett_api_key_here
JACKETT_URL=http://localhost:9117

# Authorization
AUTHORIZED_CHAT_IDS=id1,id2,id3
OWNER_ID=123456789

# Bot Behavior
MAX_RESULTS=10
REDACT_AFTER_SECONDS=300

# Logging
LOG_FILE_PATH=logs/jackett_bot.log
CONSOLE_LOG_LEVEL=INFO
FILE_LOG_LEVEL=DEBUG
```

Notes:
- `AUTHORIZED_CHAT_IDS` is optional bootstrap data loaded from `config.env` on startup.
- `REDACT_AFTER_SECONDS` controls how many seconds release results stay visible before auto-redaction.
- `/auth` adds temporary in-memory authorization only. Those IDs are cleared when the bot restarts.
- `LOG_FILE_PATH` is where verbose rotating logs are written.
- `CONSOLE_LOG_LEVEL` and `FILE_LOG_LEVEL` accept `DEBUG`, `INFO`, `WARNING`, `ERROR`, or `CRITICAL`.

## Run

```bash
uv run python main.py
```

If required config values are missing, startup stops with an initialization error and asks you to fill `config.env` first.
If Telegram returns a startup flood wait, the bot exits with a clear message and expected wait time.

## Bot Commands

- `/r <query>` : Search releases (with inline `PREV`/`NEXT` pagination when results span multiple pages).
- `/r <query> --ptp` : Search PassThePopcorn only.
- `/r <query> --btn` : Search BroadcasTheNet only.
- `/r <query> --ptp --gp` : Golden Popcorn releases only. `--gp` requires `--ptp`.
- `/auth [id]` : Owner-only. Temporarily authorize current chat by default, or an explicit ID (clears on restart).
- `/unauth [id]` : Owner-only. Remove temporary authorization for current chat by default, or an explicit ID.
- `/unauthall` : Owner-only. Remove all temporary in-memory authorizations.

`/auth` and `/unauth` target resolution:
- If ID is provided: uses that ID.
- Else if command is a reply to a user message: uses that user ID.
- Else: uses current chat ID.

Authorization rules:
- Access is granted if any one applies: owner, configured authorized ID, or temporary authorized ID.
- Because of that, removing one grant may still leave another grant active.
- `/unauthall` clears only temporary in-memory grants; configured IDs and owner access still remain active.

Rate limit behavior:
- Telegram `FloodWait` during normal bot operations is handled gracefully (send/edit/delete/callback paths).
- Temporary rate-limit responses are returned to users instead of crashing handler execution.

## Project Structure

- `jackett_bot/app.py` : Bot wiring and command registration.
- `jackett_bot/config.py` : Environment-based configuration.
- `jackett_bot/handlers/commands.py` : Telegram command handlers.
- `jackett_bot/services/auth.py` : In-memory authorization storage and lookups.
- `jackett_bot/services/jackett.py` : Jackett query and parsing logic.
- `main.py` : Application entry point.

## Go build

The Go port mirrors the Python bot's commands and result UI, and adds persistent authorization,
a `/server` stats command, and `/logs`.

### Setup

Requires Go 1.26+ and a reachable Jackett instance.

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
- The Go build reads `.env`; the Python build reads `config.env`. They do not share a config file.
- `MAX_RESULTS` is the page size of a result message, `SEARCH_REDACT_SECONDS` is how long results
  stay visible (`0` keeps them until `CLOSE` is tapped).
- `DATABASE_PATH` is the SQLite file holding IDs authorized at runtime with `/auth`. Unlike the
  Python build, those grants survive a restart; `AUTHORIZED_CHAT_IDS` entries stay in `.env`.
- Logs are written to `logs/jackett_bot.log`, size-rotated, and uploaded by `/logs`.

### Run

```bash
./start.sh
```

Or build and run manually:

```bash
go build -o bin/jackettbot ./cmd/jackettbot
./bin/jackettbot
```

Missing or invalid configuration stops startup with a `Configuration error:` line naming the value.

### Bot commands

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

Only the requester (or the owner) can page or close a result message, and only in the chat the
search was run in.

### Project structure

- `cmd/jackettbot/main.go` : Entry point, logging, signal handling.
- `internal/config/config.go` : Environment-based configuration and validation.
- `internal/bot/bot.go` : Client setup, command routing, authorization, help text.
- `internal/bot/search.go` : `/r`, pagination sessions, redaction, callbacks.
- `internal/bot/admin.go` : `/logs`, `/auth`, `/unauth`, `/unauthall`.
- `internal/bot/stats.go` : `/server` host stats.
- `internal/jackett/jackett.go` : Torznab query, indexer scoping, feed parsing, size formatting.
- `internal/store/store.go` : SQLite-backed authorizations.

## Best Practices

- Keep secrets only in local `config.env` / `.env`; do not commit tokens or API keys.
- Run commands with `uv run ...` to avoid global dependency conflicts.
- Keep dependencies in `pyproject.toml` and run `uv lock` after dependency changes; for the Go
  build, run `go mod tidy` after adding imports.
- Run the bot with a process manager in production (for example: `systemd`, Docker restart policy, or PM2).
- Rotate credentials immediately if they are ever exposed.
