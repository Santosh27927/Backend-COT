# Scripted learning conditions

Executable versioned conditions and specifications for learning Levels 1-3. Prices are
authored and hypothetical; no live API calls belong here. Go embeds eleven validated JSON conditions; the backend executes them with a
persisted cursor and private snapshots. Lesson READMEs describe the objectives.

| Directory | Content |
|---|---|
| [level-01-spot](level-01-spot/README.md) | Spot buy/sell, then order conditions/time in force |
| [level-02-perpetuals](level-02-perpetuals/README.md) | Long/short, isolated margin, 1x/2x/5x leverage, liquidation, order controls |
| [level-03-combined](level-03-combined/README.md) | Combined spot and perpetual practice |

Conditions use descriptive names ending in `.conditions.v1.json`, with
`sourceMode: scripted`. Bump versions when published conditions change. No SDK,
credentials, or signing logic belongs here.

All levels have zero fees/funding charges, no slippage/spread cost, and unlimited
simulated market quantity. Wallet, ownership, margin, price/trigger, and deadline
checks remain. IOC/FOK partial-fill differences are deferred.

The separate future directory `/Users/santosh/git/COT/Live-Market-COT/` stays empty.
See [market modes and deferred realism](../../docs/market-modes.md).
