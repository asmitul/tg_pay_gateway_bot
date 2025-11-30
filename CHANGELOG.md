# Changelog

## v1.0.0 - 2025-11-30
- Skeleton release: config loader with `.env` in development, structured logrus logger, Mongo manager with base indexes, and domain models for users/groups.
- Telegram long polling with graceful shutdown, user/group registrars, owner bootstrap, and commands `/start`, `/ping`, `/status` (owner-only).
- Diagnostics: `-config-only` dry-run, Mongo connectivity check, and contextual structured logging on each update.
- Packaging: multi-stage Dockerfile (distroless runtime, non-root) and local `docker-compose.local.yml` for Mongo + bot.
- CI/CD: `CI` runs fmt/tests/build; `Release` builds/pushes GHCR images tagged by commit SHA (+main/latest, version tags on `v*`); `Production Deploy` pulls the release image and deploys to the VPS with rollback safety.
