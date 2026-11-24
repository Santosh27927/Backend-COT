# Current backend architecture

Updated 2026-11-24 after backend phases 3-5.

## Runtime boundary

Go owns the backend core. PostgreSQL owns durable sessions, receipts, journals,
progress and trading state. game-service currently coordinates learning/execution
in-process using one transaction. trading-service remains a separate foundation
entry point; no distributed financial transaction is implied.

Separate packages own players, scenarios, sessions, accounts, ledger, execution,
HTTP and startup. Future service extraction requires a reliable command/event
handoff and explicit ownership. Current financial writes stay coordinated.

## Scripted execution

There are eleven executable lessons in `simulation/scripted/`, plus the unchanged
preparation profile. Go embeds validated versioned conditions. A session stores
its private condition snapshot, progress cursor and state, so changed source files
do not rewrite an existing attempt. Future price frames are not exposed by HTTP.

The execution package is a deterministic state machine. It produces state changes,
single-asset postings and public events without network or SQL dependencies. The
session coordinator commits them together under the owned session lock, verifies
cash/asset agreement with the ledger, and saves successful command receipts.

Trading state is a bounded per-session JSON aggregate, not an unbounded global book.
Cash accounting uses scale12 and asset quantity/price scale6. Financial JSON values
are strings. Ledger journals have exactly two balanced immutable entries per asset;
operation IDs connect fills and financial postings. The original preparation account
keeps scale2 and its original published snapshot checksum.

## Wallet and positions

Spot fills transfer virtual cash and owned quantities. Pending orders reserve
cash/assets. Perpetual margin reserves wallet capacity rather than depositing notional.
A mixed lesson shares the same available cash; committed collateral cannot be spent twice.

One-way isolated positions use weighted entry notional. Partial closes preserve
remaining basis/margin. The training model uses 1% maintenance and a loss cap at
isolated collateral, with explicit gap backstop reporting. It is not a Phoenix
risk implementation. Ordinary frame processing evaluates risk before pending fills
and again afterward.

All levels have zero fees/funding charges, zero slippage and unlimited market
quantity. IOC/FOK partial-fill differences are deferred. Limit and trigger rules,
wallet checks, deadlines, ownership and idempotency remain enforced.

## Clock, completion and recovery

Database time determines command admission and the session deadline. The worker
processes active scripted sessions and overdue preparation sessions without a phone
connection. A durable cursor replays missed frames chronologically after restart.

At the fixed deadline, pending orders cancel before the endpoint price can create
a new conditional fill. That price is used for risk and closing existing exposure.
Explicit finish/restart uses current scripted prices. A terminal report and
practical objective progress commit with settlement. Completion alone does not
prove mastery. Player command locks permit worker FK progress updates without a
lock-order deadlock.

The app retries accepted commands using their original key and then fetches
current state/events. New offline trades must not be silently queued.

## Identity and diagnostics

Existing local bearer identity determines ownership; requests cannot select a
player through bodies or headers. Production authentication is deferred.
Configuration requires local/test mode, safe credentials and explicit LAN opt-in.

JSON logs use service, request reference, route, status and duration, while command
logs include session/account references. Credentials and private bodies/queries
are omitted. Liveness is database-independent; readiness verifies migration history.
Shutdown allows 10 seconds against the 5-second header deadline.

## Remaining boundaries

- Android/iOS frontend and graphics integration.
- Live spot data and a separate Phoenix Rise perpetual adapter.
- Real venue metadata, precision, fees, liquidity, funding and risk behavior.
- Production identity, RBAC, cloud, load/availability and multiplayer.
- The local database credential is shared; it is not production role isolation.
- JSON aggregate storage and full snapshots are intended for bounded local lessons;
  larger-scale projections, pagination and worker scheduling need load evidence.

The future `Live-Market-COT` sibling directory is empty. Live prices alone will
not implement venue-accurate fills or costs, and the current plan remains paper-only.

See [API reference](trading-api.md), [market modes](market-modes.md) and
[phase summaries](phase-five.md).
