// Package importprogress turns an import's stage and counts into one number.
//
// Every import tool on the platform has a progress bar and, before this, each
// one computed its percentage its own way. The results were not comparable and
// two of them were actively misleading:
//
//   - The admin catalogue import reported progress WITHIN the current stage, so
//     the bar ran 0→100 five times over. An administrator who looked away and
//     back could not tell whether they were watching the first pass or the last.
//   - The saving-products import moved 10 → 15 → 30 → 98 on fixed constants and
//     then sat at 98 for however long the AI tier took, which on a large file is
//     most of the run.
//   - Both, and the vendor import, wrote 100% the moment the background call
//     returned, before the commit that follows it had written anything.
//
// The rules here are the same for all of them:
//
//	One bar, 0–100, across the whole run.
//	Each stage owns a contiguous band, sized by how long that stage really takes.
//	Inside a stage with a count, interpolate across its band.
//	Inside a stage without one, drift toward the band's end and never arrive.
//	100 is written once, by the terminal state, and never inferred.
//
// The drift is the part that matters for the stages that wait on a network. A
// bar that stops moving is read as a hung import, and the honest alternative —
// an indeterminate barber-pole — throws away the information that the run is
// four fifths of the way through its stages. Drifting asymptotically toward the
// band's end says both things at once: still working, and this far along.
package importprogress

import (
	"math"
	"time"
)

// Band is one stage's contiguous share of the overall bar, in percent.
type Band struct {
	Start float64
	End   float64
}

// Valid reports whether the band is a usable range.
func (b Band) Valid() bool { return b.End > b.Start && b.Start >= 0 && b.End <= 100 }

// Width is the band's span in percentage points.
func (b Band) Width() float64 { return b.End - b.Start }

// driftHalfLife is how long an uncounted stage takes to cross half its
// remaining band.
//
// Twelve seconds. Short enough that the bar visibly moves while somebody is
// watching it, long enough that a stage lasting three minutes has not exhausted
// its band by the thirty-second mark. The curve is exponential, so it never
// completes: at one half-life it is 50% through its band, at four 94%, and at no
// point 100%.
const driftHalfLife = 12 * time.Second

// driftCeiling is the share of a band that drift alone may reach.
//
// Ninety-five per cent. A stage that finishes normally reports its own
// completion and the next stage's band takes over; the remaining sliver exists
// so a drifting bar never looks finished when it is not.
const driftCeiling = 0.95

// Percent computes the overall progress for a stage.
//
// current/total interpolate within the band when total is positive. When it is
// not — an AI adjudication whose batch count is not yet known, a database write
// with no row counter — elapsed drives the drift instead.
func Percent(b Band, current, total int, elapsed time.Duration) int {
	if !b.Valid() {
		return clampPercent(b.Start)
	}
	return clampPercent(b.Start + b.Width()*Fraction(current, total, elapsed))
}

// Fraction is how far through its own band a stage has got, 0–1.
func Fraction(current, total int, elapsed time.Duration) float64 {
	if total > 0 {
		if current <= 0 {
			return 0
		}
		return math.Min(float64(current)/float64(total), 1)
	}
	return Drift(elapsed)
}

// Drift is the time-based curve used when a stage cannot count its work.
//
// 1 − 2^(−t/halfLife), scaled by the ceiling. It is monotonic, starts at zero,
// and is bounded strictly below the band's end however long the stage runs.
func Drift(elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	halfLives := float64(elapsed) / float64(driftHalfLife)
	return driftCeiling * (1 - math.Exp2(-halfLives))
}

// clampPercent keeps a computed value inside 0–99.
//
// Ninety-nine, not a hundred: the only thing entitled to say 100 is a run that
// has finished, and it says so by being in a terminal state rather than by its
// arithmetic happening to land there. Every "the import is stuck at 100%"
// report the platform has had came from a bar that reached the end before the
// work did.
func clampPercent(v float64) int {
	if v <= 0 {
		return 0
	}
	if v >= 99 {
		return 99
	}
	return int(v + 0.5)
}

// Complete is the value a terminal state reports.
const Complete = 100
