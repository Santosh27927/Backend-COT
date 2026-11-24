// Package ledger owns balanced, append-only financial records.
package ledger

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
)

func Allocate(ctx context.Context, tx pgx.Tx, accountID, journalID string, minor int64, at time.Time) error {
	if minor <= 0 {
		return fmt.Errorf("allocation must be positive")
	}
	return Post(ctx, tx, accountID, journalID, journalID, "USD", "DEMO_ALLOCATION", minor, at)
}

// Post records one exact single-asset transfer against the simulation source.
func Post(ctx context.Context, tx pgx.Tx, accountID, journalID, operationID, asset, kind string, amount int64, at time.Time) error {
	if amount == 0 {
		return nil
	}
	if amount == -9223372036854775808 {
		return fmt.Errorf("amount out of range")
	}
	if _, err := tx.Exec(ctx, "INSERT INTO trading.journals(id,account_id,kind,effective_at,asset,operation_id) VALUES ($1,$2,$3,$4,$5,$6)", journalID, accountID, kind, at, asset, operationID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "INSERT INTO trading.ledger_entries(journal_id,bucket,amount_minor) VALUES ($1,'player_cash',$2),($1,'demo_source',$3)", journalID, amount, -amount)
	return err
}

type Entry struct {
	OperationID string    `json:"operationId"`
	Scale       int       `json:"scale"`
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	AmountMinor string    `json:"amountMinor"`
	Asset       string    `json:"asset"`
	EffectiveAt time.Time `json:"effectiveAt"`
}

func History(ctx context.Context, tx pgx.Tx, accountID string) ([]Entry, error) {
	rows, err := tx.Query(ctx, "SELECT j.id::text,j.kind,e.amount_minor::text,j.asset,COALESCE(j.operation_id::text,''),CASE WHEN j.asset='USD' THEN a.currency_scale ELSE 6 END,j.effective_at FROM trading.journals j JOIN trading.accounts a ON a.id=j.account_id JOIN trading.ledger_entries e ON e.journal_id=j.id WHERE j.account_id=$1 AND e.bucket='player_cash' ORDER BY j.effective_at,j.id", accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Entry{}
	for rows.Next() {
		var entry Entry
		if err := rows.Scan(&entry.ID, &entry.Kind, &entry.AmountMinor, &entry.Asset, &entry.OperationID, &entry.Scale, &entry.EffectiveAt); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}
