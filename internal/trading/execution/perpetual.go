package execution

import (
	"fmt"
	"github.com/Santosh27927/Backend-COT/internal/exact"
	"github.com/Santosh27927/Backend-COT/internal/identity"
)

// Position uses weighted entry notional. It is one-way and isolated per symbol.
// This is a versioned training model, not a venue risk implementation.
type Position struct {
	Symbol, Side                    string
	Leverage                        int
	Quantity, EntryNotional, Margin int64
}

func ceilDivide(n, d int64) int64 {
	value := n / d
	if n%d != 0 {
		value++
	}
	return value
}
func (b *Book) MarginUsed() int64 {
	var amount int64
	for _, p := range b.Positions {
		amount += p.Margin
	}
	return amount
}
func (b *Book) AvailablePosition(symbol, exclude string) int64 {
	p := b.Positions[symbol]
	value := p.Quantity
	for _, o := range b.Orders {
		if o.Status == "open" && o.Product == "perpetual" && o.Symbol == symbol && o.ID != exclude {
			value -= o.ReservedPosition
		}
	}
	return value
}
func (b *Book) preparePerpetual(o *Order, cost int64) error {
	p, exists := b.Positions[o.Symbol]
	if o.ReduceOnly {
		if !exists || o.Side == p.Side || o.Quantity > b.AvailablePosition(o.Symbol, "") {
			return ErrFunds
		}
		if o.Leverage != 0 && o.Leverage != p.Leverage {
			return ErrUnsupported
		}
		o.Leverage = p.Leverage
		o.ReservedPosition = o.Quantity
		return nil
	}
	if o.Leverage == 0 {
		o.Leverage = 1
	}
	permitted := false
	for _, lev := range b.Condition.Leverages {
		if lev == o.Leverage {
			permitted = true
		}
	}
	if !permitted || (exists && (p.Side != o.Side || p.Leverage != o.Leverage)) {
		return ErrUnsupported
	}
	required := ceilDivide(cost, int64(o.Leverage))
	if required > b.AvailableCash("") {
		return ErrFunds
	}
	o.ReservedCash = required
	return nil
}
func (b *Book) fillPerpetual(o *Order, price, at int64, reason string) error {
	value, err := exact.Product(o.Quantity, price)
	if err != nil {
		return err
	}
	p, exists := b.Positions[o.Symbol]
	pnl, backstop, marginChange := int64(0), int64(0), int64(0)
	if o.ReduceOnly {
		if !exists || p.Side == o.Side || o.Quantity > b.AvailablePosition(o.Symbol, o.ID) {
			b.cancel(o, at, "position_not_available")
			return nil
		}
		basis, err := exact.Proportion(p.EntryNotional, o.Quantity, p.Quantity)
		if err != nil {
			return err
		}
		released, err := exact.Proportion(p.Margin, o.Quantity, p.Quantity)
		if err != nil {
			return err
		}
		if p.Side == "BUY" {
			pnl = value - basis
		} else {
			pnl = basis - value
		}
		if reason == "liquidation" && pnl < -released {
			backstop = -released - pnl
			pnl = -released
		}
		nextCash, err := exact.Add(b.Cash, pnl)
		if err != nil || nextCash < 0 {
			return fmt.Errorf("isolated accounting invariant failed")
		}
		b.Cash = nextCash
		marginChange = -released
		p.Quantity -= o.Quantity
		p.EntryNotional -= basis
		p.Margin -= released
		if p.Quantity == 0 {
			delete(b.Positions, o.Symbol)
		} else {
			b.Positions[o.Symbol] = p
		}
		if reason == "user" {
			b.Goals["perp_closed"] = true
		}
	} else {
		if exists && (p.Side != o.Side || p.Leverage != o.Leverage) {
			b.cancel(o, at, "position_direction_or_leverage_changed")
			return nil
		}
		required := ceilDivide(value, int64(o.Leverage))
		if required > b.AvailableCash(o.ID) {
			b.cancel(o, at, "insufficient_margin")
			return nil
		}
		qty, err := exact.Add(p.Quantity, o.Quantity)
		if err != nil {
			return err
		}
		entry, err := exact.Add(p.EntryNotional, value)
		if err != nil {
			return err
		}
		margin, err := exact.Add(p.Margin, required)
		if err != nil {
			return err
		}
		if b.Positions == nil {
			b.Positions = map[string]Position{}
		}
		b.Positions[o.Symbol] = Position{Symbol: o.Symbol, Side: o.Side, Leverage: o.Leverage, Quantity: qty, EntryNotional: entry, Margin: margin}
		marginChange = required
		if reason == "user" {
			if o.Side == "BUY" {
				b.Goals["perp_long"] = true
			} else {
				b.Goals["perp_short"] = true
			}
			b.Goals[fmt.Sprintf("leverage_%d", o.Leverage)] = true
		}
	}
	id := identity.Stable(b.SessionID, o.ID, fmt.Sprint(at), "fill")
	if pnl != 0 {
		b.Postings = append(b.Postings, Posting{id, "USD", "PERP_PNL", pnl, at})
	}
	fill := Fill{ID: id, OrderID: o.ID, Product: o.Product, Symbol: o.Symbol, Side: o.Side, Reason: reason, Quantity: o.Quantity, Price: price, OffsetMS: at, PnL: pnl, Backstop: backstop, MarginChange: marginChange}
	b.Fills = append(b.Fills, fill)
	o.Status = "filled"
	o.ReservedCash = 0
	o.ReservedAsset = 0
	o.ReservedPosition = 0
	if reason == "user" && o.TimeInForce == "GTC" {
		b.Goals["gtc_filled"] = true
	}
	b.emit("ORDER_FILLED", at, publicFill(fill))
	return nil
}
func (b *Book) closePosition(symbol string, price, at int64, reason string) error {
	p, exists := b.Positions[symbol]
	if !exists {
		return nil
	}
	side := "BUY"
	if p.Side == "BUY" {
		side = "SELL"
	}
	o := Order{ID: identity.Stable(b.SessionID, reason, symbol, fmt.Sprint(at)), Product: "perpetual", Symbol: symbol, Side: side, Type: "MARKET", TimeInForce: "IOC", Status: "open", Quantity: p.Quantity, AcceptedMS: at, Leverage: p.Leverage, ReduceOnly: true}
	// A system close supersedes queued reduce orders; release their claims first.
	for i := range b.Orders {
		if b.Orders[i].Status == "open" && b.Orders[i].Product == "perpetual" && b.Orders[i].Symbol == symbol {
			b.cancel(&b.Orders[i], at, reason)
		}
	}
	b.Orders = append(b.Orders, o)
	if err := b.fillPerpetual(&b.Orders[len(b.Orders)-1], price, at, reason); err != nil {
		return err
	}
	if reason == "liquidation" {
		b.Goals["liquidated"] = true
		f := b.Fills[len(b.Fills)-1]
		b.emit("POSITION_LIQUIDATED", at, map[string]string{"symbol": symbol, "price": exact.Format(price, 6), "recognizedPnl": exact.Format(f.PnL, 12), "trainingBackstop": exact.Format(f.Backstop, 12), "model": "isolated-training-v1"})
	}
	return nil
}
func (b *Book) risk(at int64) error {
	for _, symbol := range []string{"XRP", "SOL", "ETH"} {
		p, exists := b.Positions[symbol]
		if !exists {
			continue
		}
		price, err := b.Condition.Price(symbol, at)
		if err != nil {
			return err
		}
		notional, err := exact.Product(p.Quantity, price)
		if err != nil {
			return err
		}
		pnl := notional - p.EntryNotional
		if p.Side == "SELL" {
			pnl = -pnl
		}
		equity, err := exact.Add(p.Margin, pnl)
		if err != nil {
			return err
		}
		// Condition validation fixes maintenance at 100 bps = 1% for model v1.
		maintenance := ceilDivide(notional, 100)
		if equity <= maintenance {
			if err = b.closePosition(symbol, price, at, "liquidation"); err != nil {
				return err
			}
		}
	}
	return nil
}
