//go:build smoke

package smoke

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Santosh27927/Backend-COT/internal/sessions"
	"github.com/Santosh27927/Backend-COT/internal/testsupport"
	"github.com/Santosh27927/Backend-COT/migrations"
	"net/http"
	"testing"
	"time"
)

// This exercises the shipped HTTP routes and real prerequisites. Only the
// owned fixture clock is advanced; no test-only bypass is exposed by the API.
func TestScriptedSpotAndPerpetualCourse(t *testing.T) {
	requireLocal(t)
	fixture := testsupport.Postgres(t)
	known, _ := migrations.Embedded()
	if err := migrations.Apply(context.Background(), fixture.Pool, known); err != nil {
		t.Fatal(err)
	}
	bind := address(t)
	p := start(t, "game-service", map[string]string{"COT_HTTP_ADDR": bind, "COT_DATABASE_URL": fixture.URL, "COT_DEV_PLAYER_ID": "course-smoke"})
	base := "http://" + bind
	client := &http.Client{Timeout: 5 * time.Second}
	awaitLiveness(t, client, base, p)
	seq := 0
	post := func(path string, body any) sessions.View {
		t.Helper()
		seq++
		raw, _ := json.Marshal(body)
		v, _ := learningCall(t, client, base, "POST", path, string(raw), fmt.Sprintf("course-command-%04d", seq), map[bool]int{true: 201, false: 200}[path == "/v1/sessions"])
		return v.Data
	}
	begin := func(id string) sessions.View { return post("/v1/sessions", map[string]string{"scenarioId": id}) }
	order := func(v sessions.View, body map[string]any) sessions.View {
		return post("/v1/sessions/"+v.Session.ID+"/orders", body)
	}
	finish := func(v sessions.View) sessions.View {
		t.Helper()
		done := post("/v1/sessions/"+v.Session.ID+"/finish", struct{}{})
		if done.Session.Report == nil || !done.Session.Report.LearningPassed {
			t.Fatalf("lesson did not pass: %s", v.Session.Scenario.ID)
		}
		return done
	}
	advance := func(v sessions.View, ms int64) {
		t.Helper()
		if _, err := fixture.Pool.Exec(context.Background(), "UPDATE game.sessions SET started_at=clock_timestamp()-$2::bigint*interval '1 millisecond',expires_at=clock_timestamp()+(180000-$2::bigint)*interval '1 millisecond' WHERE id=$1", v.Session.ID, ms); err != nil {
			t.Fatal(err)
		}
	}
	spot := func(side string) map[string]any {
		return map[string]any{"product": "spot", "symbol": "XRP", "side": side, "type": "MARKET", "quantity": "1"}
	}
	perp := func(side string, lev int, reduce bool) map[string]any {
		return map[string]any{"product": "perpetual", "symbol": "SOL", "side": side, "type": "MARKET", "quantity": "1", "leverage": lev, "reduceOnly": reduce}
	}

	v := begin("spot-market-buy-sell")
	order(v, spot("BUY"))
	order(v, spot("SELL"))
	finish(v)
	v = begin("spot-gtc-price-wait")
	c := spot("BUY")
	c["type"] = "LIMIT"
	c["timeInForce"] = "GTC"
	c["limitPrice"] = "1.8"
	order(v, c)
	advance(v, 95000)
	finish(v)
	v = begin("spot-ioc-fok-immediate")
	for _, tif := range []string{"IOC", "FOK"} {
		c = spot("BUY")
		c["type"] = "LIMIT"
		c["timeInForce"] = tif
		c["limitPrice"] = "2"
		order(v, c)
	}
	finish(v)
	v = begin("spot-stop-limit-trigger")
	c = spot("BUY")
	c["type"] = "STOP_LIMIT"
	c["timeInForce"] = "GTC"
	c["stopPrice"] = "2.1"
	c["limitPrice"] = "2.2"
	order(v, c)
	advance(v, 35000)
	finish(v)

	v = begin("perpetual-long-short-1x")
	order(v, perp("BUY", 1, false))
	order(v, perp("SELL", 0, true))
	order(v, perp("SELL", 1, false))
	order(v, perp("BUY", 0, true))
	finish(v)
	v = begin("perpetual-isolated-leverage-2x")
	order(v, perp("BUY", 2, false))
	order(v, perp("SELL", 0, true))
	finish(v)
	v = begin("perpetual-isolated-liquidation-5x")
	c = perp("BUY", 5, false)
	c["quantity"] = "10"
	order(v, c)
	advance(v, 130000)
	deadline := time.Now().Add(5 * time.Second)
	for {
		var count int
		if err := fixture.Pool.QueryRow(context.Background(), "SELECT count(*) FROM game.session_events WHERE session_id=$1 AND kind='POSITION_LIQUIDATED'", v.Session.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("liquidation waited for an app request")
		}
		time.Sleep(25 * time.Millisecond)
	}
	done := finish(v)
	if done.Trading.Cash != "800.000000000000" {
		t.Fatal("course liquidation result wrong")
	}
	v = begin("perpetual-order-controls")
	c = perp("BUY", 1, false)
	c["type"] = "LIMIT"
	c["timeInForce"] = "GTC"
	c["limitPrice"] = "90"
	order(v, c)
	advance(v, 95000)
	for _, tif := range []string{"IOC", "FOK"} {
		c = perp("BUY", 1, false)
		c["type"] = "LIMIT"
		c["timeInForce"] = tif
		c["limitPrice"] = "100"
		order(v, c)
	}
	c = perp("BUY", 1, false)
	c["type"] = "STOP_LIMIT"
	c["timeInForce"] = "GTC"
	c["stopPrice"] = "100"
	c["limitPrice"] = "110"
	order(v, c)
	advance(v, 125000)
	finish(v)
	v = begin("combined-spot-perpetual-exposure")
	order(v, spot("BUY"))
	order(v, perp("BUY", 1, false))
	order(v, spot("SELL"))
	order(v, perp("SELL", 0, true))
	finish(v)
	v = begin("combined-wallet-margin-accounting")
	order(v, spot("BUY"))
	order(v, perp("BUY", 2, false))
	order(v, spot("SELL"))
	order(v, perp("SELL", 0, true))
	finish(v)
	v = begin("combined-session-completion")
	c = spot("BUY")
	c["symbol"] = "SOL"
	c["quantity"] = "5"
	order(v, c)
	c = perp("BUY", 5, false)
	c["quantity"] = "25"
	order(v, c)
	advance(v, 181000)
	deadline = time.Now().Add(5 * time.Second)
	for {
		var state string
		if err := fixture.Pool.QueryRow(context.Background(), "SELECT state FROM game.sessions WHERE id=$1", v.Session.ID).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("mixed lesson required a player request to finish")
		}
		time.Sleep(25 * time.Millisecond)
	}
	completed, _ := learningCall(t, client, base, "GET", "/v1/sessions/"+v.Session.ID, "", "", 200)
	if completed.Data.Session.Report == nil || !completed.Data.Session.Report.LearningPassed || completed.Data.Trading.Cash != "500.000000000000" || len(completed.Data.Trading.Positions) != 0 {
		t.Fatal("mixed liquidation/settlement result wrong")
	}
	var passed int
	if err := fixture.Pool.QueryRow(context.Background(), "SELECT count(*) FROM game.lesson_progress WHERE player_id='course-smoke'").Scan(&passed); err != nil || passed != 11 {
		t.Fatal("course progress was not persisted")
	}
	progress, _ := learningCall(t, client, base, "GET", "/v1/progress", "", "", 200)
	if len(progress.Progress) != 11 {
		t.Fatal("app cannot restore completed course progress")
	}
}
