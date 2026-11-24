# Phase-one validation

Validated locally on 2026-11-24 with Go 1.27.4 and PostgreSQL 17.

## Passed

- `make check`: formatting, go vet, and unit tests.
- `make build`: game, trading, and migration executables.
- `make migrate` and `make migration-status`: local schema ready.
- `make test-race`: race detector for default packages.
- `make test-integration`: real PostgreSQL lifecycle, migration idempotency,
  concurrent migration runners, rollback, drift detection, and readiness transitions.
- `make test-smoke`: both built services, identity authentication, response
  references, error statuses, safe logs, invalid configuration, database outage,
  and graceful process shutdown including incomplete request headers.

A process smoke check initially exposed a shutdown deadline that was too close
to the header-read deadline. The shutdown allowance is now 10 seconds and the
incomplete-header regression check passes.

- `docker build --build-arg SERVICE=game-service -t cot-backend/game-service:phase-one .`: passed.
- The built image declares the non-root `cot` user.
- Private `.env` permissions are 0600 and credentials/binaries are gitignored.
- Final fixture inspection found zero leftover `cot_test_*` databases.

The container build does not constitute a cloud deployment. The game image was
built; both service executables and the migration executable were built natively.

## Test isolation

Integration tests create randomly named `cot_test_*` databases and remove only
their own databases. Process tests choose temporary loopback ports and signal
only the child processes they started.

The local COT PostgreSQL fixture retains its data volume. Test service processes
stop after verification. No frontend, Phoenix, load, or production security
certification is implied by these checks.
