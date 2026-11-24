# Scripted simulation and future live-market integration

Confirmed on 2026-11-24. Learning Levels 1-3 use scripted markets. Historical
market replay and live prices are not selected for these levels.

| Rule | Current decision |
|---|---|
| Assets | XRP, SOL, ETH |
| Quote / funding label | Demo USD |
| Starting funds | 1,000 demo USD per new scenario |
| Default duration | 180 seconds, backend-owned |
| Price source | Authored, versioned scenario conditions |
| Trading and funding charges | Zero |
| Slippage / spread cost | None; eligible fills use the scripted price |
| Available market quantity | Unlimited; no order-book depth caps |
| Spot spending | Available cash, accounting for reservations |
| Spot selling | Owned, unreserved quantity |
| Perpetual model | Training-only isolated long/short; 1x introduction, then guided 2x/5x leverage and liquidation |
| Order conditions | Preserve price, trigger, and time-in-force rules |
| Session end | Cancel pending orders; close at the scripted ending price; retain report/history |

Disabling costs and market-depth caps does not disable authorization, deadlines,
idempotency, atomic balance updates, ownership, or margin checks. Market liquidity
and the player's wallet balance are different concepts.

Perpetual collateral and notional exposure must be separate. Leverage does not
deposit extra spendable money. The implemented training model uses 1% maintenance of current notional, weighted
entry basis, and an isolated loss cap with explicit gap-backstop reporting. Golden
examples and recovery tests verify it. No Phoenix-equivalent risk accuracy is claimed.

The user chose unrestricted IOC/FOK scripts too: teach immediate fill/cancel and
defer partial-fill differences. Do not add hidden quantity caps or random outcomes
to imitate that difference.

## Local boundaries

```text
COT/
  Backend-COT/
    simulation/scripted/
      README.md
      level-01-spot/README.md
      level-02-perpetuals/README.md
      level-03-combined/README.md
    docs/
      market-modes.md
      phase-three.md
  Live-Market-COT/              # Empty future repository directory
```

The scripted directory contains eleven executable, validated JSON condition files
embedded by Go, plus lesson specifications. The engine consumes private per-session
snapshots and persists its cursor. Published profiles and old attempts are immutable.

Use descriptive future names such as `spot-market-buy-sell.conditions.v1.json`
and `perpetual-isolated-liquidation-5x.conditions.v1.json`. Declare the source
mode (`scripted`), ID/version, objective, hypothetical prices, time offsets,
expected actions, and training rules. Persist the condition version and execution
progress. The backend session clock is authoritative; reconnecting never restarts
the script.

Domain code should consume a normalized market input contract. Scripted inputs
and a later live adapter implement that boundary without placing SDK types or
credentials inside orders/accounts/lessons. Keep acquisition separate from risk
and execution rules: changing the price feed does not automatically change fees
or the fill model.

`Live-Market-COT` is reserved for a later separate Git repository. It is outside
the backend repository and has no GitHub remote. Git cannot preserve a truly empty
directory in a commit; files will be added when integration starts. It has no
placeholder now, honoring the request to keep it empty.

## Deferred until realistic-market practice

- Actual venue instruments, quote/collateral assets, metadata, price/quantity
  steps, and minimum sizes.
- Real market inputs, timestamps/sequences, freshness, reconnects, and recovery.
- Bid/ask, finite liquidity, partial fills, slippage, and price protection.
- IOC partial-fill versus FOK full-fill behavior.
- Maker/taker fees, fee assets, rounding, funding, and settlement schedules.
- Venue-accurate maintenance requirements and liquidation calculations.
- Explicit closing-price/liquidity rules and a pending state for missing data.
- An appropriate spot adapter and a separate Phoenix Rise perpetual adapter.

The later live-data stage still uses paper funds. No signer, deposits, or real
order submission is authorized by the current plan.
