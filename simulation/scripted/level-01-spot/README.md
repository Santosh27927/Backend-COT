# Level 1: spot trading

Introduce one exercise at a time using XRP, SOL, and ETH at hypothetical demo-USD
prices. Each scenario starts with 1,000 demo USD and a backend-owned 180-second timer.

| Executable condition file | Practical objective |
|---|---|
| spot-market-buy-sell.conditions.v1.json | Buy at the scripted price, inspect cash/owned quantity, then sell |
| spot-gtc-price-wait.conditions.v1.json | Wait for an eligible scripted price with a limit order |
| spot-ioc-fok-immediate.conditions.v1.json | Immediate fill/cancel policies; partial-fill differences deferred |
| spot-stop-limit-trigger.conditions.v1.json | Distinguish trigger activation from limit-price eligibility |

These versioned condition files are embedded and executed by the backend.
Buys use available cash and sells use owned, unreserved quantity. Reserve funds
or assets for pending orders to prevent double spending. No fees, slippage, or
market-depth caps apply. At the deadline, cancel pending orders and close remaining
holdings at the scripted ending price, retaining the report and learning objectives.
