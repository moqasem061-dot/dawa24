package matcheval

import (
	"math"
)

// ClopperPearsonLower computes the exact lower bound of a binomial proportion confidence interval.
// successes is the number of successes (k), n is the sample size, and confidence is e.g. 0.95.
// It returns a one-sided lower confidence bound (alpha = 1 - confidence).
// Special cases:
// - n <= 0, successes <= 0, or invalid input returns 0.
// - successes >= n returns alpha^(1/n).
func ClopperPearsonLower(successes, n int, confidence float64) float64 {
	if n <= 0 || successes <= 0 || confidence <= 0 || confidence >= 1 {
		return 0
	}
	if successes > n {
		successes = n
	}
	alpha := 1.0 - confidence
	if successes == n {
		return math.Pow(alpha, 1.0/float64(n))
	}

	a := float64(successes)
	b := float64(n - successes + 1)

	return incBetaInverse(alpha, a, b)
}

// Wilson computes the two-sided Wilson score interval for a binomial proportion.
// successes is k, n is total, z is the standard normal quantile (e.g. 1.95996 for 95%).
func Wilson(successes, n int, z float64) (lo, hi float64) {
	if n <= 0 {
		return 0, 0
	}
	k := float64(successes)
	nf := float64(n)
	p := k / nf
	z2 := z * z

	denom := 1.0 + z2/nf
	center := (p + z2/(2.0*nf)) / denom
	factor := (z / denom) * math.Sqrt(p*(1.0-p)/nf+z2/(4.0*nf*nf))

	lo = center - factor
	hi = center + factor

	if lo < 0 {
		lo = 0
	}
	if hi > 1 {
		hi = 1
	}
	return lo, hi
}

// incBeta computes the regularized incomplete beta function I_x(a, b).
func incBeta(x, a, b float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}

	// Symmetry relation when x > (a+1)/(a+b+2)
	if x > (a+1.0)/(a+b+2.0) {
		return 1.0 - incBeta(1.0-x, b, a)
	}

	// Factor in front: exp(a*ln(x) + b*ln(1-x) - lnBeta(a, b)) / a
	lnBeta, _ := math.Lgamma(a)
	lnBetaB, _ := math.Lgamma(b)
	lnBetaAB, _ := math.Lgamma(a + b)
	lnB := lnBeta + lnBetaB - lnBetaAB

	front := math.Exp(a*math.Log(x)+b*math.Log(1.0-x)-lnB) / a

	// Continued fraction: Lentz's method
	cf := betaCF(x, a, b)
	return front * cf
}

// betaCF evaluates the continued fraction for the incomplete beta function.
func betaCF(x, a, b float64) float64 {
	const (
		maxIter = 200
		eps     = 1e-15
		fpMin   = 1e-30
	)

	qab := a + b
	qap := a + 1.0
	qam := a - 1.0

	c := 1.0
	d := 1.0 - qab*x/qap
	if math.Abs(d) < fpMin {
		d = fpMin
	}
	d = 1.0 / d
	h := d

	for m := 1; m <= maxIter; m++ {
		mf := float64(m)
		m2 := 2.0 * mf

		// Even step: d_{2m}
		numer := mf * (b - mf) * x / ((qam + m2) * (a + m2))
		d = 1.0 + numer*d
		if math.Abs(d) < fpMin {
			d = fpMin
		}
		c = 1.0 + numer/c
		if math.Abs(c) < fpMin {
			c = fpMin
		}
		d = 1.0 / d
		h *= d * c

		// Odd step: d_{2m+1}
		numer = -(a + mf) * (qab + mf) * x / ((a + m2) * (qap + m2))
		d = 1.0 + numer*d
		if math.Abs(d) < fpMin {
			d = fpMin
		}
		c = 1.0 + numer/c
		if math.Abs(c) < fpMin {
			c = fpMin
		}
		d = 1.0 / d
		del := d * c
		h *= del

		if math.Abs(del-1.0) < eps {
			break
		}
	}
	return h
}

// incBetaInverse computes x such that I_x(a, b) = p using Newton-Raphson + Bisection.
func incBetaInverse(p, a, b float64) float64 {
	if p <= 0 {
		return 0
	}
	if p >= 1 {
		return 1
	}

	lnBeta, _ := math.Lgamma(a)
	lnBetaB, _ := math.Lgamma(b)
	lnBetaAB, _ := math.Lgamma(a + b)
	lnB := lnBeta + lnBetaB - lnBetaAB

	// Initial guess using Cornish-Fisher expansion or mean
	x := a / (a + b)
	low := 0.0
	high := 1.0

	for iter := 0; iter < 100; iter++ {
		val := incBeta(x, a, b)
		diff := val - p

		if math.Abs(diff) < 1e-15 || (high-low) < 1e-15 {
			return x
		}

		if diff > 0 {
			high = x
		} else {
			low = x
		}

		// Derivative of I_x(a,b) with respect to x:
		// f'(x) = x^(a-1) * (1-x)^(b-1) / Beta(a,b)
		lnDeriv := (a-1.0)*math.Log(x) + (b-1.0)*math.Log(1.0-x) - lnB
		deriv := math.Exp(lnDeriv)

		var nextX float64
		if deriv > 1e-30 {
			nextX = x - diff/deriv
		}

		// If Newton step is out of bounds or step is too small, use bisection
		if nextX <= low || nextX >= high {
			nextX = 0.5 * (low + high)
		}
		x = nextX
	}
	return x
}
