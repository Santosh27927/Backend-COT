# Level 3: combined practice

Combine previously taught spot and perpetual concepts. Prices remain scripted;
costs and market-depth restrictions stay disabled until realistic-market practice.

| Executable condition file | Practical objective |
|---|---|
| combined-spot-perpetual-exposure.conditions.v1.json | Compare spot ownership and isolated contract exposure |
| combined-wallet-margin-accounting.conditions.v1.json | Track available cash, reservations, margin, and outcomes |
| combined-session-completion.conditions.v1.json | Reconnect to a completed mixed lesson and explain its closing events |

These versioned condition files are embedded and executed by the backend.
Use explicit cash/collateral ownership and coordinated ledger updates. Spot buys
cannot reuse funds already committed as isolated margin. Enable features per
exercise as they are taught.

Complete the scripted progression before live-data integration. That later stage
needs a separate validated execution/risk profile, not only a new price feed.
