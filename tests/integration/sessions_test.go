//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Santosh27927/Backend-COT/internal/authentication"
	"github.com/Santosh27927/Backend-COT/internal/httpapi"
	"github.com/Santosh27927/Backend-COT/internal/scenarios"
	"github.com/Santosh27927/Backend-COT/internal/sessions"
	"github.com/Santosh27927/Backend-COT/migrations"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func learningStore(t *testing.T) (*sessions.Store, *pgxpool.Pool) {
	t.Helper()
	pool := isolatedDatabase(t)
	known, err := migrations.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if err = migrations.Apply(context.Background(), pool, known); err != nil {
		t.Fatal(err)
	}
	store, err := sessions.New(pool, scenarios.Foundation())
	if err != nil {
		t.Fatal(err)
	}
	return store, pool
}
func startSession(t *testing.T, store *sessions.Store, player, key string) sessions.View {
	t.Helper()
	v, replay, err := store.Start(context.Background(), player, key, sessions.StartCommand{ScenarioID: "spot-basics"})
	if err != nil || replay {
		t.Fatalf("start failed or unexpectedly replayed: %v", err)
	}
	return v
}
func makeDue(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	// Move only an isolated test session's schedule into the past, avoiding a
	// three-minute sleep while exercising database-authoritative expiry.
	if _, err := pool.Exec(context.Background(), "UPDATE game.sessions SET started_at=clock_timestamp()-interval '4 minutes',expires_at=clock_timestamp()-interval '1 minute' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
}
func rowCount(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestConcurrentStartAllocatesOnceAndScopesKeysToPlayer(t *testing.T) {
	store, pool := learningStore(t)
	const n = 12
	var wg sync.WaitGroup
	results := make(chan sessions.View, n)
	failures := make(chan error, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, _, err := store.Start(context.Background(), "alice", "start-once-001", sessions.StartCommand{ScenarioID: "spot-basics"})
			if err != nil {
				failures <- err
				return
			}
			results <- result
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	id := ""
	for result := range results {
		if id == "" {
			id = result.Session.ID
		}
		if result.Session.ID != id || result.Account.BalanceMinor != "100000" {
			t.Fatal("retry changed session or allocation")
		}
	}
	if rowCount(t, pool, "SELECT count(*) FROM game.sessions") != 1 || rowCount(t, pool, "SELECT count(*) FROM trading.journals") != 1 || rowCount(t, pool, "SELECT count(*) FROM trading.ledger_entries") != 2 {
		t.Fatal("concurrent retry duplicated durable records")
	}
	if _, _, err := store.Start(context.Background(), "alice", "different-key", sessions.StartCommand{ScenarioID: "spot-basics"}); !errors.Is(err, sessions.ErrActive) {
		t.Fatalf("expected active-session conflict: %v", err)
	}
	if _, _, err := store.Start(context.Background(), "alice", "start-once-001", sessions.StartCommand{RestartSessionID: id}); !errors.Is(err, sessions.ErrKeyConflict) {
		t.Fatalf("expected payload/key conflict: %v", err)
	}
	other := startSession(t, store, "bob", "start-once-001")
	if other.Session.ID == id {
		t.Fatal("different players shared an account")
	}
	// Distinct keys arriving together also admit only one active allocation.
	var winners int
	var lock sync.Mutex
	for _, key := range []string{"new-key-001", "new-key-002", "new-key-003", "new-key-004"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, _, err := store.Start(context.Background(), "charlie", key, sessions.StartCommand{ScenarioID: "spot-basics"})
			if err == nil {
				lock.Lock()
				winners++
				lock.Unlock()
			} else if !errors.Is(err, sessions.ErrActive) {
				t.Error(err)
			}
		}(key)
	}
	wg.Wait()
	if winners != 1 || rowCount(t, pool, "SELECT count(*) FROM trading.accounts WHERE player_id='charlie'") != 1 {
		t.Fatal("distinct concurrent starts created extra accounts")
	}
}

func TestExpiryRestartHistoryAndVersioning(t *testing.T) {
	store, pool := learningStore(t)
	first := startSession(t, store, "alice", "first-session")
	if first.Session.ExpiresAt.Sub(first.Session.StartedAt) != 180*time.Second || first.Session.Scenario.ExecutionEnabled {
		t.Fatal("incorrect foundation scenario")
	}
	makeDue(t, pool, first.Session.ID)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.CompleteDue(context.Background(), 50); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	finished, err := store.Get(context.Background(), "alice", first.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Session.State != "completed" || finished.Session.Report == nil || finished.Session.Report.Reason != "time_elapsed" || finished.Session.Report.FinalAccount.BalanceMinor != "100000" || !finished.Session.Report.EffectiveAt.Equal(finished.Session.ExpiresAt) {
		t.Fatal("missing durable final report")
	}
	if rowCount(t, pool, "SELECT count(*) FROM game.session_events WHERE session_id=$1 AND kind='SESSION_COMPLETED'", first.Session.ID) != 1 {
		t.Fatal("workers duplicated completion")
	}
	second, replay, err := store.Start(context.Background(), "alice", "restart-first", sessions.StartCommand{RestartSessionID: first.Session.ID})
	if err != nil || replay {
		t.Fatal(err)
	}
	third, _, err := store.Start(context.Background(), "alice", "restart-active", sessions.StartCommand{RestartSessionID: second.Session.ID})
	if err != nil {
		t.Fatal(err)
	}
	if third.Session.ID == second.Session.ID || third.Account.ID == second.Account.ID || third.Account.BalanceMinor != "100000" {
		t.Fatal("restart did not create a fresh account")
	}
	old, err := store.Get(context.Background(), "alice", second.Session.ID)
	if err != nil || old.Session.Report == nil || old.Session.Report.Reason != "restarted" {
		t.Fatal("restarted session history lost")
	}
	if _, _, err = store.Start(context.Background(), "alice", "stale-restart", sessions.StartCommand{RestartSessionID: first.Session.ID}); !errors.Is(err, sessions.ErrActive) {
		t.Fatal("stale restart replaced a newer active session")
	}
	// A retry replays its original receipt; the client then fetches current state.
	original, replayed, err := store.Start(context.Background(), "alice", "first-session", sessions.StartCommand{ScenarioID: "spot-basics"})
	if err != nil || !replayed || original.Session.ID != first.Session.ID {
		t.Fatal("historical receipt was not retained")
	}
	changed := scenarios.Foundation()
	changed.Title = "Changed without a version bump"
	changedStore, err := sessions.New(pool, changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = changedStore.Start(context.Background(), "charlie", "changed-rules", sessions.StartCommand{ScenarioID: "spot-basics"}); !errors.Is(err, sessions.ErrVersionConflict) {
		t.Fatal("immutable scenario version accepted changed rules")
	}
	if rowCount(t, pool, "SELECT count(*) FROM game.players WHERE id='charlie'") != 0 {
		t.Fatal("failed command leaked a partial player")
	}
	if rowCount(t, pool, "SELECT count(*) FROM game.sessions WHERE player_id='alice'") != 3 {
		t.Fatal("history removed or duplicated")
	}
}

func TestFailedAllocationRollsBackEntireCommand(t *testing.T) {
	store, pool := learningStore(t)
	_, err := pool.Exec(context.Background(), `CREATE FUNCTION trading.fail_test_allocation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test allocation failure'; END; $$; CREATE TRIGGER fail_test BEFORE INSERT ON trading.ledger_entries FOR EACH ROW EXECUTE FUNCTION trading.fail_test_allocation()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.Start(context.Background(), "alice", "retry-after-failure", sessions.StartCommand{ScenarioID: "spot-basics"}); err == nil {
		t.Fatal("injected allocation failure was ignored")
	}
	for _, query := range []string{"SELECT count(*) FROM game.players", "SELECT count(*) FROM game.sessions", "SELECT count(*) FROM trading.accounts", "SELECT count(*) FROM trading.journals", "SELECT count(*) FROM game.command_receipts"} {
		if rowCount(t, pool, query) != 0 {
			t.Fatal("failed allocation committed partial state")
		}
	}
	if _, err = pool.Exec(context.Background(), "DROP TRIGGER fail_test ON trading.ledger_entries"); err != nil {
		t.Fatal(err)
	}
	startSession(t, store, "alice", "retry-after-failure")
	var firstID string
	if err = pool.QueryRow(context.Background(), "SELECT id::text FROM game.sessions WHERE player_id='alice'").Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(context.Background(), "CREATE TRIGGER fail_test BEFORE INSERT ON trading.ledger_entries FOR EACH ROW EXECUTE FUNCTION trading.fail_test_allocation()"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.Start(context.Background(), "alice", "failed-restart", sessions.StartCommand{RestartSessionID: firstID}); err == nil {
		t.Fatal("failed restart unexpectedly committed")
	}
	retained, err := store.Get(context.Background(), "alice", firstID)
	if err != nil || retained.Session.State != "active" || retained.Account.BalanceMinor != "100000" {
		t.Fatal("failed restart changed the prior session/account")
	}
}

func TestLedgerAndHistoryDatabaseInvariants(t *testing.T) {
	store, pool := learningStore(t)
	first := startSession(t, store, "alice", "ledger-invariants")
	for _, query := range []string{"UPDATE trading.ledger_entries SET amount_minor=1", "DELETE FROM trading.journals", "DELETE FROM game.session_events", "UPDATE game.scenario_versions SET checksum='changed'"} {
		if _, err := pool.Exec(context.Background(), query); err == nil {
			t.Fatal("immutable history was modified")
		}
	}
	makeDue(t, pool, first.Session.ID)
	if _, err := store.Get(context.Background(), "alice", first.Session.ID); err != nil {
		t.Fatal(err)
	}
	for _, entries := range []string{"", ",($1,'demo_source',-99999)"} {
		tx, err := pool.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(context.Background(), `INSERT INTO game.sessions(id,player_id,scenario_id,scenario_version,state,started_at,expires_at) VALUES ('00000000-0000-4000-8000-000000000001','alice','spot-basics',1,'active',now(),now()+interval '3 minutes'); INSERT INTO trading.accounts(id,session_id,player_id,currency,created_at) VALUES ('00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000001','alice','USD',now()); INSERT INTO trading.journals(id,account_id,kind,effective_at) VALUES ('00000000-0000-4000-8000-000000000003','00000000-0000-4000-8000-000000000002','DEMO_ALLOCATION',now())`)
		if err != nil {
			_ = tx.Rollback(context.Background())
			t.Fatal(err)
		}
		// One entry or an unbalanced pair must fail at transaction commit.
		query := "INSERT INTO trading.ledger_entries(journal_id,bucket,amount_minor) VALUES ($1,'player_cash',100000)" + entries
		if _, err = tx.Exec(context.Background(), query, "00000000-0000-4000-8000-000000000003"); err != nil {
			_ = tx.Rollback(context.Background())
			t.Fatal(err)
		}
		if err = tx.Commit(context.Background()); err == nil {
			t.Fatal("incomplete or unbalanced journal committed")
		}
		var invariant *pgconn.PgError
		if !errors.As(err, &invariant) || invariant.Code != "23514" {
			t.Fatal("journal failed for a reason other than its balance constraint")
		}
	}
}

func TestLearningHTTPIdentityAndReconnectContract(t *testing.T) {
	store, _ := learningStore(t)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := func(player string) http.Handler {
		a := authentication.NewDevelopment("local-test-token", player)
		return httpapi.Handler("game-service", logger, a, func(context.Context) (bool, string) { return true, "ready" }, httpapi.LearningRoutes(a, store, logger))
	}
	call := func(h http.Handler, method, path, body, key string, auth bool, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		if auth {
			req.Header.Set("Authorization", "Bearer local-test-token")
		}
		req.Header.Set("X-Player-ID", "bob")
		out := httptest.NewRecorder()
		h.ServeHTTP(out, req)
		if out.Code != want {
			t.Fatalf("%s %s status=%d want=%d body=%s", method, path, out.Code, want, out.Body.String())
		}
		return out
	}
	alice, bob := handler("alice"), handler("bob")
	call(alice, "POST", "/v1/sessions", `{"scenarioId":"spot-basics"}`, "http-start-key", false, 401)
	response := call(alice, "POST", "/v1/sessions", `{"scenarioId":"spot-basics"}`, "http-start-key", true, 201)
	var body struct {
		Data sessions.View `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	id := body.Data.Session.ID
	if body.Data.Session.PlayerID != "alice" || body.Data.Account.BalanceMinor != "100000" {
		t.Fatal("caller overrode identity or initial funds")
	}
	replayed := call(alice, "POST", "/v1/sessions", `{"scenarioId":"spot-basics"}`, "http-start-key", true, 201)
	if replayed.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatal("replay was not identified")
	}
	for _, suffix := range []string{"", "/account", "/ledger", "/events"} {
		call(bob, "GET", "/v1/sessions/"+id+suffix, "", "", true, 404)
	}
	call(bob, "POST", "/v1/sessions/"+id+"/restart", `{}`, "bob-restart-key", true, 404)
	call(alice, "POST", "/v1/sessions", `{"scenarioId":"spot-basics","playerId":"bob"}`, "unknown-field", true, 400)
	call(alice, "POST", "/v1/sessions", `{"scenarioId":"spot-basics"} {}`, "multiple-json", true, 400)
	call(alice, "POST", "/v1/sessions", "null", "null-object", true, 400)
	call(alice, "POST", "/v1/sessions/"+id+"/restart", "null", "null-restart", true, 400)
	call(alice, "POST", "/v1/sessions", `{"scenarioId":"spot-basics"}`, "", true, 400)
	call(alice, "GET", "/v1/sessions/"+id+"/events?after=-1", "", "", true, 400)
	call(alice, "GET", "/v1/sessions/"+id+"/events?limit=101", "", "", true, 400)
	call(alice, "GET", "/v1/sessions/"+id+"/events?limit=1&limit=2", "", "", true, 400)
	page := call(alice, "GET", "/v1/sessions/"+id+"/events?after=1&limit=1", "", "", true, 200)
	var events struct {
		Events []sessions.Event `json:"events"`
		Next   int64            `json:"nextAfter"`
	}
	if err := json.Unmarshal(page.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}
	if len(events.Events) != 1 || events.Events[0].Kind != "DEMO_FUNDS_ALLOCATED" || events.Next != 2 {
		t.Fatal("event cursor skipped or duplicated history")
	}
	call(alice, "GET", "/v1/sessions/"+id+"/ledger", "", "", true, 200)
}
