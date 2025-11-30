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

## Shutdown Flow
- Bot listens for `SIGINT`/`SIGTERM` and logs a `shutdown_signal` event when caught; Telegram polling runs on a cancelable background context with a 10s shutdown wait (`telegramShutdownTimeout`) to stop receiving new updates.
- After polling stops (or the wait times out), MongoDB closes with a 5s timeout (`mongoDisconnectTimeout`) and logs `mongo_disconnect`.
- Lifecycle ends with a `shutdown_complete` log once resources are closed to document orderly termination.

## User Registration
- `internal/feature/user.Registrar` upserts users on first contact with `role=user`, populating `created_at`/`updated_at`/`last_seen_at`, and refreshes `updated_at`/`last_seen_at` on every subsequent update.
- The Telegram default handler invokes the registrar for any update carrying a `user_id` before routing; failures log `event=user_registration_failed` with chat/user context while routing continues.

## Group Registration
- `internal/feature/group.Registrar` upserts groups when the bot sees activity in a group/supergroup chat, setting `joined_at`/`last_seen_at` plus the trimmed chat title on first sight and refreshing `last_seen_at` (and title when provided) on subsequent interactions.
- The Telegram default handler invokes the registrar for updates in group/supergroup chats; failures log `event=group_registration_failed` with chat context while routing continues.

## Diagnostics
- `store.Manager` exposes `Ping(ctx)` to verify Mongo connectivity and wraps failures for caller-friendly errors.
- `/ping` command replies with `pong`, `env`, `uptime` (derived from process start time), and `mongo: ok|error`; Mongo ping uses a 2s timeout and logs failures but still responds to the user.
- HTTP health endpoint `/healthz` served on `HTTP_PORT` (default 8080) returns `{"status":"ok"}` when Mongo ping succeeds and `{"status":"degraded","mongo":"error"}` when Mongo is unreachable or the checker is missing; Mongo health ping uses a 2s timeout and the server shuts down gracefully with the process.

## Permissions & Admin Commands
- Owner-only commands validate the Mongo-backed user role and require the `BOT_OWNER` id; unauthorized users receive a short “permission denied” reply with audit logs.
- `/status` (owner only) returns `bot_status: running`, `env`, `connected_chats`, and `registered_users` from live Mongo counts; count failures are logged and surface `error` placeholders while still responding.

## Local Development Stack
- `docker-compose.local.yml` provides MongoDB 6.0 for development (no auth, bound to 0.0.0.0:27017) with a persistent `mongo_data` volume.
- Default database `tg_bot_dev` is set via `MONGO_INITDB_DATABASE`; production deployments must enable credentials and use `tg_bot` (pattern `tg_bot_{APP_ENV}` is acceptable).

## Containerization
- Multi-stage Dockerfile builds the bot from `golang:1.25-alpine` with `CGO_ENABLED=0` and `-trimpath -ldflags "-s -w"` producing a static `bot` binary before copying into a `gcr.io/distroless/static-debian12` runtime.
- Runtime runs as `nonroot:nonroot`, exposes port 8080 for `/healthz`, and relies on the same env vars (`TELEGRAM_TOKEN`, `BOT_OWNER`, `MONGO_URI`, `MONGO_DB`, `HTTP_PORT`, `APP_ENV`, `LOG_LEVEL`) for configuration.
- `.dockerignore` trims docs/editor/test artifacts from the build context to keep rebuilds and image layers small.
- Local build/verify example: `docker build -t tg-pay-gateway-bot:local .` then `docker run --rm -e TELEGRAM_TOKEN=dummy -e BOT_OWNER=1 -e MONGO_URI=mongodb://localhost:27017 -e MONGO_DB=tg_bot_dev tg-pay-gateway-bot:local -config-only`.

## Database Schema
- Base collections created for the bot skeleton:
  - `users`: fields `user_id` (unique), `role`, `created_at`, `updated_at`, `last_seen_at` (updated for each user interaction).
  - `groups`: fields `chat_id` (unique), `title`, `joined_at`, `last_seen_at` (set to `joined_at` on insert and refreshed on each group interaction).
- Unique indexes are ensured at startup via `store.Manager.EnsureBaseIndexes`: `users.user_id` (`user_id_unique`) and `groups.chat_id` (`chat_id_unique`).
