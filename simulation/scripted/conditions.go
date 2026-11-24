// Package scripted embeds authored condition data; it never contacts a market.
package scripted

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/Santosh27927/Backend-COT/internal/exact"
	"io/fs"
	"sort"
	"strings"
)

//go:embed level-*/*.json
var files embed.FS

type Frame struct {
	OffsetMS int64             `json:"offsetMs"`
	Prices   map[string]string `json:"prices"`
}
type Condition struct {
	ID              string   `json:"id"`
	Version         int      `json:"version"`
	Level           int      `json:"level"`
	Title           string   `json:"title"`
	SourceMode      string   `json:"sourceMode"`
	DurationSeconds int      `json:"durationSeconds"`
	Products        []string `json:"products"`
	OrderTypes      []string `json:"orderTypes"`
	TimeInForce     []string `json:"timeInForce"`
	Goals           []string `json:"goals"`
	Requires        []string `json:"requires"`
	Frames          []Frame  `json:"frames"`
	FeeBps          int      `json:"feeBps"`
	FundingBps      int      `json:"fundingBps"`
	SlippageBps     int      `json:"slippageBps"`
	Liquidity       string   `json:"liquidity"`
	Leverages       []int    `json:"leverages,omitempty"`
	MaintenanceBps  int      `json:"maintenanceBps,omitempty"`
}

func Has(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func (c Condition) Validate() error {
	if c.ID == "" || c.Version < 1 || c.Level < 1 || c.Level > 3 || c.SourceMode != "scripted" || c.DurationSeconds != 180 || c.Liquidity != "unlimited" || c.FeeBps != 0 || c.FundingBps != 0 || c.SlippageBps != 0 || len(c.Frames) < 2 || len(c.Goals) == 0 || len(c.Products) == 0 {
		return fmt.Errorf("invalid scripted condition profile")
	}
	for _, p := range c.Products {
		if p != "spot" && p != "perpetual" {
			return fmt.Errorf("unsupported product")
		}
	}
	for _, kind := range c.OrderTypes {
		if !Has([]string{"MARKET", "LIMIT", "STOP_LIMIT"}, kind) {
			return fmt.Errorf("unsupported order type")
		}
	}
	for _, tif := range c.TimeInForce {
		if !Has([]string{"GTC", "IOC", "FOK"}, tif) {
			return fmt.Errorf("unsupported time in force")
		}
	}
	for i, f := range c.Frames {
		if f.OffsetMS < 0 || (i > 0 && f.OffsetMS <= c.Frames[i-1].OffsetMS) || len(f.Prices) != 3 {
			return fmt.Errorf("invalid scripted timeline")
		}
		for _, symbol := range []string{"XRP", "SOL", "ETH"} {
			price, err := exact.Parse(f.Prices[symbol])
			if err != nil || price <= 0 || price > 1_000_000_000_000 {
				return fmt.Errorf("invalid scripted price")
			}
		}
	}
	if c.Frames[0].OffsetMS != 0 || c.Frames[len(c.Frames)-1].OffsetMS != 180000 {
		return fmt.Errorf("script must cover session endpoints")
	}
	if Has(c.Products, "perpetual") {
		if c.MaintenanceBps != 100 || len(c.Leverages) == 0 {
			return fmt.Errorf("invalid training risk model")
		}
		for _, v := range c.Leverages {
			if v != 1 && v != 2 && v != 5 {
				return fmt.Errorf("unsupported leverage")
			}
		}
	}
	return nil
}
func (c Condition) Hash() string {
	data, _ := json.Marshal(c)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
func (c Condition) Price(symbol string, offset int64) (int64, error) {
	if offset < 0 || offset > int64(c.DurationSeconds)*1000 {
		return 0, fmt.Errorf("outside scripted timeline")
	}
	for i := len(c.Frames) - 1; i >= 0; i-- {
		if c.Frames[i].OffsetMS <= offset {
			return exact.Parse(c.Frames[i].Prices[symbol])
		}
	}
	return 0, fmt.Errorf("missing price")
}
func All() ([]Condition, error) {
	result := []Condition{}
	seen := map[string]bool{}
	err := fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		data, err := files.ReadFile(path)
		if err != nil {
			return err
		}
		var c Condition
		if err = json.Unmarshal(data, &c); err != nil {
			return err
		}
		if err = c.Validate(); err != nil {
			return err
		}
		if seen[c.ID] {
			return fmt.Errorf("duplicate condition ID")
		}
		seen[c.ID] = true
		result = append(result, c)
		return nil
	})
	sort.Slice(result, func(i, j int) bool {
		if result[i].Level != result[j].Level {
			return result[i].Level < result[j].Level
		}
		return result[i].ID < result[j].ID
	})
	return result, err
}
