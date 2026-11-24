package database

import (
	"context"
	"time"

	"github.com/Santosh27927/Backend-COT/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, settings config.Database) (*pgxpool.Pool, error) {
	parsed, err := pgxpool.ParseConfig(settings.URL)
	if err != nil {
		return nil, err
	}
	parsed.MaxConns = settings.MaxConnections
	parsed.MinConns = 0
	parsed.ConnConfig.ConnectTimeout = settings.Timeout
	parsed.MaxConnIdleTime = 5 * time.Minute
	parsed.MaxConnLifetime = 30 * time.Minute
	return pgxpool.NewWithConfig(ctx, parsed)
}
