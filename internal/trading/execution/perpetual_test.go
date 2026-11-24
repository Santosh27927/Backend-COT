package execution

import (
	"errors"
	"testing"
)

func perp(side, qty string, leverage int, reduce bool) Command {
	return Command{Product: "perpetual", Symbol: "SOL", Side: side, Type: "MARKET", Quantity: qty, Leverage: leverage, ReduceOnly: reduce}
}
func TestWeightedPositionIncreaseReduceAndRealizedPnL(t *testing.T) {
	b := book(t, "perpetual-isolated-leverage-2x")
	if err := b.Command(perp("BUY", "10", 2, false), 0); err != nil {
		t.Fatal(err)
	}
	if b.Cash != InitialCash || b.MarginUsed() != 500_000_000_000_000 || b.AvailableCash("") != 500_000_000_000_000 || b.Holdings["SOL"] != 0 {
		t.Fatal("notional was treated as cash or spot ownership")
	}
	if err := b.Command(perp("SELL", "4", 0, true), 30000); err != nil {
		t.Fatal(err)
	}
	p := b.Positions["SOL"]
	if b.Cash != 1_020_000_000_000_000 || p.Quantity != 6_000_000 || p.Margin != 300_000_000_000_000 {
		t.Fatal("partial close accounting incorrect")
	}
	if err := b.Command(perp("BUY", "2", 2, false), 30000); err != nil {
		t.Fatal(err)
	}
	p = b.Positions["SOL"]
	if p.Quantity != 8_000_000 || p.EntryNotional != 810_000_000_000_000 || p.Margin != 405_000_000_000_000 {
		t.Fatal("weighted increase incorrect")
	}
	if err := b.Command(perp("SELL", "8", 0, true), 30000); err != nil {
		t.Fatal(err)
	}
	if b.Cash != 1_050_000_000_000_000 || b.MarginUsed() != 0 || len(b.Positions) != 0 || !b.Passed() {
		t.Fatal("realized result or collateral release incorrect")
	}
}
func TestShortAndOneWayPositionRules(t *testing.T) {
	b := book(t, "perpetual-long-short-1x")
	if err := b.Command(perp("SELL", "1", 1, false), 0); err != nil {
		t.Fatal(err)
	}
	if err := b.Command(perp("BUY", "1", 1, false), 0); !errors.Is(err, ErrUnsupported) {
		t.Fatal("opposite order reversed instead of requiring explicit close")
	}
	if err := b.Command(perp("SELL", "1", 2, false), 0); !errors.Is(err, ErrUnsupported) {
		t.Fatal("leverage changed while position open")
	}
	if err := b.Command(perp("BUY", "1.000001", 0, true), 0); !errors.Is(err, ErrFunds) {
		t.Fatal("reduce order enlarged position")
	}
	if err := b.Command(perp("BUY", "1", 0, true), 30000); err != nil {
		t.Fatal(err)
	}
	if b.Cash != 995_000_000_000_000 || len(b.Positions) != 0 {
		t.Fatal("short PnL sign incorrect")
	}
}
func TestLiquidationGapUsesOnlyIsolatedCollateral(t *testing.T) {
	for _, side := range []string{"BUY", "SELL"} {
		t.Run(side, func(t *testing.T) {
			b := book(t, "perpetual-isolated-liquidation-5x")
			if err := b.Command(perp(side, "10", 5, false), 0); err != nil {
				t.Fatal(err)
			}
			if err := b.Reconcile(180000); err != nil {
				t.Fatal(err)
			}
			if b.Cash != 800_000_000_000_000 || b.AvailableCash("") != b.Cash || len(b.Positions) != 0 || !b.Passed() {
				t.Fatal("liquidation consumed more than isolated margin")
			}
			last := b.Fills[len(b.Fills)-1]
			wantOffset, wantBackstop := int64(120000), int64(100_000_000_000_000)
			if side == "SELL" {
				wantOffset, wantBackstop = 60000, 50_000_000_000_000
			}
			if last.Reason != "liquidation" || last.OffsetMS != wantOffset || last.PnL != -200_000_000_000_000 || last.Backstop != wantBackstop {
				t.Fatal("gap treatment or historical trigger incorrect")
			}
			if err := b.Reconcile(180000); err != nil {
				t.Fatal(err)
			}
			if len(b.Fills) != 2 {
				t.Fatal("duplicate liquidation")
			}
		})
	}
}
func TestPendingReduceCannotOversubscribePosition(t *testing.T) {
	b := book(t, "perpetual-order-controls")
	if err := b.Command(perp("BUY", "10", 2, false), 0); err != nil {
		t.Fatal(err)
	}
	c := perp("SELL", "8", 0, true)
	c.Type = "LIMIT"
	c.LimitPrice = "120"
	c.TimeInForce = "GTC"
	if err := b.Command(c, 0); err != nil {
		t.Fatal(err)
	}
	if err := b.Command(perp("SELL", "3", 0, true), 0); !errors.Is(err, ErrFunds) {
		t.Fatal("pending reduce quantity reused")
	}
	if err := b.Command(perp("SELL", "2", 0, true), 0); err != nil {
		t.Fatal(err)
	}
	if err := b.Settle(180000); err != nil {
		t.Fatal(err)
	}
	if len(b.Positions) != 0 || b.Cash != 1_020_000_000_000_000 {
		t.Fatal("system settlement failed to supersede pending close")
	}
}
