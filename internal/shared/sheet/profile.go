package sheet

import (
	"math"
	"sort"
	"strings"
)

// Column profiling.
//
// A header is a claim; the values under it are the evidence. This is the
// evidence, gathered once per column and with no idea what a product is.
//
// It matters because supplier headers lie constantly, and not out of malice.
// One real distributor labels its discount column "القائمة" — "the list" —
// because that is what the sales team calls the discount list. Another labels
// it "المرجح", another "جملة", another "مندوب". Nothing in those words says
// percentage. What says percentage is that every value is between 9 and 59, has
// at most one decimal place, and sits beside a column whose values run from 9
// to 3,400. Read the header alone and the column becomes a price; read the
// values and it cannot be anything else.

// valueCap bounds the numeric sample kept for quantiles. Percentiles over two
// thousand values are as good as over two hundred thousand, and the bound keeps
// profiling a fifty-column sheet cheap.
const valueCap = 2048

// distinctCap bounds the distinct-value set kept per column. Enough to
// recognise a vocabulary — twenty dosage forms, a dozen units — without holding
// a copy of a name column.
const distinctCap = 256

// ColumnProfile is everything measurable about one column's sampled values.
type ColumnProfile struct {
	Index     int    `json:"index"`
	Header    string `json:"header"`
	HeaderKey string `json:"header_key"`

	// Rows is how many data rows were offered, Filled how many held anything.
	Rows   int `json:"rows"`
	Filled int `json:"filled"`

	// Distinct counts distinct non-empty values up to distinctCap; Truncated
	// says the real count is higher.
	Distinct          int      `json:"distinct"`
	DistinctTruncated bool     `json:"distinct_truncated"`
	Sample            []string `json:"sample,omitempty"`

	// Numeric and its refinements count cells that parsed as numbers.
	Numeric   int  `json:"numeric"`
	Integer   int  `json:"integer"`
	Negative  int  `json:"negative"`
	Zero      int  `json:"zero"`
	Percent   int  `json:"percent"`
	OneDp     int  `json:"one_dp"`
	TwoDp     int  `json:"two_dp"`
	DeepDp    int  `json:"deep_dp"`
	LeadZero  int  `json:"lead_zero"`
	Monotonic bool `json:"monotonic"`

	// Digit-string shape, which is what separates a barcode from a price.
	Digits    int         `json:"digits"`
	GTIN      int         `json:"gtin"`
	DigitLens map[int]int `json:"digit_lens,omitempty"`

	// Text shape.
	Arabic   int     `json:"arabic"`
	Latin    int     `json:"latin"`
	Wordy    int     `json:"wordy"`
	URL      int     `json:"url"`
	Boolean  int     `json:"boolean"`
	Dated    int     `json:"dated"`
	Serial   int     `json:"serial"`
	MinRunes int     `json:"min_runes"`
	MaxRunes int     `json:"max_runes"`
	AvgRunes float64 `json:"avg_runes"`
	AvgWords float64 `json:"avg_words"`

	// values holds the sampled numbers, sorted lazily for quantiles.
	values []float64
	sorted bool
	// seen backs the distinct count without retaining every string.
	seen map[string]struct{}
	// lastNum tracks the running monotonicity check.
	lastNum   float64
	monoBroke bool
	runeSum   int
	wordSum   int
}

// NewColumnProfile starts a profile for one column.
func NewColumnProfile(index int, header string) *ColumnProfile {
	return &ColumnProfile{
		Index:     index,
		Header:    CleanCell(header),
		HeaderKey: NormalizeKey(header),
		DigitLens: map[int]int{},
		seen:      make(map[string]struct{}, distinctCap),
		MinRunes:  math.MaxInt32,
		lastNum:   math.Inf(-1),
	}
}

// Observe folds one cell into the profile.
func (p *ColumnProfile) Observe(raw string) {
	p.Rows++
	v := CleanCell(raw)
	if v == "" {
		return
	}
	p.Filled++

	p.observeDistinct(v)
	p.observeShape(v)
	p.observeNumber(v)
	p.observeDigits(v)
}

func (p *ColumnProfile) observeDistinct(v string) {
	key := NormalizeKey(v)
	if key == "" {
		key = v
	}
	if _, ok := p.seen[key]; ok {
		return
	}
	if len(p.seen) >= distinctCap {
		p.DistinctTruncated = true
		return
	}
	p.seen[key] = struct{}{}
	p.Distinct = len(p.seen)
	if len(p.Sample) < 32 {
		p.Sample = append(p.Sample, v)
	}
}

func (p *ColumnProfile) observeShape(v string) {
	runes := []rune(v)
	n := len(runes)
	p.runeSum += n
	if n < p.MinRunes {
		p.MinRunes = n
	}
	if n > p.MaxRunes {
		p.MaxRunes = n
	}
	words := len(strings.Fields(v))
	p.wordSum += words

	sc := Profile(v)
	switch {
	case sc.Arabic > sc.Latin && sc.Arabic > 0:
		p.Arabic++
	case sc.Latin > 0:
		p.Latin++
	}
	// "Wordy" is the shape of a name: several words, mostly letters. A code has
	// one token; a price has none.
	if words >= 2 && sc.Letters() >= 6 {
		p.Wordy++
	}
	lower := strings.ToLower(v)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "www.") {
		p.URL++
	}
	if isBooleanish(lower) {
		p.Boolean++
	}
	res, dErr := CoerceDate(v)
	switch {
	case dErr != nil:
	case looksDated(v):
		p.Dated++
	case res.FromSerial:
		p.Serial++
	}
}

func (p *ColumnProfile) observeNumber(v string) {
	d, err := Coerce(v)
	if err != nil {
		p.monoBroke = true
		return
	}
	p.Numeric++
	if d.Percent {
		p.Percent++
	}
	switch {
	case d.Scale == 0:
		p.Integer++
	case d.Scale == 1:
		p.OneDp++
	case d.Scale == 2:
		p.TwoDp++
	default:
		p.DeepDp++
	}
	switch {
	case d.Float < 0:
		p.Negative++
	case d.Float == 0:
		p.Zero++
	}
	// A leading zero means the value is an identifier the file is trying to
	// preserve, not a quantity: "007431" is a code, 7431 is a number.
	trimmed := strings.TrimSpace(NormalizeDigits(v))
	if len(trimmed) > 1 && trimmed[0] == '0' && trimmed[1] != '.' {
		p.LeadZero++
	}

	if len(p.values) < valueCap {
		p.values = append(p.values, d.Float)
		p.sorted = false
	}
	if d.Float <= p.lastNum {
		p.monoBroke = true
	}
	p.lastNum = d.Float
}

func (p *ColumnProfile) observeDigits(v string) {
	digits := DigitsOnly(v)
	if digits == "" || len(digits) != len([]rune(strings.TrimSpace(v))) {
		return // the cell holds more than digits
	}
	p.Digits++
	p.DigitLens[len(digits)]++
	if ValidGTIN(digits) {
		p.GTIN++
	}
}

// Seal finalises the derived averages. Call it once after the last Observe.
func (p *ColumnProfile) Seal() {
	if p.Filled == 0 {
		p.MinRunes = 0
		return
	}
	p.AvgRunes = float64(p.runeSum) / float64(p.Filled)
	p.AvgWords = float64(p.wordSum) / float64(p.Filled)
	// Monotonic only means something over a run long enough to be a sequence.
	p.Monotonic = !p.monoBroke && p.Numeric >= 5 && p.Numeric == p.Filled
}

// Rate is n as a share of the filled cells, or zero when the column is empty.
func (p *ColumnProfile) Rate(n int) float64 {
	if p.Filled == 0 {
		return 0
	}
	return float64(n) / float64(p.Filled)
}

// FillRate is the share of sampled rows that held anything.
func (p *ColumnProfile) FillRate() float64 {
	if p.Rows == 0 {
		return 0
	}
	return float64(p.Filled) / float64(p.Rows)
}

// UniqueRate is how close the column is to being a key.
func (p *ColumnProfile) UniqueRate() float64 {
	if p.Filled == 0 {
		return 0
	}
	if p.DistinctTruncated {
		return 1
	}
	return float64(p.Distinct) / float64(p.Filled)
}

// Quantile returns the value at fraction q of the sampled numbers.
func (p *ColumnProfile) Quantile(q float64) float64 {
	if len(p.values) == 0 {
		return 0
	}
	p.ensureSorted()
	idx := int(q * float64(len(p.values)-1))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(p.values) {
		idx = len(p.values) - 1
	}
	return p.values[idx]
}

// Median is the middle sampled number.
func (p *ColumnProfile) Median() float64 { return p.Quantile(0.5) }

// Min and Max bound the sampled numbers.
func (p *ColumnProfile) Min() float64 { return p.Quantile(0) }
func (p *ColumnProfile) Max() float64 { return p.Quantile(1) }

// InRange is the share of sampled numbers falling inside [lo, hi].
func (p *ColumnProfile) InRange(lo, hi float64) float64 {
	if len(p.values) == 0 {
		return 0
	}
	n := 0
	for _, v := range p.values {
		if v >= lo && v <= hi {
			n++
		}
	}
	return float64(n) / float64(len(p.values))
}

// Spread is the ratio of the 90th to the 10th percentile, which separates a
// price column — where the cheapest item is a hundredth of the dearest — from a
// discount column, where every value sits inside a narrow band.
func (p *ColumnProfile) Spread() float64 {
	lo, hi := p.Quantile(0.1), p.Quantile(0.9)
	if lo <= 0 {
		lo = 0.5
	}
	if hi <= 0 {
		return 1
	}
	return hi / lo
}

// Values exposes the sampled numbers in file order, for the cross-column checks
// that compare one column against another row by row.
func (p *ColumnProfile) Values() []float64 { return p.values }

// DominantDigitLen is the digit length shared by most digit-only cells, and how
// dominant it is. A barcode column is 13 at ~1.0; an item-code column is spread
// across three or four lengths.
func (p *ColumnProfile) DominantDigitLen() (length int, share float64) {
	if p.Digits == 0 {
		return 0, 0
	}
	best, bestN := 0, 0
	for l, n := range p.DigitLens {
		if n > bestN || (n == bestN && l < best) {
			best, bestN = l, n
		}
	}
	return best, float64(bestN) / float64(p.Digits)
}

func (p *ColumnProfile) ensureSorted() {
	if p.sorted {
		return
	}
	sort.Float64s(p.values)
	p.sorted = true
}

func isBooleanish(lower string) bool {
	switch NormalizeKey(lower) {
	case "yes", "no", "true", "false", "y", "n", "0", "1",
		"نعم", "لا", "مفعل", "معطل", "نشط", "غيرنشط", "متاح", "غيرمتاح":
		return true
	}
	return false
}

// looksDated rejects the bare integers CoerceDate accepts as Excel serials.
// Those are counted separately: a quantity column whose values happen to sit
// around 40,000 must not read as a column of dates in 2009 just because the
// arithmetic works, but a genuine serial-date column has to be recognisable
// when the header says so.
func looksDated(v string) bool {
	return strings.ContainsAny(v, "/-. ") || Profile(v).Letters() > 0
}
