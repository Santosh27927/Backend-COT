# Level 2: perpetual training

Confirmed training model: isolated long/short at 1x, then guided 2x/5x leverage
and liquidation exercises, followed by order controls. This is a simplified
training model, not a claim of Phoenix-equivalent risk calculations.

| Executable condition file | Practical objective |
|---|---|
| perpetual-long-short-1x.conditions.v1.json | Distinguish contract exposure from owning a spot asset |
| perpetual-isolated-leverage-2x.conditions.v1.json | Separate wallet cash, committed margin, and notional exposure |
| perpetual-isolated-liquidation-5x.conditions.v1.json | Observe isolated equity and liquidation under adverse scripted prices |
| perpetual-order-controls.conditions.v1.json | Apply price/trigger/time-in-force controls to the contract |

These versioned condition files are embedded and executed by the backend.
The implemented training model uses 1% maintenance, weighted entry basis, and an
isolated loss cap with explicit gap-backstop reporting. Worked examples and recovery
tests validate the model; real-venue risk equivalence remains deferred.

Trading/funding charges and slippage are zero; market quantity is unlimited.
Margin checks remain and leverage does not create spendable wallet cash.
IOC/FOK partial-fill differences remain deferred. Reports must distinguish an
earlier liquidation from the later scenario-end closing of surviving positions.
