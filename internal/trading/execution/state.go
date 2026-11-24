// Package execution is the deterministic, network-free training state machine.
package execution

import (
	"errors"
	"fmt"
	"github.com/Santosh27927/Backend-COT/internal/exact"
	"github.com/Santosh27927/Backend-COT/internal/identity"
	"github.com/Santosh27927/Backend-COT/simulation/scripted"
)

var (
	ErrInvalid     = errors.New("invalid trading command")
	ErrFunds       = errors.New("insufficient available cash or owned quantity")
	ErrClosed      = errors.New("session is completed")
	ErrUnsupported = errors.New("feature is not enabled for this lesson")
)

const InitialCash int64 = 1_000_000_000_000_000 // USD 1000, scale 12.
type Command struct {
	Action      string `json:"action,omitempty"`
	OrderID     string `json:"orderId,omitempty"`
	Product     string `json:"product,omitempty"`
	Symbol      string `json:"symbol,omitempty"`
	Side        string `json:"side,omitempty"`
	Type        string `json:"type,omitempty"`
	TimeInForce string `json:"timeInForce,omitempty"`
	Quantity    string `json:"quantity,omitempty"`
	LimitPrice  string `json:"limitPrice,omitempty"`
	StopPrice   string `json:"stopPrice,omitempty"`
	Leverage    int    `json:"leverage,omitempty"`
	ReduceOnly  bool   `json:"reduceOnly,omitempty"`
}
type Order struct {
	ID, Product, Symbol, Side, Type, TimeInForce, Status, Reason                               string
	Quantity, LimitPrice, StopPrice, AcceptedMS, ReservedCash, ReservedAsset, ReservedPosition int64
	Triggered                                                                                  bool
	Leverage                                                                                   int
	ReduceOnly                                                                                 bool
}
type Fill struct {
	ID, OrderID, Product, Symbol, Side, Reason string
	Quantity, Price, OffsetMS                  int64
	PnL, Backstop, MarginChange                int64
}
type Posting struct {
	OperationID, Asset, Kind string
	Amount                   int64
	OffsetMS                 int64
}
type Event struct {
	Kind     string
	OffsetMS int64
	Payload  any
}
type Book struct {
	SchemaVersion int                 `json:"schemaVersion"`
	SessionID     string              `json:"sessionId"`
	Condition     scripted.Condition  `json:"condition"`
	Positions     map[string]Position `json:"positions,omitempty"`
	CursorMS      int64               `json:"cursorMs"`
	Cash          int64               `json:"cash"`
	Holdings      map[string]int64    `json:"holdings"`
	Orders        []Order             `json:"orders"`
	Fills         []Fill              `json:"fills"`
	Goals         map[string]bool     `json:"goals"`
	Postings      []Posting           `json:"-"`
	Events        []Event             `json:"-"`
}

func New(id string, c scripted.Condition) (*Book, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	return &Book{SchemaVersion: 1, SessionID: id, Condition: c, CursorMS: -1, Cash: InitialCash, Holdings: map[string]int64{"XRP": 0, "SOL": 0, "ETH": 0}, Orders: []Order{}, Fills: []Fill{}, Goals: map[string]bool{}, Positions: map[string]Position{}}, nil
}
func (b *Book) Passed() bool {
	for _, goal := range b.Condition.Goals {
		if !b.Goals[goal] {
			return false
		}
	}
	return true
}
func (b *Book) emit(kind string, at int64, payload any) {
	b.Events = append(b.Events, Event{Kind: kind, OffsetMS: at, Payload: payload})
}
func (b *Book) AvailableCash(exclude string) int64 {
	value := b.Cash - b.MarginUsed()
	for _, o := range b.Orders {
		if o.Status == "open" && o.ID != exclude {
			value -= o.ReservedCash
		}
	}
	return value
}
func (b *Book) AvailableAsset(symbol, exclude string) int64 {
	value := b.Holdings[symbol]
	for _, o := range b.Orders {
		if o.Status == "open" && o.Symbol == symbol && o.Product == "spot" && o.ID != exclude {
			value -= o.ReservedAsset
		}
	}
	return value
}
func (b *Book) mark(o Order) {
	if o.Type == "LIMIT" {
		b.Goals["limit_used"] = true
		if o.TimeInForce == "IOC" {
			b.Goals["ioc_used"] = true
		}
		if o.TimeInForce == "FOK" {
			b.Goals["fok_used"] = true
		}
	}
}
func (b *Book) eligible(o *Order, price, at int64) bool {
	if o.Type == "STOP_LIMIT" && !o.Triggered {
		if (o.Side == "BUY" && price >= o.StopPrice) || (o.Side == "SELL" && price <= o.StopPrice) {
			o.Triggered = true
			b.Goals["stop_triggered"] = true
			b.emit("ORDER_TRIGGERED", at, map[string]any{"orderId": o.ID, "price": exact.Format(price, 6)})
		}
		if !o.Triggered {
			return false
		}
	}
	return o.Type == "MARKET" || (o.Side == "BUY" && price <= o.LimitPrice) || (o.Side == "SELL" && price >= o.LimitPrice)
}
func (b *Book) cancel(o *Order, at int64, reason string) {
	o.Status = "canceled"
	o.Reason = reason
	o.ReservedCash = 0
	o.ReservedAsset = 0
	o.ReservedPosition = 0
	b.emit("ORDER_CANCELED", at, map[string]string{"orderId": o.ID, "reason": reason})
}
func (b *Book) submit(command Command, at int64) error {
	if len(b.Orders) >= 256 {
		return ErrInvalid
	}
	if !scripted.Has(b.Condition.Products, command.Product) || (command.Product == "spot" && (command.ReduceOnly || command.Leverage != 0)) {
		return ErrUnsupported
	}
	if !scripted.Has([]string{"XRP", "SOL", "ETH"}, command.Symbol) || !scripted.Has([]string{"BUY", "SELL"}, command.Side) || !scripted.Has(b.Condition.OrderTypes, command.Type) {
		return ErrInvalid
	}
	if command.TimeInForce == "" {
		if command.Type == "MARKET" {
			command.TimeInForce = "IOC"
		} else {
			command.TimeInForce = "GTC"
		}
	}
	if !scripted.Has(b.Condition.TimeInForce, command.TimeInForce) || (command.Type == "MARKET" && command.TimeInForce != "IOC") || (command.Type == "STOP_LIMIT" && command.TimeInForce != "GTC") {
		return ErrInvalid
	}
	qty, err := exact.Parse(command.Quantity)
	if err != nil || qty <= 0 {
		return ErrInvalid
	}
	o := Order{ID: identity.New(), Product: command.Product, Symbol: command.Symbol, Side: command.Side, Type: command.Type, TimeInForce: command.TimeInForce, Status: "open", Quantity: qty, AcceptedMS: at, Leverage: command.Leverage, ReduceOnly: command.ReduceOnly}
	if o.Type != "MARKET" {
		o.LimitPrice, err = exact.Parse(command.LimitPrice)
		if err != nil || o.LimitPrice <= 0 {
			return ErrInvalid
		}
	} else if command.LimitPrice != "" {
		return ErrInvalid
	}
	if o.Type == "STOP_LIMIT" {
		o.StopPrice, err = exact.Parse(command.StopPrice)
		if err != nil || o.StopPrice <= 0 {
			return ErrInvalid
		}
	} else if command.StopPrice != "" {
		return ErrInvalid
	}
	price, err := b.Condition.Price(o.Symbol, at)
	if err != nil {
		return ErrInvalid
	}
	reservedPrice := price
	if o.Type != "MARKET" {
		reservedPrice = o.LimitPrice
	}
	cost, err := exact.Product(qty, reservedPrice)
	if err != nil || cost <= 0 {
		return ErrInvalid
	}
	if o.Product == "perpetual" {
		if err = b.preparePerpetual(&o, cost); err != nil {
			return err
		}
	} else if o.Side == "BUY" {
		if cost > b.AvailableCash("") {
			return ErrFunds
		}
		o.ReservedCash = cost
	} else {
		if qty > b.AvailableAsset(o.Symbol, "") {
			return ErrFunds
		}
		o.ReservedAsset = qty
	}
	b.Orders = append(b.Orders, o)
	b.mark(o)
	b.emit("ORDER_ACCEPTED", at, publicOrder(o))
	target := &b.Orders[len(b.Orders)-1]
	if b.eligible(target, price, at) {
		return b.fill(target, price, at, "user")
	}
	if o.TimeInForce == "IOC" || o.TimeInForce == "FOK" {
		b.cancel(target, at, "price_not_eligible")
	}
	return nil
}
func (b *Book) fill(o *Order, price, at int64, reason string) error {
	if o.Product == "perpetual" {
		return b.fillPerpetual(o, price, at, reason)
	}
	value, err := exact.Product(o.Quantity, price)
	if err != nil {
		return err
	}
	if o.Side == "BUY" {
		if value > b.AvailableCash(o.ID) {
			b.cancel(o, at, "insufficient_cash")
			return nil
		}
		b.Cash -= value
		next, addErr := exact.Add(b.Holdings[o.Symbol], o.Quantity)
		if addErr != nil {
			return addErr
		}
		b.Holdings[o.Symbol] = next
	} else {
		if o.Quantity > b.AvailableAsset(o.Symbol, o.ID) {
			b.cancel(o, at, "insufficient_asset")
			return nil
		}
		b.Cash, err = exact.Add(b.Cash, value)
		if err != nil {
			return err
		}
		b.Holdings[o.Symbol] -= o.Quantity
	}
	id := identity.Stable(b.SessionID, o.ID, fmt.Sprint(at), "fill")
	cash, asset, kind := -value, o.Quantity, "SPOT_BUY"
	if o.Side == "SELL" {
		cash, asset, kind = value, -o.Quantity, "SPOT_SELL"
	}
	b.Postings = append(b.Postings, Posting{id, "USD", kind, cash, at}, Posting{id, o.Symbol, kind, asset, at})
	fill := Fill{ID: id, OrderID: o.ID, Product: o.Product, Symbol: o.Symbol, Side: o.Side, Reason: reason, Quantity: o.Quantity, Price: price, OffsetMS: at}
	b.Fills = append(b.Fills, fill)
	o.Status = "filled"
	o.ReservedCash = 0
	o.ReservedAsset = 0
	if reason == "user" {
		if o.Side == "BUY" {
			b.Goals["spot_buy"] = true
		} else {
			b.Goals["spot_sell"] = true
		}
		if o.TimeInForce == "GTC" {
			b.Goals["gtc_filled"] = true
		}
	}
	b.emit("ORDER_FILLED", at, publicFill(fill))
	return nil
}
func (b *Book) Reconcile(at int64) error {
	if at < 0 || at > 180000 {
		return ErrInvalid
	}
	for _, frame := range b.Condition.Frames {
		if frame.OffsetMS <= b.CursorMS || frame.OffsetMS > at {
			continue
		}
		if err := b.risk(frame.OffsetMS); err != nil {
			return err
		}
		for i := range b.Orders {
			o := &b.Orders[i]
			if o.Status != "open" {
				continue
			}
			price, err := b.Condition.Price(o.Symbol, frame.OffsetMS)
			if err != nil {
				return err
			}
			if b.eligible(o, price, frame.OffsetMS) {
				if err = b.fill(o, price, frame.OffsetMS, "user"); err != nil {
					return err
				}
			}
		}
		if err := b.risk(frame.OffsetMS); err != nil {
			return err
		}
		b.CursorMS = frame.OffsetMS
	}
	return nil
}
func (b *Book) Command(c Command, at int64) error {
	if at < 0 || at >= 180000 {
		return ErrClosed
	}
	if at < b.CursorMS {
		return ErrInvalid
	}
	if err := b.Reconcile(at); err != nil {
		return err
	}
	if c.Action == "cancel" {
		for i := range b.Orders {
			if b.Orders[i].ID == c.OrderID {
				if b.Orders[i].Status == "filled" {
					return ErrInvalid
				}
				if b.Orders[i].Status == "open" {
					b.cancel(&b.Orders[i], at, "user")
				}
				return nil
			}
		}
		return ErrInvalid
	}
	if c.Action != "" && c.Action != "submit" {
		return ErrInvalid
	}
	return b.submit(c, at)
}
func (b *Book) Settle(at int64) error {
	// At the fixed deadline, cancellation wins over new conditional fills.
	// The endpoint price is still used to value/close existing exposure.
	reconcileAt := at
	if at == int64(b.Condition.DurationSeconds)*1000 {
		reconcileAt = at - 1
	}
	if err := b.Reconcile(reconcileAt); err != nil {
		return err
	}
	for i := range b.Orders {
		if b.Orders[i].Status == "open" {
			b.cancel(&b.Orders[i], at, "session_end")
		}
	}
	if err := b.risk(at); err != nil {
		return err
	}
	for _, symbol := range []string{"XRP", "SOL", "ETH"} {
		qty := b.Holdings[symbol]
		if qty == 0 {
			continue
		}
		price, err := b.Condition.Price(symbol, at)
		if err != nil {
			return err
		}
		o := Order{ID: identity.Stable(b.SessionID, "close", symbol), Product: "spot", Symbol: symbol, Side: "SELL", Type: "MARKET", TimeInForce: "IOC", Status: "open", Quantity: qty, AcceptedMS: at}
		b.Orders = append(b.Orders, o)
		if err = b.fill(&b.Orders[len(b.Orders)-1], price, at, "session_end"); err != nil {
			return err
		}
	}
	for _, symbol := range []string{"XRP", "SOL", "ETH"} {
		price, err := b.Condition.Price(symbol, at)
		if err != nil {
			return err
		}
		if err = b.closePosition(symbol, price, at, "session_end"); err != nil {
			return err
		}
	}
	b.Goals["session_settled"] = true
	b.CursorMS = at
	return nil
}
