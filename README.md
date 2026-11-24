# Backend-COT

Local Go backend for the COT trading-learning game. Android is the first frontend
target; the backend now supports the scripted learning progression through Levels 1-3.

## Implemented

| Backend phase | Delivery |
|---|---|
| 1 | Service entry points, validated config, PostgreSQL, migrations, diagnostics, local authentication |
| 2 | Players, owned timed sessions, paper accounts, immutable ledger, retries, history and reports |
| 3 | Scripted spot trading and market/limit/stop-limit/GTC/IOC/FOK lessons |
| 4 | Isolated long/short, 1x/2x/5x leverage, PnL, liquidation and perpetual order controls |
| 5 | Mixed spot/perpetual lessons, shared wallet/collateral and course progression |

There are eleven executable lessons plus the original preparation scenario.
Each new lesson starts with **1,000 demo USD** and a **180-second** server clock.
Prices are authored and hypothetical. All three levels have zero trading/funding
charges, no slippage and unlimited simulated market quantity. Wallet, ownership,
reservation and margin checks remain.

This is a local teaching backend. The risk model is deliberately simplified;
live data, actual venue equivalence, real transactions, Android UI, public
authentication and cloud deployment are separate milestones.

## Start locally

Requirements: Go 1.27.4, Docker Desktop with Compose running, Make, a POSIX shell and OpenSSL.

```sh
make init-local
make db-up
make migrate
make run-game
```

Game-service uses `127.0.0.1:8080`. In a second terminal, `make run-trading`
starts the separate trading entry point on `127.0.0.1:8081`; its routes remain
foundation endpoints. Current learning/execution coordination runs in game-service
against one database transaction. PostgreSQL uses loopback port 55432.

`make init-local` creates private, gitignored credentials in `.env` and preserves
existing files. The development identity is an opaque bearer token, not a JWT.
Only explicit local/test environments are supported. LAN testing requires an
explicit listener and `COT_ALLOW_LAN=true`.

## Use the learning API

1. `GET /v1/scenarios` lists lesson IDs/prerequisites; `GET /v1/progress` restores owned completed lessons.
2. `POST /v1/sessions` with `{"scenarioId":"spot-market-buy-sell"}`.
3. Submit commands to `POST /v1/sessions/{id}/orders`.
4. Retrieve current state with `GET /v1/sessions/{id}`.
5. Finish explicitly or let the backend deadline settle the session.
6. Retrieve missed events using `GET /v1/sessions/{id}/events?after=...`.

All learning routes use the configured local bearer token. Mutating requests require
a stable `Idempotency-Key`; replayed responses are original receipts, so fetch the
session for current state. The app cannot pause the timer or change the script clock.

See [order examples and API rules](docs/trading-api.md) and the
[HTTP contract](contracts/http/learning.openapi.json).

Both processes retain `GET /health/live`, `GET /health/ready`, and `GET /v1/me`.
Readiness verifies database migration history; liveness is independent of the database.

## Validate

```sh
make check
make test-race
make test-integration
make test-smoke
```

Integration tests use PostgreSQL and the race detector. Smoke tests build all three
executables and exercise real HTTP processes, crash recovery, and the complete
eleven-lesson course. Tests create/remove only randomly named `cot_test_*` databases.
They do not reset `cot`. Test processes use temporary loopback ports and are stopped.

`make migration-status` checks readiness. `make db-stop` stops only the COT fixture
and retains its volume.

## Structure

```text
cmd/                         # Game, trading and migration entry points
internal/app/                # Startup and graceful shutdown
internal/config/             # Validated local process settings
internal/authentication/     # Existing local identity boundary
internal/players/            # Player persistence and command serialization
internal/scenarios/          # Published scenario metadata and private snapshots
internal/sessions/           # Transactions, ownership, receipts, reports and worker
internal/trading/accounts/   # Paper accounts
internal/trading/ledger/     # Balanced immutable single-asset journals
internal/trading/execution/  # Deterministic spot/perpetual/mixed training engine
internal/exact/              # Exact decimals and checked integer arithmetic
internal/httpapi/            # HTTP contracts, authentication and error mapping
simulation/scripted/         # Embedded, versioned authored conditions
migrations/                  # Immutable embedded SQL
contracts/http/              # Language-independent HTTP contracts
dev/                         # Backend-only PostgreSQL fixture
tests/                       # Database and built-process boundaries
docs/                        # Phase summaries, architecture and operating notes
```

The future sibling directory `/Users/santosh/git/COT/Live-Market-COT/` remains empty.

## Phase records

Each summary records what was built, decisions, validation and next work.

- [Phase 1](docs/phase-one.md) and [validation](docs/validation.md)
- [Phase 2](docs/phase-two.md) and [validation](docs/phase-two-validation.md)
- [Phase 3](docs/phase-three.md)
- [Phase 4](docs/phase-four.md)
- [Phase 5](docs/phase-five.md)
- [Architecture](docs/architecture.md)
- [Learning roadmap](docs/learning-roadmap.md)
- [Market-mode boundaries](docs/market-modes.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Foundation HTTP contract](contracts/http/foundation.openapi.yaml)

Related repositories: [Frontend-COT](https://github.com/Santosh27927/Frontend-COT),
[DevOps-COT](https://github.com/Santosh27927/DevOps-COT).
