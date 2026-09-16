package productmatch

import (
	"fmt"
	"math"
	"sort"
)

// Cross-column consistency.
//
// The detectors judge each column alone, and alone a price and a discount
// percentage can be genuinely indistinguishable: a file of cheap medicines
// prices at 22, 35, 45 and discounts at 24, 32, 39. Nothing inside either
// column separates them.
//
// The arithmetic between the columns does. If one column times one minus
// another over a hundred equals a third, for nine rows out of ten, then those
// three columns are the public price, the discount rate and the net price —
// and no header, however wrong, can outweigh that. This is the strongest
// evidence the engine has, and it is the only evidence that can rebind a
// column the resolver had already settled.

// NumericGrid holds the sampled data rows as numbers, aligned by column, with
// NaN standing for a cell that held nothing readable.
//
// It is separate from the per-column profiles because those drop unparseable
// cells and so lose the row alignment that these checks depend on.
type NumericGrid struct {
	Rows  [][]float64
	Width int
}

// relationSupport is the share of comparable rows an arithmetic relation must
// satisfy before it is believed. Real files carry rounding, a hand-edited row
// and the odd blank, so demanding every row would find nothing.
const relationSupport = 0.85

// minRelationRows is how many rows must be comparable at all. Three rows
// agreeing is a coincidence; twenty-five is a rule.
const minRelationRows = 25

// relationTolerance is how far the computed net may sit from the stated net,
// relative to the stated net. Two percent absorbs piastre rounding without
// admitting a relation that merely correlates.
const relationTolerance = 0.02

// Relation is an arithmetic identity found between three columns.
type Relation struct {
	Base    int     `json:"base"`
	Rate    int     `json:"rate"`
	Net     int     `json:"net"`
	Support float64 `json:"support"`
	// Amount is true when the middle column is a money discount rather than a
	// percentage.
	Amount bool `json:"amount"`
}

// FindPriceRelation looks for base × (1 − rate/100) = net, or base − rate = net,
// across the numeric columns of the sample.
//
// Only columns that are mostly numeric are considered, and no more than eight
// of them, which bounds the search at a few hundred triples over a few hundred
// rows — microseconds, and the same answer every time.
func FindPriceRelation(grid NumericGrid, profiles []*shape) (Relation, bool) {
	cols := numericColumns(profiles)
	if len(cols) < 3 {
		return Relation{}, false
	}

	best, found := Relation{}, false
	for _, base := range cols {
		for _, rate := range cols {
			if rate == base {
				continue
			}
			for _, net := range cols {
				if net == base || net == rate {
					continue
				}
				if rel, ok := testRelation(grid, base, rate, net, false); ok && rel.Support > best.Support {
					best, found = rel, true
				}
				if rel, ok := testRelation(grid, base, rate, net, true); ok && rel.Support > best.Support {
					best, found = rel, true
				}
			}
		}
	}
	return best, found
}

// numericColumns lists the columns worth testing, most numeric first, capped.
func numericColumns(profiles []*shape) []int {
	type scored struct {
		col  int
		rate float64
	}
	var candidates []scored
	for i, s := range profiles {
		if s == nil || s.empty() || s.numeric < 0.8 || s.p.Numeric < minRelationRows {
			continue
		}
		candidates = append(candidates, scored{col: i, rate: s.numeric})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].rate != candidates[j].rate {
			return candidates[i].rate > candidates[j].rate
		}
		return candidates[i].col < candidates[j].col
	})
	if len(candidates) > 8 {
		candidates = candidates[:8]
	}
	out := make([]int, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.col)
	}
	sort.Ints(out)
	return out
}

// testRelation measures how often one triple satisfies the identity.
func testRelation(grid NumericGrid, base, rate, net int, amount bool) (Relation, bool) {
	comparable, agree := 0, 0
	for _, row := range grid.Rows {
		if base >= len(row) || rate >= len(row) || net >= len(row) {
			continue
		}
		b, r, n := row[base], row[rate], row[net]
		if math.IsNaN(b) || math.IsNaN(r) || math.IsNaN(n) || b <= 0 || n <= 0 {
			continue
		}
		if !amount && (r < 0 || r > 100) {
			return Relation{}, false // not a percentage anywhere in this column
		}
		comparable++

		var want float64
		if amount {
			want = b - r
		} else {
			want = b * (1 - r/100)
		}
		if want <= 0 {
			continue
		}
		if math.Abs(want-n)/n <= relationTolerance {
			agree++
		}
	}
	if comparable < minRelationRows {
		return Relation{}, false
	}
	support := float64(agree) / float64(comparable)
	if support < relationSupport {
		return Relation{}, false
	}
	return Relation{Base: base, Rate: rate, Net: net, Support: support, Amount: amount}, true
}

// ApplyRelation rebinds the three columns of a proven arithmetic identity.
//
// It overrides whatever the header said, because the arithmetic is not an
// opinion. Every rebinding is recorded as a note so the vendor can see the
// engine changed its mind and why — silently overriding a header would be the
// same failure as silently trusting one.
func ApplyRelation(m *Mapping, rel Relation) {
	rateField := FieldDiscountPct
	rateLabel := "نسبة الخصم"
	if rel.Amount {
		rateField, rateLabel = FieldDiscountAmt, "قيمة الخصم"
	}

	// Each rebind runs; the results are collected afterwards so short-circuit
	// evaluation cannot skip one.
	movedBase := rebind(m, rel.Base, FieldPublicPrice)
	movedRate := rebind(m, rel.Rate, rateField)
	movedNet := rebind(m, rel.Net, FieldNetPrice)
	changed := movedBase || movedRate || movedNet

	why := fmt.Sprintf(
		"تم التحقق حسابياً من %d%% من الصفوف: «%s» × (بعد خصم «%s») = «%s».",
		int(rel.Support*100),
		headerOf(m, rel.Base), rateLabel, headerOf(m, rel.Net))
	if rel.Amount {
		why = fmt.Sprintf(
			"تم التحقق حسابياً من %d%% من الصفوف: «%s» ناقص «%s» = «%s».",
			int(rel.Support*100), headerOf(m, rel.Base), rateLabel, headerOf(m, rel.Net))
	}

	for _, col := range []int{rel.Base, rel.Rate, rel.Net} {
		if col < len(m.Columns) {
			m.Columns[col].Why = append(m.Columns[col].Why, why)
			m.Columns[col].Score = 1
			m.Columns[col].Confidence = ConfidenceCertain
		}
	}
	if changed {
		m.Notes = append(m.Notes, Note{Severity: SeverityInfo, Message: why})
	}
}

// rebind moves a column onto a field, freeing whatever held either before.
func rebind(m *Mapping, col int, f Field) bool {
	if col < 0 || col >= len(m.Columns) {
		return false
	}
	if m.Columns[col].Field == f {
		return false
	}
	// Release the field's previous column and the column's previous field.
	if prev, ok := m.ByField[f]; ok && prev != col {
		m.Columns[prev].Field = ""
		m.Columns[prev].Confidence = ""
	}
	if old := m.Columns[col].Field; old != "" {
		delete(m.ByField, old)
	}
	m.Columns[col].Field = f
	m.ByField[f] = col
	return true
}

func headerOf(m *Mapping, col int) string {
	if col < 0 || col >= len(m.Columns) {
		return ""
	}
	if h := m.Columns[col].Header; h != "" {
		return h
	}
	return fmt.Sprintf("العمود %d", col+1)
}

// CheckOrdering verifies the inequalities a price list must satisfy and reports
// the ones it does not.
//
// A cost above the selling price, or a net above the public price, is almost
// always two columns swapped. Catching it here means the vendor is asked before
// the import rather than after their margin has been published inverted.
func CheckOrdering(m *Mapping, grid NumericGrid) []Conflict {
	var out []Conflict
	type rule struct {
		lower, upper Field
		message      string
	}
	rules := []rule{
		{FieldCostPrice, FieldPublicPrice, "سعر التكلفة أعلى من سعر الجمهور في %d%% من الصفوف — تحقق من ترتيب العمودين."},
		{FieldNetPrice, FieldPublicPrice, "السعر الصافي أعلى من سعر الجمهور في %d%% من الصفوف — تحقق من ترتيب العمودين."},
		{FieldPrice, FieldPublicPrice, "سعر البيع للصيدلية أعلى من سعر الجمهور في %d%% من الصفوف — تحقق من ترتيب العمودين."},
	}

	for _, r := range rules {
		lo, okLo := m.ByField[r.lower]
		hi, okHi := m.ByField[r.upper]
		if !okLo || !okHi {
			continue
		}
		comparable, violated := 0, 0
		for _, row := range grid.Rows {
			if lo >= len(row) || hi >= len(row) {
				continue
			}
			a, b := row[lo], row[hi]
			if math.IsNaN(a) || math.IsNaN(b) || a <= 0 || b <= 0 {
				continue
			}
			comparable++
			if a > b*1.001 {
				violated++
			}
		}
		if comparable < minRelationRows {
			continue
		}
		if share := float64(violated) / float64(comparable); share >= 0.5 {
			out = append(out, Conflict{
				Kind:     ConflictInconsistent,
				Field:    r.lower,
				Column:   lo,
				Severity: SeverityWarning,
				Message:  fmt.Sprintf(r.message, int(share*100)),
			})
		}
	}
	return out
}

// CheckMissingWith reports only the absences that matter to this importer.
//
// A pharmacy's shopping list has no price column and never will; telling the
// buyer their items "will be imported at zero and stay hidden" is a warning
// about a field their file is not supposed to contain, and a wizard that cries
// wolf on every run teaches people to click past the one warning that mattered.
func CheckMissingWith(m *Mapping, fields *FieldSet) []Conflict {
	var out []Conflict

	if !m.Has(FieldName) && !m.Has(FieldSKU) && !m.Has(FieldBarcode) {
		out = append(out, Conflict{
			Kind:     ConflictMissing,
			Severity: SeverityError,
			Column:   -1,
			Message: "لا يوجد عمود لاسم الصنف ولا للكود ولا للباركود. لا يمكن التعرف على الأصناف " +
				"بدون واحد منها على الأقل.",
		})
	}
	if fields.Allows(FieldPublicPrice) &&
		!m.Has(FieldPublicPrice) && !m.Has(FieldPrice) && !m.Has(FieldNetPrice) {
		out = append(out, Conflict{
			Kind:     ConflictMissing,
			Field:    FieldPublicPrice,
			Severity: SeverityWarning,
			Column:   -1,
			Message: "لم يتم العثور على أي عمود أسعار. سيتم استيراد الأصناف بسعر صفر " +
				"ولن تظهر للصيدليات حتى تُسعّرها يدوياً.",
		})
	}
	if fields.Allows(FieldQuantity) && !m.Has(FieldQuantity) {
		out = append(out, Conflict{
			Kind:     ConflictMissing,
			Field:    FieldQuantity,
			Severity: SeverityWarning,
			Column:   -1,
			Message: "لم يتم العثور على عمود للكمية. سيتم الاحتفاظ بالأرصدة الحالية كما هي " +
				"ولن يتم تحديث المخزون من هذا الملف.",
		})
	}
	return out
}
