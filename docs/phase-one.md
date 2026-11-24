# Phase one: local Go foundation

## Acceptance criteria

1. Game and trading executables build from one Go module.
2. Invalid configuration fails with a useful, credential-free error.
3. Default listeners are loopback-only; LAN binding needs explicit opt-in.
4. Development authentication is refused outside explicit local/test environments.
5. `/v1/me` requires the configured token and derives player identity server-side.
6. `/health/live` stays available independently of PostgreSQL.
7. `/health/ready` returns 503 for a missing database/schema or altered migration history.
8. Migration reruns are idempotent, concurrent runners serialize, and failed DDL rolls back.
9. HTTP responses and JSON logs carry a safe request reference.
10. Unit, database integration, race, and process-smoke checks exercise these boundaries.

## Delivered files

- `go.mod` and `go.sum`: pinned module dependencies.
- `cmd/game-service`, `cmd/trading-service`, `cmd/migrate`: entry points.
- `internal`: configuration, auth, HTTP, database, and process lifecycle.
- `migrations`: embedded SQL and runner.
- `.env.example` and setup scripts: private local configuration generation.
- `dev/compose.yaml`: isolated PostgreSQL dependency fixture.
- `contracts/http/foundation.openapi.yaml`: current HTTP surface.
- `Dockerfile`: optional container build for each executable.
- Tests and operating documentation.

## Startup configuration

| Variable | Requirement |
|---|---|
| `COT_ENV` | Explicit `local` or `test` |
| `COT_AUTH_MODE` | Explicit `dev-token` |
| `COT_DEV_AUTH_TOKEN` | Generated 32-512 character token, no whitespace or placeholder |
| `COT_DEV_PLAYER_ID` | 1-64 letters/digits/hyphens/underscores |
| `COT_DATABASE_URL` | Valid PostgreSQL URL with host and database |
| `COT_DB_MAX_CONNECTIONS` | 1-32, default 4 |
| `COT_HTTP_ADDR` | Defaults to game :8080 / trading :8081 on 127.0.0.1 |
| `COT_ALLOW_LAN` | Explicit true to allow non-loopback listeners |
| `COT_LOG_LEVEL` | debug, info, warn, or error; default info |

`COT_DB_PORT` and `COT_POSTGRES_PASSWORD` configure the local Docker fixture.
They do not dynamically resize infrastructure. No live Phoenix credentials are
required for this phase.

## Next phase

Before financial domain implementation, select the initial market/account
profile and independently checked examples. Then build session preparation,
virtual allocations, immutable ledger entries, command idempotency, and the
first order lifecycle. Keep the Phoenix SDK adapter outside the Go core.

Phase two now implements the account/session foundation without venue execution.
See [phase-two scope](phase-two.md); market-specific choices remain deferred until
actual trading implementation.
