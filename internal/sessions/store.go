// Package sessions coordinates durable learning sessions and paper allocations.
package sessions

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/Santosh27927/Backend-COT/internal/players"
	"github.com/Santosh27927/Backend-COT/internal/scenarios"
	"github.com/Santosh27927/Backend-COT/internal/trading/accounts"
	"github.com/Santosh27927/Backend-COT/internal/trading/execution"
	"github.com/Santosh27927/Backend-COT/internal/trading/ledger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sort"
)

var (
	ErrInvalid         = errors.New("invalid session command")
	ErrNotFound        = errors.New("session or scenario not found")
	ErrActive          = errors.New("scenario already has an active session")
	ErrKeyConflict     = errors.New("idempotency key used for a different command")
	ErrVersionConflict = errors.New("scenario version changed without a version increment")
	idPattern          = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
	keyPattern         = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)
	playerPattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

type Store struct {
	Pool        *pgxpool.Pool
	definition  scenarios.Definition
	definitions map[string]scenarios.Definition
}

func New(pool *pgxpool.Pool, definition scenarios.Definition) (*Store, error) {
	return NewCatalog(pool, []scenarios.Definition{definition})
}
func NewCatalog(pool *pgxpool.Pool, definitions []scenarios.Definition) (*Store, error) {
	if len(definitions) == 0 {
		return nil, ErrInvalid
	}
	store := &Store{Pool: pool, definitions: map[string]scenarios.Definition{}}
	for _, d := range definitions {
		if err := d.Validate(); err != nil {
			return nil, err
		}
		if _, exists := store.definitions[d.ID]; exists {
			return nil, ErrInvalid
		}
		data, _ := d.Snapshot()
		clone, err := scenarios.Restore(data)
		if err != nil {
			return nil, err
		}
		store.definitions[d.ID] = clone
	}
	store.definition = store.definitions[definitions[0].ID]
	return store, nil
}
func (s *Store) Scenarios() []scenarios.Definition {
	result := []scenarios.Definition{}
	for _, d := range s.definitions {
		result = append(result, d)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].TownHall != result[j].TownHall {
			return result[i].TownHall < result[j].TownHall
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func (s *Store) Scenario() scenarios.Definition {
	d := s.definition
	d.Objectives = append([]string(nil), d.Objectives...)
	return d
}

type Report struct {
	Reason                string              `json:"reason"`
	EndPolicy             string              `json:"endPolicy"`
	EffectiveAt           time.Time           `json:"effectiveAt"`
	ProcessedAt           time.Time           `json:"processedAt"`
	FinalAccount          accounts.Snapshot   `json:"finalAccount"`
	ExecutionEnabled      bool                `json:"executionEnabled"`
	PendingOrdersCanceled int                 `json:"pendingOrdersCanceled"`
	HoldingsClosed        int                 `json:"holdingsClosed"`
	Trading               *execution.Snapshot `json:"trading,omitempty"`
	LearningPassed        bool                `json:"learningPassed"`
}
type Session struct {
	ID          string               `json:"id"`
	PlayerID    string               `json:"playerId"`
	Scenario    scenarios.Definition `json:"scenario"`
	State       string               `json:"state"`
	StartedAt   time.Time            `json:"startedAt"`
	ExpiresAt   time.Time            `json:"expiresAt"`
	CompletedAt *time.Time           `json:"completedAt,omitempty"`
	Report      *Report              `json:"report,omitempty"`
}
type View struct {
	Session    Session             `json:"session"`
	Account    accounts.Snapshot   `json:"account"`
	ServerTime time.Time           `json:"serverTime"`
	Trading    *execution.Snapshot `json:"trading,omitempty"`
}
type StartCommand struct {
	ScenarioID       string `json:"scenarioId"`
	RestartSessionID string `json:"restartSessionId,omitempty"`
}

func uuid() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
func now(ctx context.Context, tx pgx.Tx) (time.Time, error) {
	var at time.Time
	err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&at)
	return at.UTC(), err
}

const selectSession = `SELECT s.id::text,s.player_id,s.state,s.started_at,s.expires_at,s.completed_at,s.result,v.definition FROM game.sessions s JOIN game.scenario_versions v ON (v.id,v.version)=(s.scenario_id,s.scenario_version) `

func readSession(ctx context.Context, tx pgx.Tx, player, id string) (Session, error) {
	var v Session
	var result, definition []byte
	err := tx.QueryRow(ctx, selectSession+"WHERE s.id=$1 AND s.player_id=$2 FOR UPDATE OF s", id, player).Scan(&v.ID, &v.PlayerID, &v.State, &v.StartedAt, &v.ExpiresAt, &v.CompletedAt, &result, &definition)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	if v.Scenario, err = scenarios.Restore(definition); err != nil {
		return v, err
	}
	if result != nil {
		v.Report = &Report{}
		if err = json.Unmarshal(result, v.Report); err != nil {
			return v, err
		}
	}
	return v, nil
}
func appendEvent(ctx context.Context, tx pgx.Tx, id, kind string, at time.Time, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO game.session_events(session_id,sequence,kind,effective_at,payload) SELECT $1,COALESCE(max(sequence),0)+1,$2,$3,$4 FROM game.session_events WHERE session_id=$1`, id, kind, at, data)
	return err
}
func complete(ctx context.Context, tx pgx.Tx, v *Session, at time.Time, reason string) error {
	if v.State != "active" {
		return nil
	}
	effective := at
	if reason == "time_elapsed" {
		effective = v.ExpiresAt
	}
	trading, canceled, closed, err := settleTrading(ctx, tx, *v, effective)
	if err != nil {
		return err
	}
	account, err := accounts.Read(ctx, tx, v.ID)
	if err != nil {
		return err
	}
	// No execution is enabled in this profile. Future execution must settle
	// orders/holdings atomically before a completed report can be published.
	report := Report{Reason: reason, EndPolicy: v.Scenario.EndPolicy, EffectiveAt: effective, ProcessedAt: at, FinalAccount: account, ExecutionEnabled: v.Scenario.ExecutionEnabled, Trading: trading, PendingOrdersCanceled: canceled, HoldingsClosed: closed, LearningPassed: trading != nil && trading.Passed}
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE game.sessions SET state='completed',completed_at=$2,completion_reason=$3,result=$4 WHERE id=$1", v.ID, effective, reason, data); err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, v.ID, "SESSION_COMPLETED", effective, report); err != nil {
		return err
	}
	v.State = "completed"
	v.CompletedAt = &effective
	v.Report = &report
	return nil
}
func expire(ctx context.Context, tx pgx.Tx, v *Session, at time.Time) error {
	if v.State == "active" && !at.Before(v.ExpiresAt) {
		return complete(ctx, tx, v, at, "time_elapsed")
	}
	return nil
}
func view(ctx context.Context, tx pgx.Tx, v Session, at time.Time) (View, error) {
	a, err := accounts.Read(ctx, tx, v.ID)
	if err != nil {
		return View{}, err
	}
	trading, err := tradingView(ctx, tx, v, at)
	return View{Session: v, Account: a, ServerTime: at, Trading: trading}, err
}

// Start commits the session, allocation, events, and retry receipt together.
func (s *Store) Start(ctx context.Context, player, key string, command StartCommand) (View, bool, error) {
	if !playerPattern.MatchString(player) || !keyPattern.MatchString(key) {
		return View{}, false, ErrInvalid
	}
	if command.RestartSessionID != "" {
		if !idPattern.MatchString(command.RestartSessionID) || command.ScenarioID != "" {
			return View{}, false, ErrInvalid
		}
	} else if _, ok := s.definitions[command.ScenarioID]; !ok {
		return View{}, false, ErrNotFound
	}
	profile := s.definition
	if command.RestartSessionID == "" {
		profile = s.definitions[command.ScenarioID]
	}
	encoded, _ := json.Marshal(command)
	hash := fmt.Sprintf("%x", sha256.Sum256(encoded))
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
		if err = tx.Commit(ctx); err != nil {
			return View{}, false, err
		}
		return response, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return View{}, false, err
	}
	if command.RestartSessionID != "" {
		old, err := readSession(ctx, tx, player, command.RestartSessionID)
		if err != nil {
			return View{}, false, err
		}
		var ok bool
		profile, ok = s.definitions[old.Scenario.ID]
		if !ok {
			return View{}, false, ErrNotFound
		}
	}
	if profile.Training != nil {
		for _, required := range profile.Training.Requires {
			var passed bool
			if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM game.lesson_progress WHERE player_id=$1 AND lesson_id=$2)", player, required).Scan(&passed); err != nil {
				return View{}, false, err
			}
			if !passed {
				return View{}, false, ErrLocked
			}
		}
	}
	var activeID string
	err = tx.QueryRow(ctx, "SELECT id::text FROM game.sessions WHERE player_id=$1 AND scenario_id=$2 AND state='active' FOR UPDATE", player, profile.ID).Scan(&activeID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return View{}, false, err
	}
	at, err := now(ctx, tx)
	if err != nil {
		return View{}, false, err
	}
	if activeID != "" {
		active, err := readSession(ctx, tx, player, activeID)
		if err != nil {
			return View{}, false, err
		}
		if err = expire(ctx, tx, &active, at); err != nil {
			return View{}, false, err
		}
		if active.State == "active" {
			if command.RestartSessionID != active.ID {
				return View{}, false, ErrActive
			}
			if err = complete(ctx, tx, &active, at, "restarted"); err != nil {
				return View{}, false, err
			}
		}
	}
	definition, checksum := profile.Snapshot()
	if _, err = tx.Exec(ctx, "INSERT INTO game.scenario_versions(id,version,definition,checksum) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING", profile.ID, profile.Version, definition, checksum); err != nil {
		return View{}, false, err
	}
	var existing string
	if err = tx.QueryRow(ctx, "SELECT checksum FROM game.scenario_versions WHERE id=$1 AND version=$2", profile.ID, profile.Version).Scan(&existing); err != nil {
		return View{}, false, err
	}
	if checksum != existing {
		return View{}, false, ErrVersionConflict
	}
	id, err := uuid()
	if err != nil {
		return View{}, false, err
	}
	accountID, err := uuid()
	if err != nil {
		return View{}, false, err
	}
	journalID, err := uuid()
	if err != nil {
		return View{}, false, err
	}
	v := Session{ID: id, PlayerID: player, Scenario: profile, State: "active", StartedAt: at, ExpiresAt: at.Add(time.Duration(profile.DurationSeconds) * time.Second)}
	if _, err = tx.Exec(ctx, "INSERT INTO game.sessions(id,player_id,scenario_id,scenario_version,state,started_at,expires_at) VALUES ($1,$2,$3,$4,'active',$5,$6)", id, player, profile.ID, profile.Version, v.StartedAt, v.ExpiresAt); err != nil {
		return View{}, false, err
	}
	if err = accounts.OpenScale(ctx, tx, accountID, id, player, at, profile.CurrencyScale); err != nil {
		return View{}, false, err
	}
	amount, _ := strconv.ParseInt(profile.StartingBalanceMinor, 10, 64) // Validated by New.
	if err = ledger.Allocate(ctx, tx, accountID, journalID, amount, at); err != nil {
		return View{}, false, err
	}
	if err = appendEvent(ctx, tx, id, "SESSION_STARTED", at, map[string]any{"scenarioId": profile.ID, "scenarioVersion": profile.Version, "expiresAt": v.ExpiresAt}); err != nil {
		return View{}, false, err
	}
	if err = appendEvent(ctx, tx, id, "DEMO_FUNDS_ALLOCATED", at, map[string]string{"currency": profile.Currency, "amountMinor": profile.StartingBalanceMinor, "journalId": journalID}); err != nil {
		return View{}, false, err
	}
	if err = initializeTrading(ctx, tx, v); err != nil {
		return View{}, false, err
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
	if err = tx.Commit(ctx); err != nil {
		return View{}, false, err
	}
	return response, false, nil
}

func (s *Store) Get(ctx context.Context, player, id string) (View, error) {
	if !idPattern.MatchString(id) {
		return View{}, ErrNotFound
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return View{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	v, err := readSession(ctx, tx, player, id)
	if err != nil {
		return View{}, err
	}
	at, err := now(ctx, tx)
	if err != nil {
		return View{}, err
	}
	if err = expire(ctx, tx, &v, at); err != nil {
		return View{}, err
	}
	if v.State == "active" {
		if err = reconcileTrading(ctx, tx, v, at); err != nil {
			return View{}, err
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE game.sessions SET last_reconciled_at=clock_timestamp() WHERE id=$1", v.ID); err != nil {
		return View{}, err
	}
	response, err := view(ctx, tx, v, at)
	if err != nil {
		return View{}, err
	}
	return response, tx.Commit(ctx)
}

type Summary struct {
	ID         string    `json:"id"`
	ScenarioID string    `json:"scenarioId"`
	State      string    `json:"state"`
	StartedAt  time.Time `json:"startedAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

func (s *Store) List(ctx context.Context, player string) ([]Summary, error) {
	rows, err := s.Pool.Query(ctx, "SELECT id::text,scenario_id,state,started_at,expires_at FROM game.sessions WHERE player_id=$1 ORDER BY started_at DESC,id DESC LIMIT 100", player)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Summary{}
	for rows.Next() {
		var v Summary
		if err = rows.Scan(&v.ID, &v.ScenarioID, &v.State, &v.StartedAt, &v.ExpiresAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

type Event struct {
	Sequence    int64           `json:"sequence"`
	Kind        string          `json:"kind"`
	EffectiveAt time.Time       `json:"effectiveAt"`
	RecordedAt  time.Time       `json:"recordedAt"`
	Payload     json.RawMessage `json:"payload"`
}

func (s *Store) Events(ctx context.Context, player, id string, after int64, limit int) ([]Event, error) {
	if after < 0 || limit < 1 || limit > 100 {
		return nil, ErrInvalid
	}
	if _, err := s.Get(ctx, player, id); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, "SELECT sequence,kind,effective_at,recorded_at,payload FROM game.session_events WHERE session_id=$1 AND sequence>$2 ORDER BY sequence LIMIT $3", id, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Event{}
	for rows.Next() {
		var v Event
		if err = rows.Scan(&v.Sequence, &v.Kind, &v.EffectiveAt, &v.RecordedAt, &v.Payload); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
func (s *Store) Ledger(ctx context.Context, player, id string) ([]ledger.Entry, error) {
	v, err := s.Get(ctx, player, id)
	if err != nil {
		return nil, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	entries, err := ledger.History(ctx, tx, v.Account.ID)
	if err != nil {
		return nil, err
	}
	return entries, tx.Commit(ctx)
}

// CompleteDue is safe to run from multiple processes. Each session is locked and
// rechecked before publishing its one terminal event and report.
func (s *Store) CompleteDue(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, ErrInvalid
	}
	rows, err := s.Pool.Query(ctx, "SELECT id::text,player_id FROM game.sessions WHERE state='active' ORDER BY (expires_at<=clock_timestamp()) DESC,last_reconciled_at,id LIMIT $1", limit)
	if err != nil {
		return 0, err
	}
	type due struct{ id, player string }
	pending := []due{}
	for rows.Next() {
		var v due
		if err = rows.Scan(&v.id, &v.player); err != nil {
			rows.Close()
			return 0, err
		}
		pending = append(pending, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for i, v := range pending {
		if _, err = s.Get(ctx, v.player, v.id); err != nil {
			return i, err
		}
	}
	return len(pending), nil
}
