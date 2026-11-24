package sessions

import (
	"context"
	"log/slog"
	"time"
)

// RunWorker reconciles overdue sessions immediately after startup and then polls.
// PostgreSQL holds the schedule; no device timer or in-memory job is authoritative.
func (s *Store) RunWorker(ctx context.Context, logger *slog.Logger) {
	logger = logger.With("component", "session-worker")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	failed := false
	for {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		count, err := s.CompleteDue(checkCtx, 50)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			if !failed {
				logger.Error("session reconciliation failed", "code", "SESSION_RECONCILIATION_FAILED")
			}
			failed = true
		} else {
			if failed {
				logger.Info("session reconciliation recovered")
			}
			failed = false
			if count > 0 {
				logger.Info("due sessions reconciled", "count", count)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
