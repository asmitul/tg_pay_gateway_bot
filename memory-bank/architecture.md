# Architecture Notes

## File Map (current state)
- `design-document.md`: High-level product intent and scope for the Telegram payment bot.
- `tech-stack.md`: Stack choices (Go 1.25, go-telegram/bot, mongo-driver, logrus, Docker Compose).
- `implementation-plan.md`: Base-bot implementation steps and validation criteria.
- `progress.md`: Running log of completed steps and environment confirmations.
- `architecture.md`: This document; captures repository structure notes and database schema status.
- `AGENTS.md`: Repository automation/agent guidelines.
- `tmp.md`: Scratchpad file (no contract; safe to ignore for architecture).

## Database Schema
- No collections are defined or initialized yet. Planned base collections (per implementation plan Step 11) are `users` (unique `user_id`, `role`, timestamps) and `groups` (unique `chat_id`, title, joined timestamps), but they have not been created or indexed in this iteration.
