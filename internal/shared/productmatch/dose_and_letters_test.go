package productmatch

import "testing"

// The failures this file pins were all found the same way: by reading the
// wrong applied matches that test/corpus records, and asking what arithmetic
// produced each one. Every case below is a real row from a real supplier file.

// A ratio states every figure it carries, in any order.
//
// strengthPattern captures "10/20mg" whole, and parseStrength — whose job is to
// answer with ONE number — kept the head and dropped the 20. The row then
// agreed with the wrong sibling and contradicted the right one:
//
//	alkor plus 10/20mg  ->  applied to الكور بلس 10/40 مجم
//	                        instead of الكور بلس 20/10 مجم
func TestRatioStatesEveryComponent(t *testing.T) {
	for _, tc := range []struct {
		text  string
		want  []float64
		unit  string
		parts int
	}{
		{"alkor plus 10/20mg 14 tab", []float64{10, 20}, "mg", 2},
		{"الكور بلس 20/10 مجم 14 قرص", []float64{20, 10}, "mg", 2},
		{"amlosazide 5/12.5/40 mg 30 f.c. tabs", []float64{5, 12.5, 40}, "mg", 3},
		{"اتاكاند بلس 16/12.5مجم", []float64{16, 12.5}, "mg", 2},
		{"بانادول 500 مجم 24 قرص", []float64{500}, "mg", 1},
	} {
		got := strengthSet(tc.text)
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %v, want %v of unit %s", tc.text, got, tc.want, tc.unit)
			continue
		}
		for _, w := range tc.want {
			found := false
			for _, g := range got {
				if g.unit == tc.unit && g.value == w && g.parts == tc.parts {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: %v missing %g %s (parts %d)", tc.text, got, w, tc.unit, tc.parts)
			}
		}
	}
}

// Two orderings of one combination are one product; two different combinations
// are two products. This is the property the expansion exists to restore.
func TestRatioOrderDoesNotMatter(t *testing.T) {
	row := strengthSet("alkor plus 10/20mg 14 tab")
	same := strengthSet("الكور بلس 20/10 مجم 14 قرص")
	other := strengthSet("الكور بلس 10/40 مجم 14 قرص")

	if agree, comparable := compareStrengths(row, same); !comparable || !agree {
		t.Errorf("10/20 vs 20/10: agree=%v comparable=%v, want the same product", agree, comparable)
	}
	if agree, comparable := compareStrengths(row, other); !comparable || agree {
		t.Errorf("10/20 vs 10/40: agree=%v comparable=%v, want a contradiction", agree, comparable)
	}
}

// "ملجم" is milligrams. Missing from the unit alternation, it matched its first
// two letters as "مل" and the dose was recorded in millilitres — a dimension
// nothing in the catalogue could contradict. "لتر" was unreadable outright.
func TestUnitsThatWereMisreadOrMissing(t *testing.T) {
	for _, tc := range []struct {
		text  string
		value float64
		unit  string
	}{
		{"اتاكاند بلس 32 ملجم", 32, "mg"},
		{"محلول معقم 1 لتر", 1000, "ml"},
		{"بروفين 400 مج اقراص", 400, "mg"},
		{"a-viton 50.000 i.u. 20 caps", 50000, "iu"},
		{"vitamin d3 5.000 iu", 5000, "iu"},
		{"capsin 0.075% cream", 0.075, "%"},
	} {
		got := strengthSet(tc.text)
		found := false
		for _, g := range got {
			if g.unit == tc.unit && g.value == tc.value {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: got %v, want %g %s", tc.text, got, tc.value, tc.unit)
		}
	}
}

// A decimal that is a decimal must stay one. The thousands rule is allowed to
// fire only where an international unit follows it.
func TestThousandsRuleDoesNotEatDecimals(t *testing.T) {
	for _, tc := range []struct {
		text  string
		value float64
		unit  string
	}{
		{"1.500 mg", 1.5, "mg"},
		{"0.075 %", 0.075, "%"},
		{"2.5 مجم", 2.5, "mg"},
	} {
		got := strengthSet(tc.text)
		if len(got) != 1 || got[0].value != tc.value || got[0].unit != tc.unit {
			t.Errorf("%s: got %v, want exactly %g %s", tc.text, got, tc.value, tc.unit)
		}
	}
}

// c and ch each spell two sounds, and folding either one unconditionally merged
// brands that are not related. Every pair below was an applied wrong match.
func TestSkeletonReadsCByContext(t *testing.T) {
	same := [][2]string{
		{"chromax", "كروماكس"},
		{"cisplatin", "سيسبلاتين"},
		{"cefidime", "سيفيديم"},
		{"chlorhexidine", "كلورهيكسيدين"},
		{"calamine", "كالامين"},
		{"claritine", "كلاريتين"},
		{"ciprofloxacin", "سيبروفلوكساسين"},
		{"charcoal", "شاركول"},
		{"panadol", "بانادول"},
		{"augmentin", "اوجمنتين"},
	}
	for _, p := range same {
		if a, b := Skeleton(p[0]), Skeleton(p[1]); a != b {
			t.Errorf("%s (%s) and %s (%s) are the same product and must fold alike",
				p[0], a, p[1], b)
		}
	}

	different := [][2]string{
		{"chromax", "سيرومكس"},
		{"cisplatin", "اوكسابلاتين"},
		{"cefidime", "كيفاديم"},
	}
	for _, p := range different {
		if a, b := Skeleton(p[0]), Skeleton(p[1]); a == b {
			t.Errorf("%s and %s are different products and must not fold to %q",
				p[0], p[1], a)
		}
	}
}

// A pack written as its factors is the pack, not the factors.
//
// "20*10 tabs" is two hundred tablets. Read literally it states a pack of
// twenty and a pack of ten, and the catalogue's 200 contradicts both.
func TestPackMultipliersFold(t *testing.T) {
	for _, tc := range []struct {
		text string
		want float64
	}{
		{"allergyl 4 mg 20*10 tabs", 200},
		{"panadol 20x10 caps", 200},
		{"اليرجيل 4مجم 200 قرص", 200},
		{"اليرجيل 4 مجم 20 قرص", 20},
	} {
		q := readQuantities(tc.text)
		if len(q.counts) != 1 || q.counts[0].value != tc.want {
			t.Errorf("%s: counts=%v, want a single count of %g", tc.text, q.counts, tc.want)
		}
	}
}

// The letter x is also a letter. It is an operator only between two bare
// figures, or the engine would read a brand and a vitamin code as arithmetic.
func TestPackMultipliersLeaveWordsAlone(t *testing.T) {
	for _, text := range []string{"vitax 10 tabs", "b12 x 30 tab", "matrix 30 قرص"} {
		if got := foldPackMultipliers(text); got != text {
			t.Errorf("foldPackMultipliers(%q) = %q, want it unchanged", text, got)
		}
	}
}
