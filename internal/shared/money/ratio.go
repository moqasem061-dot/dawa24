package money

import (
	"math"
	"math/big"
)

// MulRatio returns a*num/den rounded half away from zero, exactly: no float is
// involved at any step. It is how a tax ratio (tax/taxable) or a fractional
// quantity (milli-units/1000) is applied to money.
func (a Amount) MulRatio(num, den int64) (Amount, error) {
	if den == 0 {
		return Zero, ErrOverflow
	}
	p := new(big.Int).Mul(big.NewInt(a.minor), big.NewInt(num))
	d := big.NewInt(den)
	q, r := new(big.Int).QuoRem(p, d, new(big.Int))
	// Round half away from zero: compare 2|r| with |den|.
	r2 := new(big.Int).Abs(r)
	r2.Lsh(r2, 1)
	if r2.Cmp(new(big.Int).Abs(d)) >= 0 {
		if (p.Sign() < 0) != (d.Sign() < 0) {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	if !q.IsInt64() {
		return Zero, ErrOverflow
	}
	return Amount{minor: q.Int64()}, nil
}

// MulQuantity scales by a possibly fractional quantity (stock counted in part
// packs). The quantity is taken to three decimal places, then applied exactly.
func (a Amount) MulQuantity(q float64) (Amount, error) {
	return a.MulRatio(int64(math.Round(q*1000)), 1000)
}

// PercentToBasisPoints converts a percentage held as a float (a NUMERIC(5,2)
// column, a form value) to basis points. The percentage is a rate, not money;
// rounding it to 0.01% here is what keeps the money arithmetic after it exact.
func PercentToBasisPoints(pct float64) int64 {
	return int64(math.Round(pct * 100))
}
