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

## Domain Models
- Users are represented by `domain.User` with `user_id`, `role` (owner/admin/user), timestamps `created_at`/`updated_at`, and `last_seen_at` (touched on every update). Role priority helper maps owner=3, admin=2, user=1 for access decisions.
- Groups are represented by `domain.Group` with `chat_id`, `title`, `joined_at`, and `last_seen_at` (defaults to `joined_at` when not pre-populated).

## Owner Bootstrap
- Startup runs an owner registrar (`internal/feature/owner.Registrar`) after indexes are ensured: it upserts the configured `BOT_OWNER` into `users` with `role=owner`, sets `created_at` on first insert and `updated_at` on every run, and logs `event=owner_bootstrap` with demote/upsert counts.
- Any existing owners whose `user_id` differs from `BOT_OWNER` are demoted to `role=admin` to enforce a single owner record.

## Telegram Client Connectivity
- Telegram wired via `github.com/go-telegram/bot` (Implementation Plan Step 12) using long polling.
- Allowed updates subscribed by default: `message`, `edited_message`, `callback_query`, `my_chat_member`, `chat_member`.
- Default handler logs update type, user/chat IDs, and text payloads; errors from the poller are logged through the shared logger. User registration runs before routing to ensure user presence/last seen tracking.
- Process uses `signal.NotifyContext` to stop polling cleanly when receiving termination signals.

## User Registration
- `internal/feature/user.Registrar` upserts users on first contact with `role=user`, populating `created_at`/`updated_at`/`last_seen_at`, and refreshes `updated_at`/`last_seen_at` on every subsequent update.
- The Telegram default handler invokes the registrar for any update carrying a `user_id` before routing; failures log `event=user_registration_failed` with chat/user context while routing continues.

## Group Registration
- `internal/feature/group.Registrar` upserts groups when the bot sees activity in a group/supergroup chat, setting `joined_at`/`last_seen_at` plus the trimmed chat title on first sight and refreshing `last_seen_at` (and title when provided) on subsequent interactions.
- The Telegram default handler invokes the registrar for updates in group/supergroup chats; failures log `event=group_registration_failed` with chat context while routing continues.

## Local Development Stack
- `docker-compose.local.yml` provides MongoDB 6.0 for development (no auth, bound to 0.0.0.0:27017) with a persistent `mongo_data` volume.
- Default database `tg_bot_dev` is set via `MONGO_INITDB_DATABASE`; production deployments must enable credentials and use `tg_bot` (pattern `tg_bot_{APP_ENV}` is acceptable).

## Database Schema
- Base collections created for the bot skeleton:
  - `users`: fields `user_id` (unique), `role`, `created_at`, `updated_at`, `last_seen_at` (updated for each user interaction).
  - `groups`: fields `chat_id` (unique), `title`, `joined_at`, `last_seen_at` (set to `joined_at` on insert and refreshed on each group interaction).
- Unique indexes are ensured at startup via `store.Manager.EnsureBaseIndexes`: `users.user_id` (`user_id_unique`) and `groups.chat_id` (`chat_id_unique`).
