# Architecture Notes

## File Map (current state)
- `design-document.md`: High-level product intent and scope for the Telegram payment bot.
- `tech-stack.md`: Stack choices (Go 1.25, go-telegram/bot, mongo-driver, logrus, Docker Compose).
- `implementation-plan.md`: Base-bot implementation steps and validation criteria.
- `progress.md`: Running log of completed steps and environment confirmations.
- `architecture.md`: This document; captures repository structure notes and database schema status.
- `AGENTS.md`: Repository automation/agent guidelines.
- `tmp.md`: Scratchpad file (no contract; safe to ignore for architecture).

## Runtime Configuration
- Config loader implemented (Implementation Plan Step 5): resolves APP_ENV (default production), loads .env only in development, validates required TELEGRAM_TOKEN/BOT_OWNER/MONGO_URI/MONGO_DB and parses BOT_OWNER/HTTP_PORT; defaults LOG_LEVEL and HTTP_PORT when unset.
- Configuration dry-run supported via `-config-only` flag: loads config, validates Mongo URI scheme/host, prints a redacted summary (hiding token/credentials), then exits without starting the bot.
- Structured logging initialized (Implementation Plan Step 7): global logrus logger with JSON format in production and text in development, default fields `service=telegram-bot` and `env`, key names `ts/level/msg`, and helpers for info/warn/error plus contextual `user_id/chat_id/event` fields.

## Database Schema
- No collections are defined or initialized yet. Planned base collections (per implementation plan Step 11) are `users` (unique `user_id`, `role`, timestamps) and `groups` (unique `chat_id`, title, joined timestamps), but they have not been created or indexed in this iteration.
