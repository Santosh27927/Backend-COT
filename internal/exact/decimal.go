// Package exact provides bounded decimal parsing and checked integer arithmetic.
package exact

import (
	"errors"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var ErrNumber = errors.New("invalid or out-of-range decimal")
var decimal = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]{1,6})?$`)

func Parse(value string) (int64, error) {
	if len(value) > 25 || !decimal.MatchString(value) {
		return 0, ErrNumber
	}
	whole, fraction, _ := strings.Cut(value, ".")
	fraction += strings.Repeat("0", 6-len(fraction))
	number, err := strconv.ParseInt(whole+fraction, 10, 64)
	if err != nil {
		return 0, ErrNumber
	}
	return number, nil
}
func Format(value int64, scale int) string {
	n := big.NewInt(value)
	negative := n.Sign() < 0
	n.Abs(n)
	digits := n.String()
	if scale > 0 {
		if len(digits) <= scale {
			digits = strings.Repeat("0", scale+1-len(digits)) + digits
		}
		digits = digits[:len(digits)-scale] + "." + digits[len(digits)-scale:]
	}
	if negative {
		digits = "-" + digits
	}
	return digits
}
func Product(a, b int64) (int64, error) {
	n := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	if !n.IsInt64() {
		return 0, ErrNumber
	}
	return n.Int64(), nil
}
func Add(a, b int64) (int64, error) {
	n := new(big.Int).Add(big.NewInt(a), big.NewInt(b))
	if !n.IsInt64() {
		return 0, ErrNumber
	}
	return n.Int64(), nil
}
func Proportion(value, numerator, denominator int64) (int64, error) {
	if denominator <= 0 || value < 0 || numerator < 0 {
		return 0, ErrNumber
	}
	n := new(big.Int).Mul(big.NewInt(value), big.NewInt(numerator))
	n.Div(n, big.NewInt(denominator))
	if !n.IsInt64() {
		return 0, ErrNumber
	}
	return n.Int64(), nil
}
