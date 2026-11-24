# Phase four: scripted perpetual training

Completed locally on 2026-11-24 after phase three was validated.

## Built

- Guided long/short, isolated leverage, liquidation, and perpetual order-control lessons.
- 1x introduction, then 2x/5x; position increases, partial closes, and explicit reduce-only closing.
- Separate cash, available cash, committed margin, entry notional, current exposure, and PnL.
- Background risk processing and historical liquidation after restart/reconnect.
- Realized PnL posts atomically to the existing immutable ledger; leverage never deposits notional as cash.
- Practical progression through the spot prerequisites and perpetual lessons.

## Decisions

- One net position per asset. Direction and leverage cannot silently change while it remains open.
- Weighted entry-notional accounting; partial closes release proportional basis/margin, with the remainder preserved.
- Training maintenance margin is 1% of current notional, rounded upward to an accounting atom.
- At each scripted frame, risk is evaluated before pending fills and again afterward.
- Liquidation losses are capped at that position's isolated collateral. Any larger gap is explicitly reported
  as a training backstop. It does not charge the rest of the wallet.
- Zero trading/funding charges, no slippage, and unlimited simulated market quantity remain.
- These are versioned teaching rules, not Phoenix-equivalent risk calculations.

## Validation

Unit/golden accounting and PostgreSQL integration race checks passed. The built-process smoke course
completed all eight spot/perpetual lessons through the public API with real prerequisites.
It verified worker-driven liquidation without a player request and persisted course progress.
Coverage includes long/short PnL, increase/reduce accounting, collateral release, one-way protection,
queued reduce reservations, gap liquidation, retry safety, and one terminal liquidation event/journal.

## Next

Phase five combines products under one wallet and tests shared collateral, mixed settlement, and final progression.
Live market data, real venue rules/costs, Android UI, public authentication, and cloud remain separate work.
