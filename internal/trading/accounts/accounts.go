// Package accounts owns paper account creation and balance queries.
package accounts

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

type Snapshot struct {
	ID           string `json:"id"`
	Currency     string `json:"currency"`
	Scale        int    `json:"scale"`
	BalanceMinor string `json:"balanceMinor"`
	Demo         bool   `json:"demo"`
}

func Open(ctx context.Context, tx pgx.Tx, id, sessionID, playerID string, at time.Time) error {
	return OpenScale(ctx, tx, id, sessionID, playerID, at, 2)
}
func OpenScale(ctx context.Context, tx pgx.Tx, id, sessionID, playerID string, at time.Time, scale int) error {
	_, err := tx.Exec(ctx, "INSERT INTO trading.accounts(id,session_id,player_id,currency,created_at,currency_scale) VALUES ($1,$2,$3,'USD',$4,$5)", id, sessionID, playerID, at, scale)
	return err
}

func Read(ctx context.Context, tx pgx.Tx, sessionID string) (Snapshot, error) {
	var s Snapshot
	err := tx.QueryRow(ctx, `SELECT a.id::text,a.currency,a.currency_scale,COALESCE((SELECT sum(e.amount_minor) FROM trading.journals j JOIN trading.ledger_entries e ON e.journal_id=j.id WHERE j.account_id=a.id AND e.bucket='player_cash' AND j.asset='USD'),0)::text FROM trading.accounts a WHERE a.session_id=$1`, sessionID).Scan(&s.ID, &s.Currency, &s.Scale, &s.BalanceMinor)
	s.Demo = true
	return s, err
}
