# Phase three: scripted spot trading

Completed locally on 2026-11-24, before beginning phase four.

## Built

- Executable, versioned XRP/SOL/ETH spot lessons with authored prices.
- Market buy/sell, GTC limit orders, IOC/FOK immediate decisions, and stop-limit activation.
- Exact cash/asset accounting, immutable balanced journals, and reservations against double spending.
- Idempotent order/cancel/finish APIs, ordered events, and current trading snapshots.
- Background processing, historical scripted fills after recovery, automatic expiry/restart settlement.
- Practical objective tracking and prerequisite lesson locks, separate from merely finishing a timer.
- The original preparation scenario and its checksum/history remain compatible.

## Decisions

- USD 1000 and 180 seconds per new lesson. Zero costs/slippage and unlimited market quantity.
- USD execution accounting uses scale12; quantities/prices use scale6, so products are exact.
- IOC/FOK partial-fill differences remain deferred. Stop-limit currently supports GTC.
- Private future script frames stay server-side; requests cannot choose the clock.
- One transaction commits trading state, ledger, events, and successful retry receipts.
- Trading state is a bounded per-session aggregate; user order cap256 is a resource limit, not a liquidity cap.
- Validation failures create no accepted order. Replays return their original receipt; GET retrieves current state.

## Validation

Formatting/vet/unit checks, PostgreSQL integration with race detection, and built-process smoke checks passed.
Covered exact arithmetic, reservations, overspend/oversell races, ownership, retries, cutoff, stop/limit/IOC/FOK rules,
progression, and crash recovery. A restarted process closed a held SOL at the scripted ending price without
a player request and did not duplicate its financial posting.

## Next

Phase four adds the confirmed isolated perpetual training model. Phase five combines products.
Live data, realistic execution costs, public authentication, Android UI, and production deployment remain later work.
