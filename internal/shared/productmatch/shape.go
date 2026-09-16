package productmatch

import (
	"regexp"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Value shape.
//
// sheet.ColumnProfile counts things; this turns the counts into the ratios and
// flags the detectors below actually reason about, computed once per column so
// twenty-nine field rules can read them without recomputing anything.

// shape is one column's measurements, normalised to rates.
type shape struct {
	p *sheet.ColumnProfile

	fill     float64
	numeric  float64
	integer  float64
	decimal  float64
	unique   float64
	wordy    float64
	url      float64
	boolean  float64
	dated    float64
	serial   float64
	digits   float64
	gtin     float64
	arabic   float64
	latin    float64
	percent  float64
	zero     float64
	negative float64
	leadZero float64

	median, p10, p90, minV, maxV float64
	// lo02 and hi98 bound the values with the extremes trimmed. A single
	// mistyped cell — one real supplier file carries 46223 in a column of
	// discount percentages — must not decide what the other two hundred and
	// twenty-nine values obviously are.
	lo02, hi98 float64
	spread     float64
	// pctBand is the share of numbers inside 0..100, which is the single most
	// useful discriminator in a pharmaceutical price list: everything a vendor
	// writes as a percentage lives there and most prices do not stay there.
	pctBand float64
	// halfStep is the share of numbers landing on a whole or half unit. Egyptian
	// discount lists are written as 32, 26.5, 38.5 and almost never as 32.17.
	halfStep float64

	distinct int
	avgRunes float64
	avgWords float64

	// Pattern rates, measured over the distinct sample rather than every row.
	concentration float64
	bonus         float64
	codeish       float64
	packish       float64
}

// newShape derives the rates of one sealed profile.
func newShape(p *sheet.ColumnProfile) *shape {
	s := &shape{p: p}
	if p == nil {
		return s
	}
	s.fill = p.FillRate()
	s.numeric = p.Rate(p.Numeric)
	s.integer = p.Rate(p.Integer)
	s.decimal = p.Rate(p.OneDp + p.TwoDp + p.DeepDp)
	s.unique = p.UniqueRate()
	s.wordy = p.Rate(p.Wordy)
	s.url = p.Rate(p.URL)
	s.boolean = p.Rate(p.Boolean)
	s.dated = p.Rate(p.Dated)
	s.serial = p.Rate(p.Serial)
	s.digits = p.Rate(p.Digits)
	s.gtin = p.Rate(p.GTIN)
	s.arabic = p.Rate(p.Arabic)
	s.latin = p.Rate(p.Latin)
	s.percent = p.Rate(p.Percent)
	s.zero = p.Rate(p.Zero)
	s.negative = p.Rate(p.Negative)
	s.leadZero = p.Rate(p.LeadZero)

	s.median = p.Median()
	s.p10, s.p90 = p.Quantile(0.1), p.Quantile(0.9)
	s.minV, s.maxV = p.Min(), p.Max()
	s.lo02, s.hi98 = p.Quantile(0.02), p.Quantile(0.98)
	s.spread = p.Spread()
	s.pctBand = p.InRange(0, 100)
	s.halfStep = halfStepRate(p.Values())

	s.distinct = p.Distinct
	s.avgRunes = p.AvgRunes
	s.avgWords = p.AvgWords

	s.concentration = sampleRate(p.Sample, looksConcentration)
	s.bonus = sampleRate(p.Sample, looksBonus)
	s.codeish = sampleRate(p.Sample, looksCode)
	s.packish = sampleRate(p.Sample, looksPackSize)
	return s
}

// empty reports whether the column carried nothing worth judging.
func (s *shape) empty() bool { return s.p == nil || s.p.Filled == 0 }

// percentBand reports whether the numbers look like percentages: all inside
// 0..100, tightly clustered, and written to at most one decimal place.
//
// The tightness matters. A cheap-medicines price column can also sit entirely
// under 100, but its cheapest item is a small fraction of its dearest, while a
// discount column's values all crowd between about twenty and forty.
func (s *shape) percentBand() bool {
	return s.numeric >= 0.9 &&
		s.pctBand >= 0.95 &&
		s.hi98 <= 100 &&
		s.lo02 >= 0 &&
		s.spread <= 5 &&
		s.halfStep >= 0.85
}

// moneyBand reports whether the numbers look like prices: non-negative, spread
// over at least an order of magnitude or reaching well past a hundred, and not
// crowded into the shape of a percentage.
func (s *shape) moneyBand() bool {
	if s.numeric < 0.85 || s.negative > 0.02 {
		return false
	}
	return s.spread >= 3 || s.hi98 > 150 || s.p.TwoDp*3 >= s.p.Numeric
}

// countBand reports whether the numbers look like a stock count: whole,
// non-negative, and free of the fractional pricing tail.
func (s *shape) countBand() bool {
	return s.numeric >= 0.9 && s.integer >= 0.9 && s.negative <= 0.02
}

// halfStepRate is the share of values that are a whole number or a half.
func halfStepRate(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	n := 0
	for _, v := range values {
		doubled := v * 2
		if diff := doubled - float64(int64(doubled)); diff < 0.001 || diff > 0.999 {
			n++
		}
	}
	return float64(n) / float64(len(values))
}

// sampleRate is the share of a column's distinct sample matching a predicate.
//
// Measuring over the distinct sample rather than every row is deliberate: what
// these predicates test is whether a *vocabulary* is present, and a unit column
// whose five distinct values are all real units is certain regardless of how
// the rows are distributed across them.
func sampleRate(sample []string, ok func(string) bool) float64 {
	if len(sample) == 0 {
		return 0
	}
	n := 0
	for _, v := range sample {
		if ok(v) {
			n++
		}
	}
	return float64(n) / float64(len(sample))
}

var (
	// concentrationPattern is a number welded to a dose unit: "500mg", "5 مجم",
	// "1g", "60ml", "SPF50". Written this way in every supplier file there is.
	concentrationPattern = regexp.MustCompile(`(?i)\d+(?:[./]\d+)?\s*(?:mg|mcg|gm|g|ml|l|iu|%|spf|ملجم|مليجرام|مجم|مكجم|جرام|جم|مل|وحده|وحدة)\b`)
	// bonusPattern is a quantity offer: "1+1", "10 + 2", "12+3".
	bonusPattern = regexp.MustCompile(`^\s*\d{1,3}\s*\+\s*\d{1,3}\s*$`)
)

func looksConcentration(v string) bool {
	v = sheet.NormalizeDigits(v)
	if len([]rune(v)) > 30 {
		return false
	}
	return concentrationPattern.MatchString(v)
}

func looksBonus(v string) bool {
	return bonusPattern.MatchString(sheet.NormalizeDigits(v))
}

// looksCode reports whether a value has the shape of an identifier: one token,
// short, and carrying at least one digit.
func looksCode(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || strings.ContainsAny(v, " \t") {
		return false
	}
	n := len([]rune(v))
	if n < 2 || n > 24 {
		return false
	}
	sc := sheet.Profile(v)
	return sc.Digit > 0 && sc.Arabic == 0
}

// looksPackSize reports whether a value is a small whole count of the kind
// printed on a carton.
func looksPackSize(v string) bool {
	n, err := sheet.CoerceInt(v)
	return err == nil && n >= 1 && n <= 1000
}

// clamp bounds x to [0,1].
func clamp(x float64) float64 {
	switch {
	case x < 0:
		return 0
	case x > 1:
		return 1
	default:
		return x
	}
}

// ramp maps x from [lo,hi] onto [0,1], flat outside.
func ramp(x, lo, hi float64) float64 {
	if hi <= lo {
		return 0
	}
	return clamp((x - lo) / (hi - lo))
}
