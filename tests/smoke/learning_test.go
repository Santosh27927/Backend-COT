//go:build smoke

package smoke

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Santosh27927/Backend-COT/internal/sessions"
	"github.com/Santosh27927/Backend-COT/internal/testsupport"
	"github.com/Santosh27927/Backend-COT/migrations"
)

type learningResponse struct {
	Data     sessions.View             `json:"data"`
	Events   []sessions.Event          `json:"events"`
	Progress []sessions.LessonProgress `json:"progress"`
}

func learningCall(t *testing.T, client *http.Client, base, method, path, body, key string, want int) (learningResponse, http.Header) {
	t.Helper()
	req, err := http.NewRequest(method, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv("COT_DEV_AUTH_TOKEN"))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal("owned learning service request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("%s %s status=%d want=%d", method, path, response.StatusCode, want)
	}
	var result learningResponse
	if err = json.NewDecoder(io.LimitReader(response.Body, 128*1024)).Decode(&result); err != nil {
		t.Fatal("invalid learning response")
	}
	return result, response.Header
}

func TestSessionSurvivesProcessCrashAndCompletesWithoutPlayer(t *testing.T) {
	requireLocal(t)
	fixture := testsupport.Postgres(t)
	known, err := migrations.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if err = migrations.Apply(context.Background(), fixture.Pool, known); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	launch := func() (*process, string) {
		bind := address(t)
		p := start(t, "game-service", map[string]string{"COT_HTTP_ADDR": bind, "COT_DATABASE_URL": fixture.URL, "COT_DEV_PLAYER_ID": "phase-two-smoke"})
		base := "http://" + bind
		awaitLiveness(t, client, base, p)
		return p, base
	}
	first, base := launch()
	created, _ := learningCall(t, client, base, "POST", "/v1/sessions", `{"scenarioId":"spot-basics"}`, "crash-retry-command", 201)
	id := created.Data.Session.ID
	if id == "" || created.Data.Account.BalanceMinor != "100000" {
		t.Fatal("initial demo allocation missing")
	}
	// Kill only our own process, simulating an abrupt backend crash.
	first.stopped = true
	if err = first.cmd.Process.Kill(); err != nil {
		t.Fatal("could not stop owned crash-test process")
	}
	select {
	case <-first.done:
	case <-time.After(3 * time.Second):
		t.Fatal("owned crash-test process did not exit")
	}
	if _, err = fixture.Pool.Exec(context.Background(), "UPDATE game.sessions SET started_at=clock_timestamp()-interval '4 minutes',expires_at=clock_timestamp()-interval '1 minute' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	_, base = launch()
	// Observe PostgreSQL, without a session GET: the worker must finish the
	// overdue session independently of a player reconnecting.
	completed := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var state string
		if err = fixture.Pool.QueryRow(context.Background(), "SELECT state FROM game.sessions WHERE id=$1", id).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state == "completed" {
			completed = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !completed {
		t.Fatal("overdue session required a player request to complete")
	}
	finished, _ := learningCall(t, client, base, "GET", "/v1/sessions/"+id, "", "", 200)
	if finished.Data.Session.Report == nil || finished.Data.Session.Report.Reason != "time_elapsed" || finished.Data.Session.Report.FinalAccount.BalanceMinor != "100000" {
		t.Fatal("reconnect did not return the durable final report")
	}
	events, _ := learningCall(t, client, base, "GET", "/v1/sessions/"+id+"/events?after=0", "", "", 200)
	if len(events.Events) != 3 || events.Events[2].Kind != "SESSION_COMPLETED" {
		t.Fatal("reconnect history incomplete or duplicated")
	}
	replayed, headers := learningCall(t, client, base, "POST", "/v1/sessions", `{"scenarioId":"spot-basics"}`, "crash-retry-command", 201)
	if replayed.Data.Session.ID != id || headers.Get("Idempotency-Replayed") != "true" {
		t.Fatal("crash recovery lost the original command receipt")
	}
	fresh, _ := learningCall(t, client, base, "POST", "/v1/sessions/"+id+"/restart", `{}`, "fresh-session-command", 201)
	if fresh.Data.Session.ID == id || fresh.Data.Account.ID == created.Data.Account.ID || fresh.Data.Account.BalanceMinor != "100000" {
		t.Fatal("restart reused the old paper account")
	}
	var count int
	if err = fixture.Pool.QueryRow(context.Background(), "SELECT count(*) FROM trading.journals").Scan(&count); err != nil || count != 2 {
		t.Fatal("retry or restart produced the wrong number of allocations")
	}
}

func TestScriptedTradeSurvivesCrashAndSettlesAtEndingPrice(t *testing.T) {
	requireLocal(t)
	fixture := testsupport.Postgres(t)
	known, _ := migrations.Embedded()
	if err := migrations.Apply(context.Background(), fixture.Pool, known); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	launch := func() (*process, string) {
		bind := address(t)
		p := start(t, "game-service", map[string]string{"COT_HTTP_ADDR": bind, "COT_DATABASE_URL": fixture.URL, "COT_DEV_PLAYER_ID": "scripted-smoke"})
		base := "http://" + bind
		awaitLiveness(t, client, base, p)
		return p, base
	}
	p, base := launch()
	created, _ := learningCall(t, client, base, "POST", "/v1/sessions", `{"scenarioId":"spot-market-buy-sell"}`, "script-smoke-start", 201)
	id := created.Data.Session.ID
	bought, _ := learningCall(t, client, base, "POST", "/v1/sessions/"+id+"/orders", `{"product":"spot","symbol":"SOL","side":"BUY","type":"MARKET","quantity":"1"}`, "script-smoke-buy", 200)
	if bought.Data.Trading == nil || bought.Data.Trading.Cash != "900.000000000000" {
		t.Fatal("built process did not execute spot order")
	}
	p.stopped = true
	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.done:
	case <-time.After(3 * time.Second):
		t.Fatal("owned process did not exit")
	}
	if _, err := fixture.Pool.Exec(context.Background(), "UPDATE game.sessions SET started_at=clock_timestamp()-interval '4 minutes',expires_at=clock_timestamp()-interval '1 minute' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	_, base = launch()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var state string
		if err := fixture.Pool.QueryRow(context.Background(), "SELECT state FROM game.sessions WHERE id=$1", id).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("trading worker did not settle without player")
		}
		time.Sleep(25 * time.Millisecond)
	}
	done, _ := learningCall(t, client, base, "GET", "/v1/sessions/"+id, "", "", 200)
	if done.Data.Trading == nil || done.Data.Trading.Cash != "1002.500000000000" || done.Data.Session.Report.LearningPassed {
		t.Fatal("wrong settlement price or automatic mastery credit")
	}
	_, headers := learningCall(t, client, base, "POST", "/v1/sessions/"+id+"/orders", `{"product":"spot","symbol":"SOL","side":"BUY","type":"MARKET","quantity":"1"}`, "script-smoke-buy", 200)
	if headers.Get("Idempotency-Replayed") != "true" {
		t.Fatal("accepted order receipt lost")
	}
	var count int
	if err := fixture.Pool.QueryRow(context.Background(), "SELECT count(*) FROM trading.journals").Scan(&count); err != nil || count != 5 {
		t.Fatal("recovery duplicated financial posting")
	}
}
