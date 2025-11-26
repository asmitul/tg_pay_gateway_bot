# Architecture Notes

## File Map (current state)
- `design-document.md`: High-level product intent and scope for the Telegram payment bot.
- `tech-stack.md`: Stack choices (Go 1.25, go-telegram/bot, mongo-driver, logrus, Docker Compose).
- `implementation-plan.md`: Base-bot implementation steps and validation criteria.
- `progress.md`: Running log of completed steps and environment confirmations.
- `architecture.md`: This document; captures repository structure notes and database schema status.
- `AGENTS.md`: Repository automation/agent guidelines.
- `error-handling-guidelines.md`: Error propagation and logging conventions (Implementation Plan Step 8).
- `tmp.md`: Scratchpad file (no contract; safe to ignore for architecture).

## Runtime Configuration
- Config loader implemented (Implementation Plan Step 5): resolves APP_ENV (default production), loads .env only in development, validates required TELEGRAM_TOKEN/BOT_OWNER/MONGO_URI/MONGO_DB and parses BOT_OWNER/HTTP_PORT; defaults LOG_LEVEL and HTTP_PORT when unset.
- Configuration dry-run supported via `-config-only` flag: loads config, validates Mongo URI scheme/host, prints a redacted summary (hiding token/credentials), then exits without starting the bot.
- Structured logging initialized (Implementation Plan Step 7): global logrus logger with JSON format in production and text in development, default fields `service=telegram-bot` and `env`, key names `ts/level/msg`, and helpers for info/warn/error plus contextual `user_id/chat_id/event` fields.

## MongoDB Client Management
- Mongo manager added (Implementation Plan Step 10): `internal/store.Manager` establishes a single Mongo client with URI/DB from config, pings the primary on startup, and exposes helpers for `users` and `groups` collections.
- Connection lifecycle: main uses a 10s connect timeout and 5s disconnect timeout; shutdown logs success or errors and cleans up the client.

## Local Development Stack
- `docker-compose.local.yml` provides MongoDB 6.0 for development (no auth, bound to 0.0.0.0:27017) with a persistent `mongo_data` volume.
- Default database `tg_bot_dev` is set via `MONGO_INITDB_DATABASE`; production deployments must enable credentials and use `tg_bot` (pattern `tg_bot_{APP_ENV}` is acceptable).

## Database Schema
- Base collections created for the bot skeleton:
  - `users`: fields `user_id` (unique), `role`, `created_at`, `updated_at`.
  - `groups`: fields `chat_id` (unique), `title`, `joined_at`.
- Unique indexes are ensured at startup via `store.Manager.EnsureBaseIndexes`: `users.user_id` (`user_id_unique`) and `groups.chat_id` (`chat_id_unique`).
