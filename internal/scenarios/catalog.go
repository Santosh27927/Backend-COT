// Package scenarios holds immutable, versioned learning rules.
package scenarios

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Santosh27927/Backend-COT/simulation/scripted"
	"strconv"
)

type Definition struct {
	ID                   string              `json:"id"`
	Version              int                 `json:"version"`
	Title                string              `json:"title"`
	TownHall             int                 `json:"townHall"`
	Market               string              `json:"market"`
	Currency             string              `json:"currency"`
	StartingBalanceMinor string              `json:"startingBalanceMinor"`
	CurrencyScale        int                 `json:"currencyScale"`
	DurationSeconds      int                 `json:"durationSeconds"`
	EndPolicy            string              `json:"endPolicy"`
	ExecutionEnabled     bool                `json:"executionEnabled"`
	SourceMode           string              `json:"sourceMode,omitempty"`
	ScriptID             string              `json:"scriptId,omitempty"`
	Training             *scripted.Condition `json:"-"`
	Objectives           []string            `json:"objectives"`
}

// Foundation is deliberately an account/session exercise, before spot execution.
func Foundation() Definition {
	return Definition{ID: "spot-basics", Version: 1, Title: "Spot basics: your demo account", TownHall: 1, Market: "spot", Currency: "USD", StartingBalanceMinor: "100000", CurrencyScale: 2, DurationSeconds: 180, EndPolicy: "cancel_pending_and_close_holdings", ExecutionEnabled: false, Objectives: []string{"Understand your demo cash balance", "Read the session timer and account history"}}
}

func (d Definition) Validate() error {
	if d.Training != nil {
		c := d.Training
		if err := c.Validate(); err != nil {
			return err
		}
		if !d.ExecutionEnabled || d.ID != c.ID || d.Version != c.Version || d.SourceMode != "scripted" || d.ScriptID != c.ID || d.TownHall != c.Level || d.DurationSeconds != c.DurationSeconds || d.Currency != "USD" || d.CurrencyScale != 12 || d.StartingBalanceMinor != "1000000000000000" || d.EndPolicy != "cancel_pending_and_close_holdings" {
			return fmt.Errorf("invalid executable scenario")
		}
		return nil
	}
	if d.SourceMode != "" || d.ScriptID != "" {
		return fmt.Errorf("unsupported profile metadata")
	}

	amount, err := strconv.ParseInt(d.StartingBalanceMinor, 10, 64)
	if err != nil || amount < 1 || amount > 1_000_000_000_000 || strconv.FormatInt(amount, 10) != d.StartingBalanceMinor {
		return fmt.Errorf("starting balance must be a canonical positive minor-unit integer within the demo limit")
	}
	if d.ID != "spot-basics" || d.Version < 1 || d.TownHall != 1 || d.Market != "spot" || d.Currency != "USD" || d.CurrencyScale != 2 || d.DurationSeconds < 1 || d.DurationSeconds > 3600 || d.EndPolicy != "cancel_pending_and_close_holdings" || d.ExecutionEnabled || len(d.Objectives) == 0 {
		return fmt.Errorf("unsupported foundation scenario rules")
	}
	return nil
}

func (d Definition) Snapshot() ([]byte, string) {
	type wire Definition
	data, _ := json.Marshal(struct {
		wire
		PrivateTraining *scripted.Condition `json:"privateTraining,omitempty"`
	}{wire(d), d.Training})
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:])
}
func Restore(data []byte) (Definition, error) {
	type wire Definition
	var v struct {
		wire
		PrivateTraining *scripted.Condition `json:"privateTraining,omitempty"`
	}
	err := json.Unmarshal(data, &v)
	d := Definition(v.wire)
	d.Training = v.PrivateTraining
	return d, err
}
func Catalog() ([]Definition, error) {
	conditions, err := scripted.All()
	if err != nil {
		return nil, err
	}
	result := []Definition{Foundation()}
	for _, c := range conditions {
		kind := c.Products[0]
		if len(c.Products) > 1 {
			kind = "mixed"
		}
		copy := c
		result = append(result, Definition{ID: c.ID, Version: c.Version, Title: c.Title, TownHall: c.Level, Market: kind, Currency: "USD", StartingBalanceMinor: "1000000000000000", CurrencyScale: 12, DurationSeconds: c.DurationSeconds, EndPolicy: "cancel_pending_and_close_holdings", ExecutionEnabled: true, SourceMode: "scripted", ScriptID: c.ID, Training: &copy, Objectives: append([]string(nil), c.Goals...)})
	}
	return result, nil
}

// MarshalJSON exposes prerequisites but never the private future price timeline.
// Snapshot uses an alias directly, preserving legacy checksum bytes.
func (d Definition) MarshalJSON() ([]byte, error) {
	type wire Definition
	var requires []string
	if d.Training != nil {
		requires = append([]string(nil), d.Training.Requires...)
	}
	return json.Marshal(struct {
		wire
		Requires []string `json:"requires,omitempty"`
	}{wire(d), requires})
}
