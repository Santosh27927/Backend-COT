# Paper-trading game backend architecture

Status: planning baseline  
Updated: 2026-11-23

## Confirmed decisions

- Use paper trading with virtual funds.
- Reset funds for each new scenario.
- Begin with one human and an automated opponent.
- Support two human participants later.
- Develop locally before choosing cloud infrastructure.
- Use Phoenix Rise SDK as the selected protocol integration.
- Contract selection and account-model scope remain undecided.
- One engineering contributor will learn and implement the stack.

## Technology baseline

- TypeScript and NestJS.
- PostgreSQL.
- Explicit decimal and integer-unit arithmetic.
- Versioned application contracts.
- Durable outcomes and idempotent processing.

Use the TypeScript Rise package: `@ellipsis-labs/rise`.

Pin and validate an exact SDK version during implementation.

## Logical runtimes

Game service:

- Village, lessons, progression, challenges, clans, and rewards.

Trading service:

- Paper accounts, orders, execution, positions, margin, funding,
  accounting, and scenario settlement.

Market-data service:

- Phoenix integration, metadata, market streams, normalization,
  freshness monitoring, and recording.

These are separate runtime responsibilities.
For early testing they may run as separate processes or containers
on one development computer.

## Structure

```text
backend/
├── runtimes/
│   ├── game-service/
│   ├── trading-service/
│   └── market-data-service/
├── domains/
│   ├── village/
│   ├── learning/
│   ├── challenges/
│   ├── opponents/
│   ├── clans/
│   ├── rewards/
│   └── trading/
├── integrations/
│   └── phoenix/
│       ├── client/
│       ├── metadata/
│       ├── streams/
│       ├── normalization/
│       └── compatibility-tests/
├── platform/
│   ├── authentication/
│   ├── database/
│   ├── messaging/
│   ├── configuration/
│   └── observability/
├── contracts/
│   ├── http/
│   ├── events/
│   └── configuration/
├── content/
│   ├── lessons/
│   ├── scenarios/
│   └── rule-profiles/
└── tests/
```

## Phoenix integration boundary

Keep SDK-specific objects and behavior inside the Phoenix integration.

Expose application-owned interfaces for:

- Market metadata.
- Prices and order-book updates.
- Supported capabilities.
- Applicable calculation inputs.

Use the SDK's documented state/cache primitives where applicable.
Share subscriptions and reconcile after reconnects or sequence gaps.

Translate SDK values into explicit application units.
Preserve integer precision, tick sizes, quantity increments, and rounding.

The mobile client does not call privileged protocol operations directly.

## Paper execution boundary

The initial execution authority is our paper-trading backend.

A paper order updates our virtual account and ledger.
It does not submit a Phoenix transaction.

Initial integration can use:

- Recorded scenario data.
- Phoenix market metadata and read-only market data.
- SDK calculation helpers where suitable and independently validated.

Transaction signing, collateral deposits, and real execution are outside
the initial paper-trading scope.

The reviewed SDK documentation does not establish a turnkey hosted
paper-trading environment for this game. That capability remains
unverified.

## Protocol fidelity

Create a capability matrix before implementing each Phoenix-specific lesson.

Record:

- Protocol and SDK versions.
- Supported market and account model.
- Order constraints and expiry semantics.
- Trigger rules.
- Fees, funding, and risk calculations.
- Known simulation approximations.

Do not copy Binance or Bybit behavior into a Phoenix rule profile.

Phoenix-specific facts to preserve:

- Funding settles hourly.
- An eight-hour quoted rate is distinct from its settlement interval.
- The documented TP/SL controls use mark-price triggers.
- Order packets support slot-based expiry.
- Cross and isolated accounts have protocol-specific representations.

Detailed contract, collateral, leverage, and account behavior still
require specification and validation.

## Automated opponent

Implement a deterministic policy-based opponent first.

The opponent:

- Receives only information available at the current scenario time.
- Uses the same command validation as a human.
- Has its own scenario account.
- Has no access to future replay events.
- Records its actions for diagnosis.

Use configurable policies and seeds.

The opponent is separate from simulated order-book liquidity.
A bot action does not guarantee a human order's execution.

## Session and reset model

Each new scenario creates a new session with:

- Participant identities.
- Fresh virtual account allocations.
- Scenario and rule versions.
- Opponent policy and seed.
- A server-controlled clock.

Use a new account/session allocation rather than deleting historical
financial records.

Keep learning progress and village progression separate from practice
fund resets.

## Trading consistency

Orders, positions, margin, funding, and accounting remain within one
coordinated transaction boundary.

Commit the relevant state changes, ledger entries, and outgoing events
atomically.

Use stable command identifiers and account-scoped concurrency control.
Workers follow the same write rules.

Other domains cannot directly update trading balances.

## Learning and reward delivery

Record structured execution reasons for explanations.

Deliver completed outcomes through a durable outbox.
Consumers tolerate duplicate events.

A delayed reward remains recoverable without reversing a committed
trading result.

## Runtime configuration

Separate:

- Deployment settings.
- Secrets.
- Operational flags.
- Scenario and trading-rule versions.
- Player/account settings.

Each domain owns its configuration schema.

Controlled sessions pin their scenario, rule, and engine versions.
New sessions can adopt newly published versions.

Operational emergency controls have separate, explicit semantics.

## Future human multiplayer

Preserve the same session and command model for humans and bots.

Two-human support adds authorization, shared lifecycle transitions,
disconnect handling, deadlines, fairness checks, and synchronization.

Begin with direct room joining before public matchmaking.

## Limitations and open decisions

- Exact first playable release scope is undecided.
- Initial Phoenix markets, contract family, collateral, and account model
  remain undecided.
- SDK compatibility and supported API access must be verified.
- Live market data requires internet connectivity.
- Public data cannot guarantee identical production fills.
- A paper account is not an on-chain Phoenix trader account.
- Local SDK margin estimates depend on current, complete inputs and
  validated assumptions.
- Phoenix LiteSVM tests require protocol artifacts and fixture setup;
  they are a separate integration-testing option, not the mobile backend.
- A shared development host or database remains a shared failure dependency.
- No implementation or performance validation has been completed.

## Validation priorities

- Independent expected results for each supported rule.
- Precision and unit conversion.
- Order lifecycle and partial execution.
- Concurrency, duplicate requests, and funding boundaries.
- Ledger reconciliation.
- Deterministic bot behavior.
- Scenario reset without loss of history.
- Stream freshness and reconnect recovery.
- Isolation between paper commands and real transaction submission.

## References

- [Rise SDK](https://docs.phoenix.trade/sdk/rise)
- [Accounts](https://docs.phoenix.trade/sdk/accounts)
- [Orders](https://docs.phoenix.trade/sdk/orders)
- [Funding](https://docs.phoenix.trade/phoenix/margin-and-risk/funding-rate)
- [Order types](https://docs.phoenix.trade/phoenix/matching-engine/order-types)
- [Streaming practices](https://docs.phoenix.trade/sdk/ws-api-best-practices)
- [Margin calculations](https://docs.phoenix.trade/sdk/margin)
- [LiteSVM testing](https://docs.phoenix.trade/sdk/litesvm-testing)
