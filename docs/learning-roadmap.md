# Learning progression

Implemented locally through the scripted backend course on 2026-11-24.
Learning levels are separate from backend implementation phases.

| Learning level | Executable lessons |
|---|---|
| 1: spot | Market buy/sell; GTC price waiting; IOC/FOK immediate decisions; stop-limit activation |
| 2: perpetuals | Long/short at 1x; isolated 2x exposure; 5x liquidation; perpetual order controls |
| 3: combined | Spot versus contract exposure; shared wallet/margin; mixed completion/recovery |

The assets are XRP, SOL and ETH. Each new lesson starts with 1,000 demo USD and
a default 180-second backend clock. Prices are authored, hypothetical and labeled
scripted. Fees/funding charges, slippage and market-depth caps remain disabled.
Wallet, reservation, ownership and margin checks remain.

Each lesson has practical goals and prerequisites in its versioned condition file.
The public catalog exposes prerequisite IDs. A terminal session records learning
progress only when its actions meet those goals. There is no quiz or profit gate.
The eleven-lesson built-process smoke course verifies the actual progression.

## Order-policy limit

GTC can wait for a qualifying scripted price. IOC/FOK show immediate fill/cancel
decisions, but unrestricted eligible quantity makes their successful fills full.
Partial-fill differences are explicitly deferred. Stop-limit activation preserves
its limit condition. Order type and time in force remain separate fields.
See [official spot definitions](https://developers.binance.com/en/docs/products/spot/enums).

## Cost and risk limits

The isolated training model uses 1x/2x/5x, weighted entry basis and 1% maintenance.
Losses are capped at that position's collateral; a gap beyond it is reported as a
training backstop. These are teaching rules, not real-venue guarantees.

Closing the app does not pause accepted backend actions. At expiry, pending orders
cancel and existing exposure settles using the scripted endpoint price. Reports,
events and old attempts remain available after reconnect/restart.

## Following stage

Introduce real market data and a separately validated paper execution/cost profile
after scripted learning. Rise covers Phoenix perpetuals; spot needs an appropriate
separate adapter. See [Rise SDK](https://docs.phoenix.trade/sdk/rise).

The future sibling `/Users/santosh/git/COT/Live-Market-COT/` remains entirely empty.
No real funds, signer, SDK or remote integration was added.

- [Executable conditions](../simulation/scripted/README.md)
- [Phase 3](phase-three.md), [phase 4](phase-four.md), [phase 5](phase-five.md)
- [Market modes](market-modes.md) and [API](trading-api.md)
