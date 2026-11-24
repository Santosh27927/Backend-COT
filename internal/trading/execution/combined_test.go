package execution

import (
	"errors"
	"testing"
)

func TestMixedWalletCannotReuseCashOrMargin(t *testing.T) {
	b := book(t, "combined-wallet-margin-accounting")
	c := spot("BUY", "5")
	c.Symbol = "SOL"
	if err := b.Command(c, 0); err != nil {
		t.Fatal(err)
	}
	if err := b.Command(perp("BUY", "10", 2, false), 0); err != nil {
		t.Fatal(err)
	}
	if b.Cash != 500_000_000_000_000 || b.MarginUsed() != 500_000_000_000_000 || b.AvailableCash("") != 0 {
		t.Fatal("cash/margin separation failed")
	}
	if err := b.Command(spot("BUY", "0.000001"), 0); !errors.Is(err, ErrFunds) {
		t.Fatal("isolated collateral was reused for spot")
	}
	if err := b.Command(perp("BUY", "1", 2, false), 0); !errors.Is(err, ErrFunds) {
		t.Fatal("committed margin was reused")
	}
	c = spot("SELL", "6")
	c.Symbol = "SOL"
	if err := b.Command(c, 0); !errors.Is(err, ErrFunds) {
		t.Fatal("perpetual quantity counted as owned spot")
	}
	if err := b.Settle(180000); err != nil {
		t.Fatal(err)
	}
	if b.Cash != 1_037_500_000_000_000 || b.MarginUsed() != 0 || b.Holdings["SOL"] != 0 || len(b.Positions) != 0 {
		t.Fatal("mixed settlement result wrong")
	}
}
func TestMixedLiquidationPreservesSpotAssets(t *testing.T) {
	b := book(t, "combined-session-completion")
	c := spot("BUY", "5")
	c.Symbol = "SOL"
	if err := b.Command(c, 0); err != nil {
		t.Fatal(err)
	}
	if err := b.Command(perp("BUY", "25", 5, false), 0); err != nil {
		t.Fatal(err)
	}
	if err := b.Reconcile(130000); err != nil {
		t.Fatal(err)
	}
	if b.Holdings["SOL"] != 5_000_000 || b.Cash != 0 || len(b.Positions) != 0 {
		t.Fatal("perpetual liquidation consumed owned spot")
	}
	if err := b.Settle(180000); err != nil {
		t.Fatal(err)
	}
	if b.Cash != 500_000_000_000_000 || !b.Passed() {
		t.Fatal("mixed final report or objectives incorrect")
	}
}
