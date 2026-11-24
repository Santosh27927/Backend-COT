package exact

import (
	"math"
	"testing"
)

func TestExactUnitsAndOverflow(t *testing.T) {
	q, _ := Parse("0.000001")
	p, _ := Parse("0.000001")
	value, err := Product(q, p)
	if err != nil || Format(value, 12) != "0.000000000001" {
		t.Fatal("lost exact product precision")
	}
	if _, err = Product(math.MaxInt64, 2); err == nil {
		t.Fatal("overflow accepted")
	}
	if Format(math.MinInt64, 0) != "-9223372036854775808" {
		t.Fatal("signed formatting overflow")
	}
}
