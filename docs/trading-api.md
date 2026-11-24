# Scripted trading API

Run `make db-up migrate` and `make run-game`. Learning/trading routes use the existing
local bearer token and game-service on port 8080. trading-service retains foundation endpoints.

## Journey

1. `GET /v1/scenarios` lists lesson prerequisites; `GET /v1/progress` restores the player's completed practical lessons.
2. `POST /v1/sessions` with `{"scenarioId":"spot-market-buy-sell"}` starts the first executable lesson.
3. Send order commands to `POST /v1/sessions/{id}/orders`.
4. `GET /v1/sessions/{id}` returns the current trading snapshot and server time.
5. `POST /v1/sessions/{id}/finish` with `{}` explicitly settles early, or let the backend deadline expire.
6. Read the report and `/v1/sessions/{id}/events?after=...` on reconnect.
7. Start the next lesson after its practical prerequisites have passed.

Every mutating request needs a unique `Idempotency-Key` of 8-128 supported ASCII characters.
Retry the same command with the same key after a lost response. A replay returns its original
snapshot; GET the session for current state. Validation failures do not create accepted orders.

## Order examples

Market spot buy:

```json
{"product":"spot","symbol":"SOL","side":"BUY","type":"MARKET","quantity":"1"}
```

Waiting spot limit:

```json
{"product":"spot","symbol":"XRP","side":"BUY","type":"LIMIT","quantity":"1","limitPrice":"1.8","timeInForce":"GTC"}
```

Stop-limit activation:

```json
{"product":"spot","symbol":"XRP","side":"BUY","type":"STOP_LIMIT","quantity":"1","stopPrice":"2.1","limitPrice":"2.2","timeInForce":"GTC"}
```

Isolated perpetual open, in an unlocked lesson permitting 2x:

```json
{"product":"perpetual","symbol":"SOL","side":"BUY","type":"MARKET","quantity":"1","leverage":2}
```

Explicit close of a long, without accidental reversal:

```json
{"product":"perpetual","symbol":"SOL","side":"SELL","type":"MARKET","quantity":"1","reduceOnly":true}
```

For shorts, opening is SELL and reducing/closing is BUY. One net position exists per asset;
leverage/direction cannot silently change while it remains open. Cancel a pending order with
`POST /v1/sessions/{id}/orders/{orderId}/cancel`, body `{}`.

## Interpretation

- All prices are authored and hypothetical. `mode` is `scripted`.
- Cash quantities use twelve decimal places internally; asset quantities/prices use six.
  JSON financial values are strings. Legacy preparation accounts keep scale 2.
- Perpetual notional is exposure, not a wallet deposit or spot ownership.
- Available cash subtracts committed isolated margin and pending cash reservations.
- Stop-limit currently uses GTC. Market orders use IOC. Immediate eligible affordable orders
  fill fully; partial IOC versus FOK behavior remains deferred.
- A 1% maintenance threshold and explicit isolated-loss cap define the training risk model.
  A gap beyond collateral is reported as `trainingBackstop`; it is not an exchange guarantee.
- Average entry price is a display rounded to price precision. Entry notional and PnL use
  exact stored basis, with proportional basis/margin and remainder preservation on partial close.
- All trading/funding charges and slippage are zero. No order-book quantity caps apply.
- The per-session user order cap of 256 bounds prototype resource use; it is not simulated liquidity.
- Invalid commands are rejected; expired sessions admit no new orders. At expiry, pending
  orders cancel before endpoint conditional fills. Existing exposure settles at the ending price.
- Future scripted price frames are private. The API exposes current prices and recorded outcomes only.

The [HTTP contract](../contracts/http/learning.openapi.json) describes these routes.
