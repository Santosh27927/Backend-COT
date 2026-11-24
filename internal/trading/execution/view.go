package execution

import (
	"github.com/Santosh27927/Backend-COT/internal/exact"
)

type OrderView struct {
	ID          string `json:"id"`
	Product     string `json:"product"`
	Symbol      string `json:"symbol"`
	Side        string `json:"side"`
	Type        string `json:"type"`
	TimeInForce string `json:"timeInForce"`
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
	Quantity    string `json:"quantity"`
	LimitPrice  string `json:"limitPrice,omitempty"`
	StopPrice   string `json:"stopPrice,omitempty"`
	AcceptedMS  int64  `json:"acceptedMs"`
	Triggered   bool   `json:"triggered"`
	Leverage    int    `json:"leverage,omitempty"`
	ReduceOnly  bool   `json:"reduceOnly,omitempty"`
}

func publicOrder(o Order) OrderView {
	v := OrderView{ID: o.ID, Product: o.Product, Symbol: o.Symbol, Side: o.Side, Type: o.Type, TimeInForce: o.TimeInForce, Status: o.Status, Reason: o.Reason, Quantity: exact.Format(o.Quantity, 6), AcceptedMS: o.AcceptedMS, Triggered: o.Triggered, Leverage: o.Leverage, ReduceOnly: o.ReduceOnly}
	if o.LimitPrice > 0 {
		v.LimitPrice = exact.Format(o.LimitPrice, 6)
	}
	if o.StopPrice > 0 {
		v.StopPrice = exact.Format(o.StopPrice, 6)
	}
	return v
}

type FillView struct {
	ID               string `json:"id"`
	OrderID          string `json:"orderId"`
	Product          string `json:"product"`
	Symbol           string `json:"symbol"`
	Side             string `json:"side"`
	Quantity         string `json:"quantity"`
	Price            string `json:"price"`
	Value            string `json:"value"`
	Fee              string `json:"fee"`
	OffsetMS         int64  `json:"offsetMs"`
	Reason           string `json:"reason"`
	RealizedPnL      string `json:"realizedPnl,omitempty"`
	TrainingBackstop string `json:"trainingBackstop,omitempty"`
	MarginChange     string `json:"marginChange,omitempty"`
}

func publicFill(f Fill) FillView {
	value, _ := exact.Product(f.Quantity, f.Price)
	v := FillView{ID: f.ID, OrderID: f.OrderID, Product: f.Product, Symbol: f.Symbol, Side: f.Side, Quantity: exact.Format(f.Quantity, 6), Price: exact.Format(f.Price, 6), Value: exact.Format(value, 12), Fee: "0", OffsetMS: f.OffsetMS, Reason: f.Reason}
	if f.Product == "perpetual" {
		v.RealizedPnL = exact.Format(f.PnL, 12)
		v.TrainingBackstop = exact.Format(f.Backstop, 12)
		v.MarginChange = exact.Format(f.MarginChange, 12)
	}
	return v
}

type HoldingView struct {
	Symbol    string `json:"symbol"`
	Quantity  string `json:"quantity"`
	Available string `json:"available"`
	Value     string `json:"value"`
}
type Snapshot struct {
	FundingBps     int               `json:"fundingBps"`
	Positions      []PositionView    `json:"positions"`
	MarginUsed     string            `json:"marginUsed"`
	Equity         string            `json:"equity"`
	Products       []string          `json:"products"`
	OrderTypes     []string          `json:"orderTypes"`
	TimeInForce    []string          `json:"timeInForce"`
	Leverages      []int             `json:"leverages,omitempty"`
	MaintenanceBps int               `json:"maintenanceBps,omitempty"`
	Mode           string            `json:"mode"`
	Cash           string            `json:"cash"`
	AvailableCash  string            `json:"availableCash"`
	Currency       string            `json:"currency"`
	CashScale      int               `json:"cashScale"`
	QuantityScale  int               `json:"quantityScale"`
	Prices         map[string]string `json:"prices"`
	Holdings       []HoldingView     `json:"holdings"`
	Orders         []OrderView       `json:"orders"`
	Fills          []FillView        `json:"fills"`
	Goals          map[string]bool   `json:"goals"`
	Passed         bool              `json:"passed"`
	FeeBps         int               `json:"feeBps"`
	SlippageBps    int               `json:"slippageBps"`
	Liquidity      string            `json:"liquidity"`
}

func (b *Book) Snapshot(at int64) Snapshot {
	v := Snapshot{Mode: "scripted", Cash: exact.Format(b.Cash, 12), AvailableCash: exact.Format(b.AvailableCash(""), 12), Currency: "USD", CashScale: 12, QuantityScale: 6, Prices: map[string]string{}, Holdings: []HoldingView{}, Orders: []OrderView{}, Fills: []FillView{}, Goals: map[string]bool{}, Passed: b.Passed(), Liquidity: "unlimited", Positions: []PositionView{}, MarginUsed: exact.Format(b.MarginUsed(), 12), Products: append([]string(nil), b.Condition.Products...), OrderTypes: append([]string(nil), b.Condition.OrderTypes...), TimeInForce: append([]string(nil), b.Condition.TimeInForce...), Leverages: append([]int(nil), b.Condition.Leverages...), MaintenanceBps: b.Condition.MaintenanceBps}
	equity := b.Cash
	for _, s := range []string{"XRP", "SOL", "ETH"} {
		p, _ := b.Condition.Price(s, at)
		v.Prices[s] = exact.Format(p, 6)
		value, _ := exact.Product(b.Holdings[s], p)
		equity += value
		if position, exists := b.Positions[s]; exists {
			pv, pnl := b.positionView(position, p)
			v.Positions = append(v.Positions, pv)
			equity += pnl
		}
		v.Holdings = append(v.Holdings, HoldingView{Symbol: s, Quantity: exact.Format(b.Holdings[s], 6), Available: exact.Format(b.AvailableAsset(s, ""), 6), Value: exact.Format(value, 12)})
	}
	for _, o := range b.Orders {
		v.Orders = append(v.Orders, publicOrder(o))
	}
	for _, f := range b.Fills {
		v.Fills = append(v.Fills, publicFill(f))
	}
	v.Equity = exact.Format(equity, 12)
	for k, value := range b.Goals {
		v.Goals[k] = value
	}
	return v
}
