package execution

import "github.com/Santosh27927/Backend-COT/internal/exact"

type PositionView struct {
	Symbol            string `json:"symbol"`
	Direction         string `json:"direction"`
	Leverage          int    `json:"leverage"`
	Quantity          string `json:"quantity"`
	AvailableToClose  string `json:"availableToClose"`
	EntryPrice        string `json:"entryPrice"`
	EntryNotional     string `json:"entryNotional"`
	Notional          string `json:"notional"`
	Margin            string `json:"margin"`
	UnrealizedPnL     string `json:"unrealizedPnl"`
	IsolatedEquity    string `json:"isolatedEquity"`
	MaintenanceMargin string `json:"maintenanceMargin"`
}

func (b *Book) positionView(p Position, price int64) (PositionView, int64) {
	notional, _ := exact.Product(p.Quantity, price)
	pnl := notional - p.EntryNotional
	direction := "LONG"
	if p.Side == "SELL" {
		pnl = -pnl
		direction = "SHORT"
	}
	return PositionView{Symbol: p.Symbol, Direction: direction, Leverage: p.Leverage, Quantity: exact.Format(p.Quantity, 6), AvailableToClose: exact.Format(b.AvailablePosition(p.Symbol, ""), 6), EntryPrice: exact.Format(p.EntryNotional/p.Quantity, 6), EntryNotional: exact.Format(p.EntryNotional, 12), Notional: exact.Format(notional, 12), Margin: exact.Format(p.Margin, 12), UnrealizedPnL: exact.Format(pnl, 12), IsolatedEquity: exact.Format(p.Margin+pnl, 12), MaintenanceMargin: exact.Format(ceilDivide(notional, 100), 12)}, pnl
}
