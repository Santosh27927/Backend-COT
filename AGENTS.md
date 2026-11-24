# COT backend working rules

## Scope and authorization

- Work only on COT and dependencies necessary for this project. Do not change other repositories.
- The user authorizes routine local COT implementation, dependency setup, and verification without repeated confirmation. Tool sandbox restrictions still apply.
- Current work is local. Pushes, merges, cloud deployments, and live trading are outside the current task.
- For a later authorized GitHub operation, verify the account is Santosh27927. Use the separate gh-cot profile with GH_TOKEN and GITHUB_TOKEN unset; never change the default account.

## Architecture

- Go owns the backend core; PostgreSQL owns durable state.
- Keep packages focused. Add business modules when their behavior is implemented.
- Game, trading, and migration entry points share the current foundation packages.
- Future order, position, margin, and ledger mutations need one coordinated consistency boundary.
- Phoenix SDK integration belongs behind an adapter; the current foundation has no signer or external venue calls.
- Development bearer authentication is local/test only. Do not describe it as production authentication.
- Android first, iOS later; frontend graphics are owned by the other contributor.

## Verification and continuity

- Read README.md, docs/phase-one.md, and docs/phase-two.md, docs/phase-three.md, docs/phase-four.md, docs/phase-five.md, docs/trading-api.md, and docs/market-modes.md before changing startup, sessions, or execution behavior.
- Use the COT-only Docker Compose fixture. Never stop unrelated services or reset an existing database to make a test pass.
- Run checks appropriate to the change. Foundation changes use make check, make test-race, make test-integration, and make test-smoke with the local fixture ready.
- Keep credentials out of tracked files, logs, and reports.
- Record scope, completed work, and remaining decisions in documentation so another chat can continue safely.

- Confirmed phase-two defaults: 1,000 demo USD, 180 seconds, spot-first curriculum, server-owned deadlines, retained restart history, cancel-and-close at lesson end once execution exists.
- Production authentication is deferred; preserve the existing local identity boundary. Do not enable spot execution in the preparation profile or claim unimplemented order/fee behavior.

- Latest learning progression supersedes the earlier plan: Level 1 spot/order controls, Level 2 perpetuals/order controls, Level 3 combined; all scripted.
- No trading/funding charges, slippage/spread costs, or market-depth caps in scripted Levels 1-3. Preserve wallet/ownership/reservation/margin/deadline checks.
- IOC/FOK partial-fill differences are deferred; do not add hidden quantity caps. Perpetual training: isolated long/short, 1x then guided 2x/5x leverage/liquidation.
- User explicitly requested an empty future integration directory at /Users/santosh/git/COT/Live-Market-COT. Keep it empty until integration work is requested; it is not a Git-initialized repo yet.
- Scripted conditions and spot/perpetual/mixed execution are implemented through backend phase five. Preserve the original preparation scenario/checksum. Private future frames must never enter HTTP responses.

- Training risk v1: 1% maintenance, one-way isolated positions, immutable leverage while open, weighted entry basis, proportional partial closes, isolated loss cap with explicit gap backstop. No venue-equivalence claims.
- USD execution scale12, asset quantity/price scale6; use checked exact arithmetic and strings in public financial fields. Pending orders cancel before endpoint fills at the fixed deadline.
- Validate phase changes sequentially. The real HTTP smoke course covers all eleven lessons; test databases/processes are owned and isolated. Read phase summaries before changing behavior.
