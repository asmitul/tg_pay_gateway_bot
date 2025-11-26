# Telegram Payment Gateway Bot (Base)

## Scope
This iteration delivers the base bot only — no payment channels, billing flows, or scheduled jobs yet. The focus is on the core skeleton needed to support later payment features.

## Responsibilities
- Load and validate configuration from environment variables (Telegram token, bot owner, Mongo URI/DB, env toggles).
- Initialize structured logging suitable for development and production.
- Connect to MongoDB and manage client lifecycle.
- Run a Telegram client with basic command routing and registration hooks.
- Provide minimal commands for health and onboarding; payments will be added in future phases.

## Configuration
Environment keys and defaults are defined centrally in `internal/config/config.go` (`Contract` is the authoritative list).

## Local Development
- Start MongoDB with `docker compose -f docker-compose.local.yml up -d` (Mongo 6.0, dev no-auth, persistent volume). Default DB is `tg_bot_dev`; production deployments should enable auth and use `tg_bot`.

## Reference Docs
- Design intent and scenarios: `memory-bank/design-document.md`
- Technology choices: `memory-bank/tech-stack.md`
- Implementation steps and validation: `memory-bank/implementation-plan.md`
