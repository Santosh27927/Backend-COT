// Package migrations embeds and applies versioned SQL migrations transactionally.
package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

//go:embed *.sql
var source embed.FS

var ErrDrift = errors.New("migration history differs from the application")

type Migration struct {
	Version int
	Name    string
	SQL     string
}

func (m Migration) Checksum() string { return fmt.Sprintf("%x", sha256.Sum256([]byte(m.SQL))) }

type Beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}
type Querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func Embedded() ([]Migration, error) {
	entries, err := source.ReadDir(".")
	if err != nil {
		return nil, err
	}
	result := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		version, err := strconv.Atoi(prefix)
		if err != nil || !ok {
			return nil, fmt.Errorf("invalid embedded migration filename")
		}
		body, err := source.ReadFile(entry.Name())
		if err != nil {
			return nil, err
		}
		result = append(result, Migration{Version: version, Name: entry.Name(), SQL: string(body)})
	}
	return result, validate(result)
}

func validate(known []Migration) error {
	if len(known) == 0 {
		return fmt.Errorf("at least one migration is required")
	}
	for i, migration := range known {
		if migration.Version != i+1 || migration.Name == "" || strings.TrimSpace(migration.SQL) == "" {
			return fmt.Errorf("migrations must be nonempty and numbered consecutively from 1")
		}
	}
	return nil
}

func checkHistory(ctx context.Context, q Querier, known []Migration) (int, error) {
	rows, err := q.Query(ctx, "SELECT version, name, checksum FROM cot.schema_migrations ORDER BY version")
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var version int
		var name, checksum string
		if err := rows.Scan(&version, &name, &checksum); err != nil {
			return 0, err
		}
		if count >= len(known) || version != known[count].Version || name != known[count].Name || checksum != known[count].Checksum() {
			return 0, ErrDrift
		}
		count++
	}
	return count, rows.Err()
}

func Apply(ctx context.Context, database Beginner, known []Migration) error {
	if err := validate(known); err != nil {
		return err
	}
	tx, err := database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	// Transaction-scoped lock prevents concurrent migration runners from racing.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(347891201)); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS cot; CREATE TABLE IF NOT EXISTS cot.schema_migrations (version integer PRIMARY KEY, name text NOT NULL, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	count, err := checkHistory(ctx, tx, known)
	if err != nil {
		return err
	}
	for _, migration := range known[count:] {
		if _, err = tx.Exec(ctx, migration.SQL); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO cot.schema_migrations (version, name, checksum) VALUES ($1, $2, $3)", migration.Version, migration.Name, migration.Checksum()); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func Ready(ctx context.Context, q Querier, known []Migration) (bool, string) {
	count, err := checkHistory(ctx, q, known)
	if err != nil {
		var postgresError *pgconn.PgError
		switch {
		case errors.Is(err, ErrDrift):
			return false, "migration_mismatch"
		case errors.As(err, &postgresError) && (postgresError.Code == "42P01" || postgresError.Code == "3F000"):
			return false, "migrations_required"
		default:
			return false, "database_unavailable"
		}
	}
	if count != len(known) {
		return false, "migrations_required"
	}
	return true, "ready"
}
