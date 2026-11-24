//go:build integration || smoke

// Package testsupport creates isolated databases for boundary tests only.
package testsupport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/url"
	"os"
	"testing"
	"time"
)

type Database struct {
	Pool *pgxpool.Pool
	URL  string
}

func Postgres(t *testing.T) Database {
	t.Helper()
	if env := os.Getenv("COT_ENV"); env != "local" && env != "test" {
		t.Fatal("tests require COT_ENV=local or test")
	}
	raw := os.Getenv("COT_TEST_DATABASE_URL")
	if raw == "" {
		t.Fatal("set COT_TEST_DATABASE_URL to the COT-only PostgreSQL fixture")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatal("test fixture must use a PostgreSQL URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatal("cannot connect to local test fixture")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = admin.Close(cleanup)
	})
	var suffix [8]byte
	if _, err = rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "cot_test_" + hex.EncodeToString(suffix[:])
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal("cannot create isolated test database")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			t.Error("could not remove owned test database", name)
		}
	})
	u.Path = "/" + name
	u.RawPath = ""
	query := u.Query()
	query.Del("dbname")
	u.RawQuery = query.Encode()
	dsn := u.String()
	configuration, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test fixture configuration")
	}
	configuration.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, configuration)
	if err != nil {
		t.Fatal("cannot open isolated test pool")
	}
	t.Cleanup(pool.Close)
	return Database{Pool: pool, URL: dsn}
}
