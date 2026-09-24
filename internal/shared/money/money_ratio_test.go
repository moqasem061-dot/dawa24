package money

import "testing"

func TestMulRatio(t *testing.T) {
	cases := []struct {
		minor, num, den, want int64
	}{
		{100000, 29, 100, 29000},   // 1000.00 * 0.29, exact where float gave 289.99
		{1001, 1, 2, 501},          // 10.01 / 2 = 5.005 -> 5.01 (half away)
		{-1001, 1, 2, -501},        // symmetric for negatives
		{1000, 1, 3, 333},          // 10.00 / 3
		{1000, 2, 3, 667},          // 6.666.. -> 6.67
		{12345, 1500, 1000, 18518}, // 1.5 units * 123.45 = 185.175 -> 185.18
		{50000, 1400, 10000, 7000}, // 14% tax on 500.00
	}
	for _, c := range cases {
		got, err := FromMinor(c.minor).MulRatio(c.num, c.den)
		if err != nil || got.Minor() != c.want {
			t.Errorf("%d*%d/%d = %d, %v; want %d", c.minor, c.num, c.den, got.Minor(), err, c.want)
		}
	}
	if _, err := FromMinor(1).MulRatio(1, 0); err == nil {
		t.Error("zero denominator must fail")
	}
}

func TestPercentToBasisPoints(t *testing.T) {
	for pct, want := range map[float64]int64{12.5: 1250, 0.29: 29, 14: 1400, 33.33: 3333} {
		if got := PercentToBasisPoints(pct); got != want {
			t.Errorf("PercentToBasisPoints(%v) = %d, want %d", pct, got, want)
		}
	}
}
