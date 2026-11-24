package execution

import (
	"encoding/json"
	"errors"
	"github.com/Santosh27927/Backend-COT/simulation/scripted"
	"strings"
	"testing"
)

func book(t *testing.T, id string) *Book {
	t.Helper()
	conditions, err := scripted.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range conditions {
		if c.ID == id {
			b, err := New("test-session", c)
			if err != nil {
				t.Fatal(err)
			}
			if err = b.Reconcile(0); err != nil {
				t.Fatal(err)
			}
			return b
		}
	}
	t.Fatal("missing condition")
	return nil
}
func spot(side, qty string) Command {
	return Command{Product: "spot", Symbol: "XRP", Side: side, Type: "MARKET", Quantity: qty}
}
func TestWholeWalletSpotAndExactSettlement(t *testing.T) {
	b := book(t, "spot-market-buy-sell")
	if err := b.Command(spot("BUY", "500"), 0); err != nil {
		t.Fatal(err)
	}
	if b.Cash != 0 || b.Holdings["XRP"] != 500_000_000 {
		t.Fatal("whole-wallet execution incorrect")
	}
	if err := b.Command(spot("BUY", "1"), 0); !errors.Is(err, ErrFunds) {
		t.Fatal("overspend accepted")
	}
	if err := b.Command(spot("SELL", "100"), 0); err != nil {
		t.Fatal(err)
	}
	if err := b.Settle(180000); err != nil {
		t.Fatal(err)
	}
	if b.Cash != 1_020_000_000_000_000 || b.Holdings["XRP"] != 0 || !b.Passed() {
		t.Fatal("cash or learned objectives incorrect")
	}
	raw, _ := json.Marshal(b.Snapshot(180000))
	if strings.Contains(string(raw), "frames") || strings.Contains(string(raw), "condition") {
		t.Fatal("future prices leaked")
	}
}
func TestReservationsAndChronologicalGTC(t *testing.T) {
	b := book(t, "spot-gtc-price-wait")
	order := spot("BUY", "500")
	order.Type = "LIMIT"
	order.LimitPrice = "1.8"
	order.TimeInForce = "GTC"
	if err := b.Command(order, 0); err != nil {
		t.Fatal(err)
	}
	if b.AvailableCash("") != 100_000_000_000_000 {
		t.Fatal("cash was not reserved")
	}
	if err := b.Command(spot("BUY", "60"), 0); !errors.Is(err, ErrFunds) {
		t.Fatal("reservation reused")
	}
	if err := b.Reconcile(90000); err != nil {
		t.Fatal(err)
	}
	if len(b.Fills) != 1 || b.Fills[0].OffsetMS != 90000 || b.Cash != 100_000_000_000_000 {
		t.Fatal("GTC used latest rather than first eligible scripted frame")
	}
	raw, _ := json.Marshal(b)
	var recovered Book
	if err := json.Unmarshal(raw, &recovered); err != nil {
		t.Fatal(err)
	}
	if err := recovered.Reconcile(120000); err != nil {
		t.Fatal(err)
	}
	if len(recovered.Fills) != 1 {
		t.Fatal("recovery duplicated a fill")
	}
}
func TestImmediatePoliciesAndStopLimit(t *testing.T) {
	b := book(t, "spot-ioc-fok-immediate")
	for _, tif := range []string{"IOC", "FOK"} {
		o := spot("BUY", "1")
		o.Type = "LIMIT"
		o.TimeInForce = tif
		o.LimitPrice = "1.5"
		if err := b.Command(o, 0); err != nil {
			t.Fatal(err)
		}
		if b.Orders[len(b.Orders)-1].Status != "canceled" {
			t.Fatal("ineligible immediate order rested")
		}
		o.LimitPrice = "2"
		if err := b.Command(o, 0); err != nil {
			t.Fatal(err)
		}
	}
	if len(b.Fills) != 2 || b.Holdings["XRP"] != 2_000_000 || b.AvailableCash("") != 996_000_000_000_000 {
		t.Fatal("unrestricted IOC/FOK outcomes incorrect")
	}
	stop := book(t, "spot-stop-limit-trigger")
	o := spot("BUY", "1")
	o.Type = "STOP_LIMIT"
	o.TimeInForce = "GTC"
	o.StopPrice = "2.1"
	o.LimitPrice = "2.2"
	if err := stop.Command(o, 0); err != nil {
		t.Fatal(err)
	}
	if err := stop.Reconcile(30000); err != nil {
		t.Fatal(err)
	}
	if len(stop.Fills) != 1 || stop.Fills[0].Price != 2_100_000 || !stop.Goals["stop_triggered"] {
		t.Fatal("stop/limit activation incorrect")
	}
}
func TestRejectMalformedAmountsAndOverselling(t *testing.T) {
	b := book(t, "spot-market-buy-sell")
	for _, value := range []string{"0", "-1", "1e3", "0.0000001", "99999999999999999999"} {
		if err := b.Command(spot("BUY", value), 0); err == nil {
			t.Fatal("invalid amount accepted")
		}
	}
	if err := b.Command(spot("SELL", "1"), 0); !errors.Is(err, ErrFunds) {
		t.Fatal("unowned asset sold")
	}
	if len(b.Orders) != 0 {
		t.Fatal("validation failures created accepted orders")
	}
}

func TestDeadlineCancelsBeforeFinalPriceMakesLimitEligible(t *testing.T) {
	b := book(t, "spot-gtc-price-wait")
	b.Condition.Frames[len(b.Condition.Frames)-1].Prices["XRP"] = "1.5"
	c := spot("BUY", "1")
	c.Type = "LIMIT"
	c.TimeInForce = "GTC"
	c.LimitPrice = "1.5"
	if err := b.Command(c, 0); err != nil {
		t.Fatal(err)
	}
	if err := b.Settle(180000); err != nil {
		t.Fatal(err)
	}
	if len(b.Fills) != 0 || b.Orders[0].Status != "canceled" || b.Cash != InitialCash || b.Goals["gtc_filled"] {
		t.Fatal("pending order filled at the deadline instead of canceling")
	}
}
