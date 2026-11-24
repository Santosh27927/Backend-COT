//go:build integration

package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Santosh27927/Backend-COT/internal/authentication"
	"github.com/Santosh27927/Backend-COT/internal/httpapi"
	"github.com/Santosh27927/Backend-COT/internal/testsupport"
	"github.com/Santosh27927/Backend-COT/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Every test creates and removes only its own randomly named cot_test_* database.
func isolatedDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testsupport.Postgres(t).Pool
}

func TestMigrationLifecycleAndDrift(t *testing.T) {
	pool := isolatedDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	known, err := migrations.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if ok, code := migrations.Ready(ctx, pool, known); ok || code != "migrations_required" {
		t.Fatalf("empty database ready=%v code=%s", ok, code)
	}
	for range 2 {
		if err := migrations.Apply(ctx, pool, known); err != nil {
			t.Fatal(err)
		}
	}
	if ok, code := migrations.Ready(ctx, pool, known); !ok || code != "ready" {
		t.Fatalf("migrated database ready=%v code=%s", ok, code)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM cot.schema_migrations").Scan(&count); err != nil || count != len(known) {
		t.Fatal("migration was duplicated or not recorded")
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.schemata WHERE schema_name IN ('game','trading')").Scan(&count); err != nil || count != 2 {
		t.Fatal("required domain schemas missing")
	}
	if _, err := pool.Exec(ctx, "UPDATE cot.schema_migrations SET checksum=$1 WHERE version=1", "tampered"); err != nil {
		t.Fatal(err)
	}
	if ok, code := migrations.Ready(ctx, pool, known); ok || code != "migration_mismatch" {
		t.Fatalf("modified migration ready=%v code=%s", ok, code)
	}
	if err := migrations.Apply(ctx, pool, known); !errors.Is(err, migrations.ErrDrift) {
		t.Fatal("modified migration was not rejected")
	}
}

func TestMigrationFailureRollsBackDDL(t *testing.T) {
	pool := isolatedDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	known := []migrations.Migration{
		{Version: 1, Name: "001_valid.sql", SQL: "CREATE TABLE public.cot_first(id integer)"},
		{Version: 2, Name: "002_invalid.sql", SQL: "CREATE TABLE public.cot_second(id integer); SELECT cot_missing_function()"},
	}
	if err := migrations.Apply(ctx, pool, known); err == nil {
		t.Fatal("invalid migration unexpectedly succeeded")
	}
	var first, second, history *string
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.cot_first')::text, to_regclass('public.cot_second')::text, to_regclass('cot.schema_migrations')::text").Scan(&first, &second, &history); err != nil {
		t.Fatal(err)
	}
	if first != nil || second != nil || history != nil {
		t.Fatal("failed migration committed partial state")
	}
}

func TestConcurrentMigrationRunners(t *testing.T) {
	pool := isolatedDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	known, err := migrations.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- migrations.Apply(ctx, pool, known) }()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM cot.schema_migrations").Scan(&count); err != nil || count != len(known) {
		t.Fatal("concurrent runners did not serialize")
	}
}

func TestReadinessTracksDatabaseAvailability(t *testing.T) {
	pool := isolatedDatabase(t)
	known, err := migrations.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.Handler("game-service", slog.New(slog.NewJSONHandler(io.Discard, nil)), authentication.NewDevelopment("unused-token", "local-player"), func(ctx context.Context) (bool, string) { return migrations.Ready(ctx, pool, known) })
	check := func(path string, status int) {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != status {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	check("/health/live", 200)
	check("/health/ready", 503)
	if err := migrations.Apply(context.Background(), pool, known); err != nil {
		t.Fatal(err)
	}
	check("/health/ready", 200)
	pool.Close()
	check("/health/live", 200)
	check("/health/ready", 503)
}
