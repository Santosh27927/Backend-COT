GO ?= go
export GOTOOLCHAIN := local
export GOCACHE ?= $(abspath ..)/.cache/go-build
export GOMODCACHE ?= $(abspath ..)/.cache/go-modules

.PHONY: init-local db-up db-stop migrate migration-status run-game run-trading build fmt fmt-check vet test test-race test-integration test-smoke check
init-local:
	./scripts/init-local.sh

db-up:
	docker compose --env-file .env -f dev/compose.yaml up -d --wait --wait-timeout 90

db-stop:
	docker compose --env-file .env -f dev/compose.yaml stop

migrate:
	./scripts/with-env.sh $(GO) run ./cmd/migrate -action up

migration-status:
	./scripts/with-env.sh $(GO) run ./cmd/migrate -action status

run-game:
	./scripts/with-env.sh $(GO) run ./cmd/game-service

run-trading:
	./scripts/with-env.sh $(GO) run ./cmd/trading-service

build:
	$(GO) build -trimpath -o bin/game-service ./cmd/game-service
	$(GO) build -trimpath -o bin/trading-service ./cmd/trading-service
	$(GO) build -trimpath -o bin/migrate ./cmd/migrate

fmt:
	gofmt -w cmd internal migrations tests simulation

fmt-check:
	@test -z "$$(gofmt -l cmd internal migrations tests simulation)" || (echo "Run make fmt"; exit 1)

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-integration:
	./scripts/with-env.sh sh -c 'COT_TEST_DATABASE_URL="$$COT_DATABASE_URL" $(GO) test -race -tags=integration ./tests/integration -count=1'

test-smoke: build
	./scripts/with-env.sh sh -c 'COT_TEST_DATABASE_URL="$$COT_DATABASE_URL" $(GO) test -tags=smoke ./tests/smoke -count=1'

check: fmt-check vet test
