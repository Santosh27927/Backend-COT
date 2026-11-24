// Package players persists the server-authenticated local player's identity.
package players

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// Lock serializes commands for one player, including retries on separate hosts.
func Lock(ctx context.Context, tx pgx.Tx, id string) error {
	if _, err := tx.Exec(ctx, "INSERT INTO game.players(id) VALUES ($1) ON CONFLICT DO NOTHING", id); err != nil {
		return err
	}
	var found string
	// Serialize commands without blocking the key-share lock taken by foreign
	// keys when a worker publishes lesson progress for this player.
	return tx.QueryRow(ctx, "SELECT id FROM game.players WHERE id=$1 FOR NO KEY UPDATE", id).Scan(&found)
}
