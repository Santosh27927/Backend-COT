//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Santosh27927/Backend-COT/internal/authentication"
	"github.com/Santosh27927/Backend-COT/internal/httpapi"
	"github.com/Santosh27927/Backend-COT/internal/players"
	"github.com/Santosh27927/Backend-COT/internal/scenarios"
	"github.com/Santosh27927/Backend-COT/internal/sessions"
	"github.com/Santosh27927/Backend-COT/internal/trading/execution"
	"github.com/Santosh27927/Backend-COT/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestExecutableHTTPContract(t *testing.T) {
	store, _ := executableStore(t)
	a := authentication.NewDevelopment("http-trading-test", "alice")
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := httpapi.Handler("game-service", logger, a, func(context.Context) (bool, string) { return true, "ready" }, httpapi.LearningRoutes(a, store, logger))
	call := func(path, body, key string, want int) sessions.View {
		t.Helper()
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer http-trading-test")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", key)
		out := httptest.NewRecorder()
		h.ServeHTTP(out, r)
		if out.Code != want {
			t.Fatalf("%s status=%d body=%s", path, out.Code, out.Body.String())
		}
		var reply struct {
			Data sessions.View `json:"data"`
		}
		if err := json.Unmarshal(out.Body.Bytes(), &reply); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.Body.String(), "privateTraining") || strings.Contains(out.Body.String(), "offsetMs\":30000,\"prices") {
			t.Fatal("future script leaked")
		}
		return reply.Data
	}
	v := call("/v1/sessions", `{"scenarioId":"spot-market-buy-sell"}`, "http-script-start", 201)
	filled := call("/v1/sessions/"+v.Session.ID+"/orders", `{"product":"spot","symbol":"SOL","side":"BUY","type":"MARKET","quantity":"1"}`, "http-script-buy", 200)
	if filled.Trading == nil || filled.Trading.Cash != "900.000000000000" {
		t.Fatal("HTTP spot trade failed")
	}
	call("/v1/sessions/"+v.Session.ID+"/finish", `{}`, "http-script-finish", 200)
	call("/v1/sessions/"+v.Session.ID+"/orders", `{"product":"spot","symbol":"SOL","side":"BUY","type":"MARKET","quantity":"1"}`, "http-script-late", 409)
}

func executableStore(t *testing.T) (*sessions.Store, *pgxpool.Pool) {
	t.Helper()
	pool := isolatedDatabase(t)
	known, err := migrations.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if err = migrations.Apply(context.Background(), pool, known); err != nil {
		t.Fatal(err)
	}
	catalog, err := scenarios.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	store, err := sessions.NewCatalog(pool, catalog)
	if err != nil {
		t.Fatal(err)
	}
	return store, pool
}
func startExecutable(t *testing.T, s *sessions.Store, player, id, key string) sessions.View {
	t.Helper()
	v, _, err := s.Start(context.Background(), player, key, sessions.StartCommand{ScenarioID: id})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func trade(t *testing.T, s *sessions.Store, player, id, key string, c execution.Command) sessions.View {
	t.Helper()
	v, _, err := s.Trade(context.Background(), player, key, id, c)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func market(side, qty string) execution.Command {
	return execution.Command{Product: "spot", Symbol: "XRP", Side: side, Type: "MARKET", Quantity: qty}
}
func TestExecutableSpotRetriesCutoffAndProgression(t *testing.T) {
	s, pool := executableStore(t)
	v := startExecutable(t, s, "alice", "spot-market-buy-sell", "new-spot-lesson")
	if v.Account.Scale != 12 || v.Trading == nil || v.Trading.AvailableCash != "1000.000000000000" {
		t.Fatal("incorrect executable funding")
	}
	filled := trade(t, s, "alice", v.Session.ID, "buy-entire-wallet", market("BUY", "500"))
	if filled.Trading.Cash != "0.000000000000" {
		t.Fatal("wallet not consumed exactly")
	}
	repeated, replay, err := s.Trade(context.Background(), "alice", "buy-entire-wallet", v.Session.ID, market("BUY", "500"))
	if err != nil || !replay || len(repeated.Trading.Fills) != 1 {
		t.Fatal("trade retry duplicated effects")
	}
	if _, _, err = s.Trade(context.Background(), "bob", "unauthorized-buy", v.Session.ID, market("BUY", "1")); !errors.Is(err, sessions.ErrNotFound) {
		t.Fatal("ownership bypass")
	}
	if _, _, err = s.Start(context.Background(), "alice", "premature-lesson", sessions.StartCommand{ScenarioID: "spot-gtc-price-wait"}); !errors.Is(err, sessions.ErrLocked) {
		t.Fatal("prerequisite bypass")
	}
	trade(t, s, "alice", v.Session.ID, "sell-some-asset", market("SELL", "100"))
	makeDue(t, pool, v.Session.ID)
	if _, _, err = s.Trade(context.Background(), "alice", "too-late-order", v.Session.ID, market("SELL", "1")); !errors.Is(err, execution.ErrClosed) {
		t.Fatal("post-deadline order accepted")
	}
	done, err := s.Get(context.Background(), "alice", v.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Session.Report == nil || !done.Session.Report.LearningPassed || done.Trading.Cash != "1020.000000000000" || done.Trading.Holdings[0].Quantity != "0.000000" {
		t.Fatal("settlement or objectives incorrect")
	}
	if rowCount(t, pool, "SELECT count(*) FROM game.lesson_progress WHERE player_id='alice'") != 1 {
		t.Fatal("progress missing")
	}
	owned, err := s.Progress(context.Background(), "alice")
	if err != nil || len(owned) != 1 {
		t.Fatal("owned progress missing")
	}
	other, err := s.Progress(context.Background(), "bob")
	if err != nil || len(other) != 0 {
		t.Fatal("another player observed progress")
	}
	next := startExecutable(t, s, "alice", "spot-gtc-price-wait", "next-spot-lesson")
	if next.Account.BalanceMinor != "1000000000000000" {
		t.Fatal("new lesson not funded freshly")
	}
}
func TestConcurrentSpotOrdersCannotOverspendOrOversell(t *testing.T) {
	s, pool := executableStore(t)
	v := startExecutable(t, s, "alice", "spot-market-buy-sell", "concurrent-wallet")
	var wg sync.WaitGroup
	var lock sync.Mutex
	winners := 0
	for _, key := range []string{"parallel-buy-001", "parallel-buy-002"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, _, err := s.Trade(context.Background(), "alice", key, v.Session.ID, market("BUY", "500"))
			if err == nil {
				lock.Lock()
				winners++
				lock.Unlock()
			} else if !errors.Is(err, execution.ErrFunds) {
				t.Error(err)
			}
		}(key)
	}
	wg.Wait()
	if winners != 1 {
		t.Fatal("concurrent overspend")
	}
	winners = 0
	for _, key := range []string{"parallel-sell-001", "parallel-sell-002"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, _, err := s.Trade(context.Background(), "alice", key, v.Session.ID, market("SELL", "500"))
			if err == nil {
				lock.Lock()
				winners++
				lock.Unlock()
			} else if !errors.Is(err, execution.ErrFunds) {
				t.Error(err)
			}
		}(key)
	}
	wg.Wait()
	if winners != 1 {
		t.Fatal("concurrent oversell")
	}
	if rowCount(t, pool, "SELECT count(*) FROM trading.journals") != 5 {
		t.Fatal("financial journals duplicated or missing")
	}
}
func TestPendingOrdersRecoverAtHistoricalFrame(t *testing.T) {
	s, pool := executableStore(t)
	// Progress is generated by real actions before enabling the next lesson.
	first := startExecutable(t, s, "alice", "spot-market-buy-sell", "precondition-run")
	trade(t, s, "alice", first.Session.ID, "precondition-buy", market("BUY", "1"))
	trade(t, s, "alice", first.Session.ID, "precondition-sell", market("SELL", "1"))
	trade(t, s, "alice", first.Session.ID, "precondition-finish", execution.Command{Action: "finish"})
	v := startExecutable(t, s, "alice", "spot-gtc-price-wait", "pending-limit-run")
	c := market("BUY", "500")
	c.Type = "LIMIT"
	c.TimeInForce = "GTC"
	c.LimitPrice = "1.8"
	trade(t, s, "alice", v.Session.ID, "pending-limit-command", c)
	if _, err := pool.Exec(context.Background(), "UPDATE game.sessions SET started_at=clock_timestamp()-interval '130 seconds',expires_at=clock_timestamp()+interval '50 seconds' WHERE id=$1", v.Session.ID); err != nil {
		t.Fatal(err)
	}
	catalog, _ := scenarios.Catalog()
	recovered, err := sessions.NewCatalog(pool, catalog)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err = recovered.CompleteDue(ctx, 50); err != nil {
		t.Fatal(err)
	}
	current, err := recovered.Get(ctx, "alice", v.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Trading.Fills) != 1 || current.Trading.Fills[0].OffsetMS != 90000 || current.Trading.Fills[0].Price != "1.800000" {
		t.Fatal("recovery fabricated a latest-price fill")
	}
}

func isolatedExecutable(t *testing.T, id string) (*sessions.Store, *pgxpool.Pool) {
	t.Helper()
	_, pool := executableStore(t)
	catalog, _ := scenarios.Catalog()
	for _, d := range catalog {
		if d.ID == id {
			d.Training.Requires = nil
			store, err := sessions.New(pool, d)
			if err != nil {
				t.Fatal(err)
			}
			return store, pool
		}
	}
	t.Fatal("missing test profile")
	return nil, nil
}
func TestPerpetualLiquidationRecoveryAndIdempotency(t *testing.T) {
	s, pool := isolatedExecutable(t, "perpetual-isolated-liquidation-5x")
	v := startExecutable(t, s, "alice", "perpetual-isolated-liquidation-5x", "perp-funding-key")
	command := execution.Command{Product: "perpetual", Symbol: "SOL", Side: "BUY", Type: "MARKET", Quantity: "10", Leverage: 5}
	opened := trade(t, s, "alice", v.Session.ID, "perp-open-key", command)
	if opened.Trading.AvailableCash != "800.000000000000" || opened.Trading.Cash != "1000.000000000000" {
		t.Fatal("collateral/notional mismatch")
	}
	_, replay, err := s.Trade(context.Background(), "alice", "perp-open-key", v.Session.ID, command)
	if err != nil || !replay {
		t.Fatal("perpetual order receipt missing")
	}
	makeDue(t, pool, v.Session.ID)
	if _, err = s.CompleteDue(context.Background(), 50); err != nil {
		t.Fatal(err)
	}
	done, err := s.Get(context.Background(), "alice", v.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(done.Trading.Positions) != 0 || done.Trading.Cash != "800.000000000000" || !done.Session.Report.LearningPassed {
		t.Fatal("perpetual recovery result wrong")
	}
	if rowCount(t, pool, "SELECT count(*) FROM trading.journals WHERE kind='PERP_PNL'") != 1 || rowCount(t, pool, "SELECT count(*) FROM game.session_events WHERE kind='POSITION_LIQUIDATED'") != 1 {
		t.Fatal("liquidation journal/event duplicated")
	}
	if _, err = s.CompleteDue(context.Background(), 50); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentSpotAndMarginCompeteForOneWallet(t *testing.T) {
	s, _ := isolatedExecutable(t, "combined-wallet-margin-accounting")
	v := startExecutable(t, s, "alice", "combined-wallet-margin-accounting", "mixed-wallet-start")
	commands := []execution.Command{market("BUY", "500"), {Product: "perpetual", Symbol: "SOL", Side: "BUY", Type: "MARKET", Quantity: "50", Leverage: 5}}
	var wg sync.WaitGroup
	var lock sync.Mutex
	winners := 0
	for i, c := range commands {
		wg.Add(1)
		go func(i int, c execution.Command) {
			defer wg.Done()
			key := []string{"mixed-spot-command", "mixed-perp-command"}[i]
			_, _, err := s.Trade(context.Background(), "alice", key, v.Session.ID, c)
			if err == nil {
				lock.Lock()
				winners++
				lock.Unlock()
			} else if !errors.Is(err, execution.ErrFunds) {
				t.Error(err)
			}
		}(i, c)
	}
	wg.Wait()
	if winners != 1 {
		t.Fatal("spot and collateral reused the same wallet")
	}
	current, err := s.Get(context.Background(), "alice", v.Session.ID)
	if err != nil || current.Trading.AvailableCash != "0.000000000000" || len(current.Trading.Fills) != 1 {
		t.Fatal("inconsistent mixed command result")
	}
}

func TestWorkerProgressDoesNotBlockOnPlayerCommandLock(t *testing.T) {
	s, pool := executableStore(t)
	v := startExecutable(t, s, "alice", "spot-market-buy-sell", "lock-order-session")
	trade(t, s, "alice", v.Session.ID, "lock-order-buy", market("BUY", "1"))
	trade(t, s, "alice", v.Session.ID, "lock-order-sell", market("SELL", "1"))
	makeDue(t, pool, v.Session.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = players.Lock(ctx, tx, "alice"); err != nil {
		t.Fatal(err)
	}
	// A worker holds the session row and needs FK key-share on the player.
	// It must finish while another command still holds the serializer lock.
	done, err := s.Get(ctx, "alice", v.Session.ID)
	if err != nil || done.Session.Report == nil || !done.Session.Report.LearningPassed {
		t.Fatal("worker progress blocked on player command lock")
	}
}
