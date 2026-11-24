package sessions

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Santosh27927/Backend-COT/internal/identity"
	"github.com/Santosh27927/Backend-COT/internal/players"
	"github.com/Santosh27927/Backend-COT/internal/trading/accounts"
	"github.com/Santosh27927/Backend-COT/internal/trading/execution"
	"github.com/Santosh27927/Backend-COT/internal/trading/ledger"
	"github.com/jackc/pgx/v5"
	"strconv"
	"time"
)

var ErrLocked = errors.New("complete prerequisite lessons first")

type LessonProgress struct {
	LessonID    string    `json:"lessonId"`
	SessionID   string    `json:"sessionId"`
	CompletedAt time.Time `json:"completedAt"`
}

// Progress is independent of the recent-session limit, so reinstalls/reconnects
// can restore course unlocks even after many old attempts.
func (s *Store) Progress(ctx context.Context, player string) ([]LessonProgress, error) {
	rows, err := s.Pool.Query(ctx, "SELECT lesson_id,session_id::text,completed_at FROM game.lesson_progress WHERE player_id=$1 ORDER BY completed_at,lesson_id", player)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []LessonProgress{}
	for rows.Next() {
		var v LessonProgress
		if err = rows.Scan(&v.LessonID, &v.SessionID, &v.CompletedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func offset(v Session, at time.Time) int64 {
	n := at.Sub(v.StartedAt).Milliseconds()
	if n < 0 {
		return 0
	}
	if n > 180000 {
		return 180000
	}
	return n
}
func loadBook(ctx context.Context, tx pgx.Tx, id string) (*execution.Book, error) {
	var data []byte
	if err := tx.QueryRow(ctx, "SELECT state FROM trading.execution_state WHERE session_id=$1", id).Scan(&data); err != nil {
		return nil, err
	}
	var b execution.Book
	err := json.Unmarshal(data, &b)
	return &b, err
}
func saveBook(ctx context.Context, tx pgx.Tx, v Session, b *execution.Book) error {
	if b.AvailableCash("") < 0 {
		return fmt.Errorf("negative available cash invariant")
	}
	for _, symbol := range []string{"XRP", "SOL", "ETH"} {
		if b.AvailableAsset(symbol, "") < 0 {
			return fmt.Errorf("negative available holding invariant")
		}
	}
	a, err := accounts.Read(ctx, tx, v.ID)
	if err != nil {
		return err
	}
	for _, post := range b.Postings {
		at := v.StartedAt.Add(time.Duration(post.OffsetMS) * time.Millisecond)
		if err = ledger.Post(ctx, tx, a.ID, identity.Stable(post.OperationID, post.Asset), post.OperationID, post.Asset, post.Kind, post.Amount, at); err != nil {
			return err
		}
	}
	for _, event := range b.Events {
		if err = appendEvent(ctx, tx, v.ID, event.Kind, v.StartedAt.Add(time.Duration(event.OffsetMS)*time.Millisecond), event.Payload); err != nil {
			return err
		}
	}
	// The state machine and immutable ledger must agree before either commits.
	expected := map[string]int64{"USD": b.Cash, "XRP": b.Holdings["XRP"], "SOL": b.Holdings["SOL"], "ETH": b.Holdings["ETH"]}
	for asset, want := range expected {
		var raw string
		if err = tx.QueryRow(ctx, "SELECT COALESCE(sum(e.amount_minor),0)::text FROM trading.journals j JOIN trading.ledger_entries e ON e.journal_id=j.id WHERE j.account_id=$1 AND j.asset=$2 AND e.bucket='player_cash'", a.ID, asset).Scan(&raw); err != nil {
			return err
		}
		got, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || got != want {
			return fmt.Errorf("execution ledger mismatch")
		}
	}
	b.Postings = nil
	b.Events = nil
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO trading.execution_state(session_id,state) VALUES ($1,$2) ON CONFLICT (session_id) DO UPDATE SET state=EXCLUDED.state,updated_at=clock_timestamp()", v.ID, data)
	return err
}
func initializeTrading(ctx context.Context, tx pgx.Tx, v Session) error {
	if !v.Scenario.ExecutionEnabled {
		return nil
	}
	b, err := execution.New(v.ID, *v.Scenario.Training)
	if err != nil {
		return err
	}
	if err = b.Reconcile(0); err != nil {
		return err
	}
	return saveBook(ctx, tx, v, b)
}
func reconcileTrading(ctx context.Context, tx pgx.Tx, v Session, at time.Time) error {
	if !v.Scenario.ExecutionEnabled {
		return nil
	}
	b, err := loadBook(ctx, tx, v.ID)
	if err != nil {
		return err
	}
	if err = b.Reconcile(offset(v, at)); err != nil {
		return err
	}
	return saveBook(ctx, tx, v, b)
}
func tradingView(ctx context.Context, tx pgx.Tx, v Session, at time.Time) (*execution.Snapshot, error) {
	if v.CompletedAt != nil {
		at = *v.CompletedAt
	}
	if !v.Scenario.ExecutionEnabled {
		return nil, nil
	}
	b, err := loadBook(ctx, tx, v.ID)
	if err != nil {
		return nil, err
	}
	snapshot := b.Snapshot(offset(v, at))
	return &snapshot, nil
}
func settleTrading(ctx context.Context, tx pgx.Tx, v Session, at time.Time) (*execution.Snapshot, int, int, error) {
	if !v.Scenario.ExecutionEnabled {
		return nil, 0, 0, nil
	}
	b, err := loadBook(ctx, tx, v.ID)
	if err != nil {
		return nil, 0, 0, err
	}
	if err = b.Settle(offset(v, at)); err != nil {
		return nil, 0, 0, err
	}
	canceled, closed := 0, 0
	for _, event := range b.Events {
		if event.Kind == "ORDER_CANCELED" {
			canceled++
		}
	}
	for _, f := range b.Fills {
		if f.Reason == "session_end" {
			closed++
		}
	}
	if err = saveBook(ctx, tx, v, b); err != nil {
		return nil, 0, 0, err
	}
	snapshot := b.Snapshot(offset(v, at))
	if snapshot.Passed {
		_, err = tx.Exec(ctx, "INSERT INTO game.lesson_progress(player_id,lesson_id,session_id,completed_at) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING", v.PlayerID, v.Scenario.ID, v.ID, at)
		if err != nil {
			return nil, 0, 0, err
		}
	}
	return &snapshot, canceled, closed, nil
}

// Trade serializes player commands, executes against server time, and commits
// effects/state/receipt together. Validation failures are not accepted orders.
func (s *Store) Trade(ctx context.Context, player, key, id string, command execution.Command) (View, bool, error) {
	if !playerPattern.MatchString(player) || !keyPattern.MatchString(key) {
		return View{}, false, ErrInvalid
	}
	if !idPattern.MatchString(id) {
		return View{}, false, ErrNotFound
	}
	raw, _ := json.Marshal(struct {
		Operation string
		SessionID string
		Command   execution.Command
	}{"trading", id, command})
	hash := fmt.Sprintf("%x", sha256.Sum256(raw))
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return View{}, false, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = players.Lock(ctx, tx, player); err != nil {
		return View{}, false, err
	}
	var previousHash string
	var previous []byte
	err = tx.QueryRow(ctx, "SELECT request_hash,response FROM game.command_receipts WHERE player_id=$1 AND command_key=$2", player, key).Scan(&previousHash, &previous)
	if err == nil {
		if previousHash != hash {
			return View{}, false, ErrKeyConflict
		}
		var response View
		if err = json.Unmarshal(previous, &response); err != nil {
			return View{}, false, err
		}
		return response, true, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return View{}, false, err
	}
	v, err := readSession(ctx, tx, player, id)
	if err != nil {
		return View{}, false, err
	}
	at, err := now(ctx, tx)
	if err != nil {
		return View{}, false, err
	}
	if err = expire(ctx, tx, &v, at); err != nil {
		return View{}, false, err
	}
	if v.State != "active" {
		if err = tx.Commit(ctx); err != nil {
			return View{}, false, err
		}
		return View{}, false, execution.ErrClosed
	}
	if command.Action == "finish" {
		if err = complete(ctx, tx, &v, at, "finished"); err != nil {
			return View{}, false, err
		}
	} else {
		if !v.Scenario.ExecutionEnabled {
			return View{}, false, execution.ErrUnsupported
		}
		b, err := loadBook(ctx, tx, id)
		if err != nil {
			return View{}, false, err
		}
		if err = b.Reconcile(offset(v, at)); err != nil {
			return View{}, false, err
		}
		if err = b.Command(command, offset(v, at)); err != nil {
			return View{}, false, err
		}
		if err = saveBook(ctx, tx, v, b); err != nil {
			return View{}, false, err
		}
	}
	response, err := view(ctx, tx, v, at)
	if err != nil {
		return View{}, false, err
	}
	data, err := json.Marshal(response)
	if err != nil {
		return View{}, false, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO game.command_receipts(player_id,command_key,request_hash,response) VALUES ($1,$2,$3,$4)", player, key, hash, data); err != nil {
		return View{}, false, err
	}
	return response, false, tx.Commit(ctx)
}
