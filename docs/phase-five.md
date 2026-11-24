# Phase five: combined scripted practice

Completed locally on 2026-11-24 after phases three and four were validated.

## Built

- Three mixed lessons combining spot ownership, perpetual exposure, shared cash/margin, and final settlement.
- One wallet per lesson: spot spending, pending reservations, and isolated collateral cannot reuse committed funds.
- Liquidation preserves owned spot assets; expiry closes surviving positions and holdings together.
- Persisted practical progression through all eleven executable lessons, with prerequisites exposed in the catalog.
- Final reports, reconnect history, and fresh-funds restarts retain prior outcomes.

## Decisions

- Reuse the tested spot/perpetual engine and one transaction boundary rather than duplicate accounting.
- Every new lesson starts with USD 1000; learning levels remain scripted and cost-free.
- At the fixed deadline, cancel pending orders before the endpoint price can create a new conditional fill.
  Use that endpoint price to evaluate risk and close existing exposure.
- Session completion and practical mastery are separate. No profit gate or quiz was introduced.
- The live-market repository directory remains completely empty.

## Validation

Formatting/vet/unit, default race, PostgreSQL integration race, and built-process smoke checks passed.
The API smoke course completed all eleven spot/perpetual/mixed lessons and persisted their progress.
Regression checks cover shared-wallet races, collateral reuse, preserved spot assets after liquidation,
mixed settlement, worker progress while a player command lock is held, and deadline cancellation precedence.
Existing preparation and crash-recovery checks continue to pass.

## Next

- Android frontend integration and learning feedback/graphics.
- Separate live-data adapters and paper execution profiles with realistic costs, liquidity, and venue rules.
- Production identity, deployment, observability/load work, and multiplayer as separately scoped milestones.

The backend is a local learning implementation, not a live exchange or Phoenix-equivalent risk engine.
