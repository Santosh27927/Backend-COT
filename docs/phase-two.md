# Phase two: durable learning sessions and paper accounts

Historical phase-two delivery. Scripted execution was subsequently completed in
[phase three](phase-three.md), [phase four](phase-four.md), and [phase five](phase-five.md).

Confirmed with the user on 2026-11-24. This phase builds the stateful backend
foundation; the spot execution engine is the next milestone.

## Confirmed product rules

- Every new scenario account starts with **1,000 demo USD**.
- The initial timed scenario lasts **180 seconds**, measured by the backend.
- Revised progression: Level 1 spot/order controls, Level 2 perpetuals/order
  controls, Level 3 combined practice; all scripted. See [current roadmap](learning-roadmap.md).
- Closing the app, losing connectivity, or force-closing it does not pause a session.
- Accepted actions belong to the backend. A disconnected phone cannot submit new actions.
- At the lesson deadline, cancel pending orders and simulate closing remaining
  holdings, explaining execution prices and costs. The latest scripted mode has
  zero fees; cost realism is deferred. This is a game rule, not a
  claim that exchange orders normally close when a mobile timer ends.
- A restart creates a fresh session/account and retains earlier history.
- Production authentication is deferred. The existing local bearer identity
  remains in place for ownership tests; no new login provider is introduced.

## What this implementation delivers

- Server-authenticated player persistence and ownership checks.
- An immutable, versioned `spot-basics` account/session preparation exercise.
- Exactly one active session per player and scenario.
- Atomic session creation, paper account creation, balanced demo allocation,
  session events, and an idempotency receipt in PostgreSQL.
- USD minor units stored as integers and exposed as strings: `100000`, scale 2,
  represents USD 1,000.00. No floating-point monetary calculations.
- Database-enforced balanced journals and immutable financial/event history.
- A three-minute deadline persisted in PostgreSQL; the phone cannot extend it.
- A game-service worker that completes overdue sessions without a player request.
- Startup recovery and a session GET that also reconciles an overdue session.
- Durable final account reports and cursor-based session events.
- Explicit restarts, retry protection, retained history, and conflict responses.

The preparation exercise has `executionEnabled: false`. It cannot place orders,
hold assets, charge trading fees, or calculate trade PnL. Its completion reports
therefore correctly show zero pending orders canceled and zero holdings closed.
Enabling execution is refused by this foundation profile. The chosen end policy
is persisted now; actual cancellation, closing fills, price explanations, and
fees must be implemented and tested with the execution engine before activation.

Finishing a timed session does not automatically prove learning mastery or
unlock Town Hall 2. Unlock criteria and assessment content remain to be designed.

## Mobile disconnect and reconnect flow

1. The app submits a command with a persistent `Idempotency-Key`.
2. The backend commits its state and receipt before returning success.
3. If the reply is lost, the app retries the same command with the same key.
   A changed command needs a new key. Do not silently queue new trades offline.
4. The backend timer continues independently of the app connection.
5. On reconnect, fetch the session by ID and fetch events after the last stored
   sequence. If the ID was lost, list the player's recent sessions.
6. An active session returns its original deadline and server time. An expired
   session returns its durable completed report and history.

A replayed POST returns the original receipt, including its original state and
server time. `Idempotency-Replayed: true` identifies it; fetch the session for
current state. A completed session cannot be renewed by repeating its start key.

## Backend failure and timing limits

Phone disconnection and backend outage are different failures. PostgreSQL retains
deadlines, receipts, accounts, and events across process restarts. The worker
reconciles due sessions immediately after restart and then polls every second.
The scheduled completion time and actual processing time are both recorded.
This is eventual processing, not a real-time availability guarantee.

When trading is added, recovery must also persist market inputs and execution
progress. Do not invent missed fills using only the latest market price. New
commands must check the server deadline while holding the relevant state lock.
At the later realistic-market stage, unusable price/liquidity data must produce a
settlement-pending state rather than a fictional completed report. The newly
selected scripted mode uses its authored ending price with unlimited quantity.

## Ownership and package responsibilities

```text
internal/players/           # Persist and lock the authenticated player
internal/scenarios/         # Versioned lesson rules and snapshots
internal/sessions/          # Session commands, reports, events, and worker
internal/trading/accounts/  # Paper account creation and balance reads
internal/trading/ledger/    # Balanced allocations and financial history
internal/httpapi/           # Authentication boundary and HTTP mapping
internal/testsupport/       # Isolated databases for tagged boundary tests only
```

Phase-two routes and the worker run in game-service. The coordinator invokes the
account/ledger modules in the same PostgreSQL transaction. Those modules own
financial writes. This is a deliberate local transaction boundary, not a network
call to trading-service and not independent database isolation. Trading-service
retains its foundation endpoints until execution work adds its responsibilities.
Later extraction requires a reliable command/event handoff and explicit ownership;
do not split one atomic financial operation into uncoordinated HTTP writes.

## Configuration and versioning

Default rules live in `internal/scenarios/catalog.go`: USD 1,000.00 and 180 seconds.
They are validated and snapshotted as a scenario version at session creation.
Existing scenario versions cannot be updated. Change rules using a new version
and deploy compatible code; do not edit an existing session's balance or deadline.
There is no runtime curriculum administration UI in this phase.

Requests cannot supply their own initial balance, player ID, duration, fees,
completion policy, or execution flag. Infrastructure/startup settings remain in
validated environment configuration, separate from lesson rules.

## HTTP surface

All learning routes are on game-service and use the existing local bearer token.

| Method and route | Purpose |
|---|---|
| GET /v1/scenarios | Current preparation scenario and its capabilities |
| POST /v1/sessions | Start once using a required Idempotency-Key |
| GET /v1/sessions | Latest 100 owned sessions, newest first |
| GET /v1/sessions/{id} | Current state, account, server time, and final report |
| POST /v1/sessions/{id}/restart | Explicitly end the current attempt and create a new one |
| GET /v1/sessions/{id}/account | Owned session's current paper balance |
| GET /v1/sessions/{id}/ledger | Allocation history as player-facing credits |
| GET /v1/sessions/{id}/events?after=0&limit=100 | Ordered events after a sequence |

POST bodies are respectively `{"scenarioId":"spot-basics"}` and `{}`.
An Idempotency-Key is 8-128 ASCII letters/digits or `.`, `_`, `:`, `-`, scoped
to the player across these commands. Concurrent starts with different keys
conflict while an active attempt exists. Ownership failures return 404.

The ledger API currently exposes the single demo-allocation journal per account.
Its balancing source entry is internal; it is not a second player deposit.

## Verification

With the COT-only database running and migrated:

```sh
make check build test-race test-integration test-smoke
```

Boundary tests cover concurrent retry allocation, payload/key conflicts, account
ownership, rollback after a forced allocation failure, database ledger invariants,
expiry, concurrent completion, explicit restart history, immutable scenario
versions, event cursors, and recovery after killing an owned test process.
Test databases and processes are isolated and removed after each test.

## Later confirmed decisions and remaining execution work

The user subsequently selected XRP, SOL, ETH and scripted markets across learning
Levels 1-3, with no fees/funding charges, slippage, or market-depth caps. Level 2
uses isolated long/short with 1x then guided 2x/5x leverage and liquidation.
IOC/FOK partial-fill differences wait until realistic-market practice.

These decisions do not alter the published preparation profile or its session
history. Actual execution was implemented afterward in backend phases 3-5. The current implementation
plan is [phase three](phase-three.md); deferred realism is listed in
[market modes](market-modes.md).

Training condition data, exact asset precision, the versioned liquidation model,
and practical objective/unlock checks were implemented and validated in phases 3-5. Production
identity, live adapters, frontend integration, cloud, and multiplayer remain later work.
