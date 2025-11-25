# Telegram Payment Bot – Base Implementation Plan

**Scope**

This Implementation Plan describes how to build the *base bot* only:

* Project skeleton and tooling
* Configuration, logging, and MongoDB wiring
* Telegram bot client with basic commands and user registration
* No payment channels, billing logic, or scheduled jobs yet (those come later)

It assumes the intent and tech stack already agreed in the design and tech documents.

---

## Phase 0 – Project Setup & Foundations

### Step 1 – Confirm runtime and tooling

Set up the development environment with:

* Go 1.25 installed
* Docker and Docker Compose installed for local MongoDB and bot container
* Git repository for version control

**Test**

* Run `go version` and verify it reports Go 1.25.x on each developer machine.
* Run `docker --version` and `docker compose version` and verify both succeed.
* Initialize a new Git repository and verify a first commit can be created.

---

### Step 2 – Create initial repository layout

Create a clear, modular directory layout aligned with the tech stack and “internal” packages, for example:

* `cmd/bot` for the main entry point
* `internal/config` for configuration loading
* `internal/logging` for logging setup
* `internal/store` for MongoDB access
* `internal/telegram` for the Telegram bot client, routing, and handlers
* `internal/domain` for core domain models (User, Group, etc.)

**Test**

* From the repository root, list directories and confirm that all of the above folders exist.
* Ensure the project builds (even if main just exits) with a simple `go build` in the repository root.

---

### Step 3 – Document base intent and scope in README

Create or update `README.md` (or an equivalent doc) to:

* Explicitly state that this iteration implements only the base bot (no payments yet).
* Summarize key responsibilities: config loading, logging, Mongo connection, Telegram client, and basic commands.
* Reference the higher-level design document and tech stack document.

**Test**

* Open `README.md` and verify:

  * The scope section explicitly says “base bot only / no payments yet”.
  * There are links or references to the design and tech stack docs.

---

## Phase 1 – Configuration System

### Step 4 – Define configuration contract

Define and document the canonical environment variables (authoritative list lives in `internal/config` docs/comments):

| Key             | Example                        | Required | Notes                                                |
| --------------- | ------------------------------ | -------- | ---------------------------------------------------- |
| TELEGRAM_TOKEN  | 123:ABC                        | Yes      | Telegram Bot Token                                   |
| BOT_OWNER       | 123456789                      | Yes      | Super admin Telegram user_id                         |
| MONGO_URI       | mongodb://localhost:27017      | Yes      | Mongo connection string                              |
| MONGO_DB        | tg_bot / tg_bot_dev            | Yes      | DB name; prod=`tg_bot`, dev=`tg_bot_dev`             |
| APP_ENV         | development / production       | No       | Default `production`; controls logging format, .env  |
| LOG_LEVEL       | info                           | No       | Overrides default log level                          |
| HTTP_PORT       | 8080                           | No       | HTTP health endpoint port; default 8080              |

`.env` loading is allowed **only** when `APP_ENV=development` (use dotenv); production must rely on environment variables.

**Test**

* Verify there is a single authoritative list of config keys, and that all developers agree on the exact key names.
* Ask another developer to read the config contract and tell you which values must be set before the bot can run; verify their list matches.

---

### Step 5 – Implement configuration loading logic

Implement a config module that:

* Reads configuration from environment variables; loads `.env` only when `APP_ENV=development`.
* Validates that all required keys are present.
* Fails fast with a clear, human-readable error if any required config is missing or malformed.

**Test**

* Set all required environment variables with dummy values and start the bot in development.

  * Expected: bot starts and logs a message indicating configuration load success.
* Unset one required variable and start the bot.

  * Expected: bot terminates quickly and prints a clear message naming the missing variable.

---

### Step 6 – Add configuration “print” or “dry-run” mode

Provide a simple way to verify configuration without starting the full Telegram bot (for example, a special mode or flag that only loads and prints config) and mask secrets in the output.

**Test**

* Run the bot in “config only” mode.

  * Expected: bot prints the current configuration summary (masking secrets) and exits successfully.
* Introduce an invalid value (e.g., malformed Mongo URI) and run again.

  * Expected: the mode reports the validation error and exits with a non-zero status.

---

## Phase 2 – Logging & Error Handling

### Step 7 – Initialize structured logging

Set up a logging module that:

* Creates a global logger using logrus with:

  * Format: JSON in production, text in development
  * Default log level: info (overridable by `LOG_LEVEL`)
  * Standard fields: timestamp (`ts`), `level`, `service=telegram-bot`, `env`, message (`msg`); include `user_id`, `chat_id`, `event` when available
* Exposes functions to log at info, warning, and error levels.

**Test**

* Start the bot in development with a simple “hello” log in main.

  * Expected: console output shows JSON logs (or consistently structured logs) with the environment and service name fields.
* Trigger an error intentionally (e.g., missing config) and confirm the error is logged with level “error”.

---

### Step 8 – Define error-handling guidelines

Create a short internal document (can be markdown under `docs/` or comments) describing:

* Which errors should be returned upward vs. handled locally.
* When to log at error vs. warning vs. info.
* A standard format for logging context (e.g., user_id, chat_id, update_type).

**Test**

* Ask another developer to read the guidelines and classify three example errors (e.g., “network timeout”, “user sent invalid command”, “Mongo unavailable”) into log levels and handling strategies.

  * Expected: their answers are consistent with the guidelines.

---

## Phase 3 – MongoDB Integration

### Step 9 – Set up local MongoDB via Docker Compose

Create a Docker Compose service for MongoDB suitable for local development:

* MongoDB version >= 6.0
* No-auth by default for development; document that production must use credentials
* Persistent volume to keep local data
* Database naming: `tg_bot_dev` for development, `tg_bot` for production (pattern `tg_bot_{APP_ENV}` is acceptable)

**Test**

* Run Docker Compose and ensure the MongoDB container starts successfully.
* Use a Mongo client or CLI to connect with the configured URI and create a test database.

  * Expected: connection succeeds, and collections can be created.

---

### Step 10 – Implement MongoDB client manager

Create a store module that:

* Initializes a single Mongo client instance during startup.
* Uses the URI and database name from configuration.
* Provides a clean shutdown function to close the Mongo client.
* Exposes helper functions to obtain handles for specific collections (e.g., users, groups).

**Test**

* Start the bot in development.

  * Expected: logs show a successful connection to MongoDB.
* Stop the bot.

  * Expected: logs show that MongoDB client cleanly disconnected with no resource leaks or panics.

---

### Step 11 – Define base collections and indices

Define base collections required by the core bot (no payments yet):

* `users`: `user_id` (Telegram ID, unique), `role` (owner/admin/user), `created_at`, `updated_at`
* `groups`: `chat_id` (unique), `title`, `joined_at`

Indices:

* `users`: unique index on `user_id`
* `groups`: unique index on `chat_id`

**Test**

* Use a migration or initialization script, or a manual procedure, to create these collections and indices in the local Mongo instance.
* Inspect MongoDB with a client:

  * Confirm that the collections exist.
  * Confirm that the expected indices are present (e.g., unique index on user ID).

---

## Phase 4 – Telegram Bot Client & Routing

### Step 12 – Connect to Telegram using the chosen library

Using the Telegram bot library specified in the tech stack, wire the bot so that:

* It authenticates with Telegram using the token from configuration.
* It starts receiving updates via **long polling** with default subscriptions:

  ```
  message
  edited_message
  callback_query
  my_chat_member
  chat_member
  ```

**Test**

* Start the bot.

  * Expected: bot logs indicate that it is connected and listening for updates.
* From a Telegram client, send a simple message to the bot.

  * Expected: logs record the incoming update (chat ID, user ID, message text).

---

### Step 13 – Implement a basic router for incoming updates

Create a minimal routing layer that:

* Distinguishes between:

  * Direct messages to the bot
  * Group messages where the bot is present
* Routes commands (messages starting with “/”) to command handlers.
* Routes non-command messages to a generic handler (even if it currently does nothing).

**Test**

* Send `/start` and `/unknown` commands in a private chat and in a group.

  * Expected: logs clearly show which handler was invoked for each case.
* Send a random text message without a command.

  * Expected: generic handler is invoked (and logged) instead of a command handler.

---

### Step 14 – Register the bot owner (admin) on startup

On startup, the bot should:

* Read the bot owner’s Telegram user ID from configuration.
* Ensure there is a corresponding user record in Mongo (create it if missing).
* Mark this user as `owner` in the domain layer.
* If an existing owner differs from `BOT_OWNER`, demote the previous owner to `admin` and promote the configured `BOT_OWNER` to `owner` automatically.

**Test**

* Start the bot with a valid owner ID.

  * Expected: a user record exists in Mongo with this ID and a role/flag marking them as owner.
* Change the owner ID in config and restart.

  * Expected: the new owner ID is now reflected as owner in Mongo, and the old owner is automatically demoted to admin.

---

## Phase 5 – Domain Models & Base Behaviors

### Step 15 – Define base domain models

Define minimal domain models for:

* User:

  * Telegram user ID (required)
  * Role: `owner` (priority 3), `admin` (priority 2), `user` (priority 1)
  * Timestamps: created/updated
* Group:

  * Chat ID
  * Title
  * Joined/seen timestamps

No payment fields yet; just what is needed for initial registration and access control.

**Test**

* Create sample records for a user and group through the domain layer (either via a test function or a small script).

  * Expected: records are stored in Mongo with the fields defined above.
* Retrieve these records and verify that the data matches what was written.

---

### Step 16 – Implement user registration on first contact

Whenever the bot receives a message from a user for the first time (any update type):

* Check if the user exists in the database.
* If not, create a new user record with default role and status.
* Update last seen time on every incoming message from that user.

**Test**

* Start the bot with an empty users collection.
* Send a message to the bot from a new Telegram account.

  * Expected: a new user record is created with correct Telegram ID and metadata, and last seen time is set.
* Send another message from the same account.

  * Expected: no duplicate user record is created, and last seen time is updated.

---

### Step 17 – Implement group registration when bot is added

When the bot is added to a group or receives a message in a new group:

* Check if the group exists in the database.
* If not, create a group record with chat ID and title.
* Optionally, mark the time when the bot first saw the group.

**Test**

* Add the bot to a new group in Telegram.

  * Expected: a new group record is created in Mongo as soon as the first message involving the bot is received.
* Remove and re-add the bot to the same group.

  * Expected: the same group record is updated if needed but not duplicated.

---

### Step 18 – Implement a “/start” command for private chats

Implement a `/start` command handler that:

* Ensures the user is registered (using the registration logic above).
* Sends a welcome message explaining:

  * That this is a base version of the payment bot.
  * That more features (payments, dashboards, etc.) will be added later.
* Optionally, shows the user’s current role and status.
* Behavior in groups: stay silent (no reply), only log the event to avoid noise.

**Test**

* From a private chat, send `/start`.

  * Expected: user record is created (if missing) and a welcome message is returned.
* From a group chat, send `/start`.

  * Expected: behavior is consistent with your design (e.g., reply with a brief help message or ignore); verify it matches the documented behavior.

---

### Step 19 – Implement a basic “/ping” or health command

Add a simple command, such as `/ping`, that:

* Returns `pong` plus concise diagnostics:

  ```
  pong
  env: <APP_ENV or default production>
  uptime: <process runtime>
  mongo: ok|error
  ```
* Do not leak sensitive information; uptime derived from process start time.

**Test**

* Send `/ping` from a private chat.

  * Expected: bot replies with `pong`, env, uptime, and `mongo: ok`.
* Induce a temporary Mongo failure (e.g., stop the local Mongo container) and send `/ping` again.

  * Expected: the response includes `mongo: error` (or equivalent) but still returns a friendly message.

---

### Step 20 – Implement basic permission enforcement in handlers

Ensure that sensitive commands are restricted to the bot owner. Provide an owner-only `/status` command that returns live counts:

```
bot_status: running
env: <APP_ENV>
connected_chats: <groups collection count>
registered_users: <users collection count>
```

* Validate user’s role before executing the command.
* If the user is not authorized, reply with a brief “permission denied” message.
* Note: admin role assignment is manual in this phase (DB edit). Future phases can add `/promote` and `/demote`.

**Test**

* Send the admin command as the owner user.

  * Expected: command executes successfully and returns live counts from Mongo.
* Send the same command as a normal user.

  * Expected: bot returns a “permission denied” message, and no privileged action is taken.

---

## Phase 6 – Observability & Operational Readiness

### Step 21 – Log all incoming updates in a structured way

Update the Telegram handling pipeline so that every incoming update is logged with:

* update type (message, command, join/leave, etc.)
* chat ID and user ID
* timestamp
* handler name (where possible)

**Test**

* Interact with the bot using a variety of message types (commands, plain text, group joins).

  * Expected: logs show one entry per update with the fields above.
* Search logs by user ID or chat ID.

  * Expected: you can reconstruct a basic history of interactions from the logs.

---

### Step 22 – Implement a graceful shutdown flow

Ensure the bot can shut down cleanly when it receives a termination signal:

* Stop receiving new updates.
* Complete processing of in-flight updates.
* Close MongoDB client and any other resources.
* Log a clear “shutdown complete” message.

**Test**

* Start the bot and interact normally.
* Send a termination signal or stop the process according to your usual workflow.

  * Expected: logs show the shutdown sequence, and there are no panics or abrupt errors.
* After restart, verify that the bot still works and Mongo connections are not leaked.

---

### Step 23 – Add a minimal healthcheck for container environments

Expose an HTTP health endpoint:

* `GET /healthz` served on `HTTP_PORT` (default 8080; overridable by env)
* Success: `{"status":"ok"}`
* Mongo offline: `{"status":"degraded","mongo":"error"}` (HTTP 200 is acceptable; 503 optional if infrastructure requires restarts)

Docker healthcheck example:

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8080/healthz"]
  interval: 10s
  timeout: 2s
  retries: 3
```

**Test**

* Start the bot and curl `/healthz`.

  * Expected: `{"status":"ok"}` with HTTP 200.
* Stop Mongo or break the Telegram token.

  * Expected: `{"status":"degraded","mongo":"error"}` (HTTP 200 or 503) and logs a clear reason.

---

## Phase 7 – Dockerization & Local E2E Test

### Step 24 – Create a Docker image for the bot

Write a Dockerfile that:

* Builds the bot into a container image.
* Uses environment variables for config.
* Produces a small image suitable for deployment.

**Test**

* Build the Docker image locally.

  * Expected: build completes successfully with no errors.
* Run the container with environment variables pointing to the local Mongo container.

  * Expected: container starts, connects to Mongo, and logs that it is listening for updates.

---

### Step 25 – Define a local Docker Compose stack

Extend the existing Docker Compose file to include:

* MongoDB service (from earlier steps).
* Bot service using the built image.
* Shared network so the bot can reach MongoDB.

**Test**

* Run the Docker Compose stack.

  * Expected: both services start successfully.
* From Telegram, send `/start` to the bot.

  * Expected: full end-to-end flow works: bot replies, user is registered in Mongo, logs show the interaction.

---

### Step 26 – Final base-bot smoke test

Perform a manual checklist to validate all base-bot behaviors:

* Config validation (missing value causes clear failure).
* Successful connection to Mongo and Telegram.
* New user registration and group registration.
* `/start` and `/ping` commands behave as expected.
* Owner-only command is correctly restricted.
* Clean shutdown with no errors.

**Test**

* Run through the checklist above in a fresh local environment (e.g., another developer’s machine).

  * Expected: all items pass without any code changes.
* Document any deviations or friction points as tickets or notes for the next iteration.

---

Once all the steps and tests in this plan are complete and stable, the base bot is ready. Future iterations can then layer on the more complex features from the design document (payment integration, daily billing push, up-stream interfaces, scheduling, etc.) without altering the core skeleton. 
