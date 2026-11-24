# Phase-two validation

Validated locally on 2026-11-24 with Go 1.27.4 and PostgreSQL 17.

## Passed commands

- `make check`: formatting, go vet, and existing unit checks.
- `make test-race`: default-package race checks.
- `make test-integration`: PostgreSQL boundary tests, now with the race detector.
- `make test-smoke`: all three executable builds and process-level checks.
- `make migration-status`: local migration history ready through migration 002.
- `docker build --build-arg SERVICE=game-service -t cot-backend/game-service:phase-two .`.
- Learning OpenAPI JSON parsing and local reference resolution.
- Tracked/new-file whitespace checks and private `.env` permissions (0600).

## Covered behavior

1. Concurrent requests with the same key create one account and allocation.
2. Concurrent starts with different keys admit one active scenario session.
3. Keys are scoped to the player; changed input conflicts with an existing key.
4. A lost reply can be recovered from its durable command receipt.
5. A forced allocation failure rolls back player/session/account/receipt writes.
6. A failed restart leaves the prior active session and account unchanged.
7. Ledger history is immutable; incomplete or unbalanced journals fail at commit.
8. One player cannot read or restart another player's session/account/history.
9. Duplicate completion workers publish only one terminal event.
10. Restarting creates fresh funds while retaining previous reports and history.
11. Changed rules cannot silently overwrite a published scenario version.
12. Event cursors return the next ordered records without replaying earlier ones.
13. An owned test process is killed after allocation, then restarted after its
    schedule is moved into the past in its isolated database. The background
    worker completes the session without a player request; the app-facing GET
    returns its report and ordered history. Retrying the start does not fund twice.
14. Existing liveness, readiness, authentication, redaction, and shutdown checks pass.

The final fixture check found zero leftover `cot_test_*` databases. Child test
processes were stopped. The COT-only PostgreSQL container remains running.

## Limits of this verification

This validates account/session preparation, not spot execution, order matching,
fees/PnL, market-data recovery, final trade settlement, production login, frontend
behavior, load capacity, cloud deployment, or multiplayer. The tests use isolated
schedule changes instead of waiting three minutes; the production default remains
180 seconds. The chosen cancel-and-close policy is recorded but trading is disabled.
