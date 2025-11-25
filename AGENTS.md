# Repository Guidelines

## Project Structure & Module Organization
- Go module lives at the repository root; the main entry point is `cmd/bot/main.go` for Telegram polling/webhook setup and dependency wiring.
- Business logic resides in `internal/` by concern: `internal/config` for env loading, `internal/telegram` for handlers/commands, `internal/feature/<name>` for pluggable payment flows, and `internal/store` for MongoDB access; shared utilities go under `pkg/` when they are not domain-specific.
- Tests stay beside code as `_test.go`; larger fixtures belong in `testdata/`.
- Architecture notes live in root docs: `design-document.md` and `tech-stack.md`.

## Build, Test, and Development Commands
- `go fmt ./...` — enforce canonical formatting before commits.
- `go build ./cmd/bot` — compile the bot binary.
- `go run ./cmd/bot` — start locally; requires `TELEGRAM_TOKEN`, `BOT_OWNER`, `MONGO_URI`, and payment provider keys in the environment.
- `go test ./...` — run unit tests; append `-run <Name>` to focus.
- `docker-compose -f docker-compose.local.yml up -d` — bring up MongoDB and other local dependencies.

## Coding Style & Naming Conventions
- Target Go 1.25; order imports stdlib, third-party, then internal.
- Package names are short/lowercase; exported symbols use PascalCase, unexported use camelCase; errors follow `ErrXYZ`.
- Log via `logrus` with structured fields; pass contexts to propagate request/trace IDs.
- Keep functions small and cohesive; place interfaces near their consumers; avoid global state except configuration wiring.

## Testing Guidelines
- Prefer table-driven tests; isolate external calls through interfaces or fakes.
- Cover command handlers, feature toggles, and Mongo persistence paths; add integration tests under `internal/...` that can run against Docker services.
- Name tests by behavior, e.g., `TestHandleWithdraw_FailsOnInsufficientBalance`.

## Commit & Pull Request Guidelines
- Use imperative commit messages (e.g., “Add merchant payout handler”) and keep changes scoped.
- PRs should describe behavior changes, risks, and validation steps; link issues/TODOs; include logs or screenshots for Telegram flows when helpful.
- Ensure `go fmt` and `go test ./...` pass before requesting review; highlight config/migration changes and update docs (`design-document.md`, `tech-stack.md`) accordingly.

## Security & Configuration Tips
- Never commit secrets; `.env` is ignored—load config via environment variables or secret stores.
- Restrict MongoDB and payment API credentials to least privilege; rotate keys regularly and redact sensitive fields in logs.
- Rate-limit and add defensive error handling to new feature modules to protect upstream payment providers.

# IMPORTANT:
# Always read memory-bank/@architecture.md before writing any code. Include entire database schema.
# Always read memory-bank/@design-document.md before writing any code.
# After adding a major feature or completing a milestone, update memory-bank/@architecture.md.