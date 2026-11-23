# Backend-COT

Backend for a village-based learning game that teaches perpetual futures
using virtual funds and controlled trading scenarios.

## Project status

Planning and prototyping.

Initial testing will support one human participant and an automated
opponent. Two-human challenges are planned.

Practice funds reset for each new scenario. Learning and village progress
are managed separately.

## Planned technology

- TypeScript
- NestJS
- PostgreSQL
- Phoenix Rise SDK integration

## Responsibilities

This repository will contain:
- Game, trading, and market-data runtimes.
- Paper accounts, orders, execution, positions, and accounting.
- Margin, funding, and supported risk calculations.
- Lessons, challenges, automated opponents, and rewards.
- Versioned API, event, and configuration contracts.

Paper execution will update our virtual accounts without submitting real
Phoenix trading transactions.

Exact first-release scope and Phoenix contract/account behavior remain
under discussion.

## Documentation

Detailed architecture, responsibility boundaries, integration assumptions,
and known limitations will be maintained in `docs/architecture.md`.

Setup and test instructions will be added alongside implementation.

## Related repositories

- [Frontend](https://github.com/Santosh27927/Frontend-COT)
- [DevOps](https://github.com/Santosh27927/DevOps-COT)
