package smartorder

import "testing"

func TestParseQuantityPlainNumbers(t *testing.T) {
	cases := map[string]float64{
		"2":    2,
		" 3 ":  3,
		"12.0": 12,
		"0":    0,
		"1000": 1000,
		"1,20": 120, // thousands separator stripped
	}
	for raw, want := range cases {
		got := ParseQuantity(raw)
		if got.Qty == nil {
			t.Errorf("%q: expected %g, got no quantity (%s)", raw, want, got.Note)
			continue
		}
		if *got.Qty != want {
			t.Errorf("%q: expected %g, got %g", raw, want, *got.Qty)
		}
		if got.Note != "" {
			t.Errorf("%q: unexpected note %q", raw, got.Note)
		}
	}
}

func TestParseQuantityArabicIndicDigits(t *testing.T) {
	// Egyptian exports carry both numeral sets, sometimes in one column.
	for raw, want := range map[string]float64{"٥": 5, "١٢": 12, "۳": 3} {
		got := ParseQuantity(raw)
		if got.Qty == nil || *got.Qty != want {
			t.Errorf("%q: expected %g, got %v (%s)", raw, want, got.Qty, got.Note)
		}
	}
}

func TestParseQuantityBlankIsNotAnError(t *testing.T) {
	// A blank cell means "use the default", which is normal and unremarkable.
	for _, raw := range []string{"", "   ", "\t"} {
		got := ParseQuantity(raw)
		if got.Qty != nil {
			t.Errorf("%q: expected no quantity", raw)
		}
		if got.Note != "" {
			t.Errorf("%q: a blank cell should not produce a note, got %q", raw, got.Note)
		}
	}
}

// The spec calls this one out by name. "2-3" must never silently become 2.
func TestParseQuantityRangeIsRefused(t *testing.T) {
	for _, raw := range []string{"2-3", "2 - 3", "٢-٣", "2 to 3"} {
		got := ParseQuantity(raw)
		if got.Qty != nil {
			t.Errorf("%q: a range must not be resolved to %g", raw, *got.Qty)
		}
		if got.Note == "" {
			t.Errorf("%q: expected a note explaining the refusal", raw)
		}
	}
}

func TestParseQuantityWithUnitKeepsTheNumberAndFlagsIt(t *testing.T) {
	// "5 علبة" is usable — the number is unambiguous — but the unit may not be
	// the supplier's unit, so the buyer is told.
	got := ParseQuantity("5 علبة")
	if got.Qty == nil || *got.Qty != 5 {
		t.Fatalf("expected 5, got %v (%s)", got.Qty, got.Note)
	}
	if got.Note == "" {
		t.Fatal("expected a note recording that a unit was ignored")
	}
}

func TestParseQuantityNegativeIsRefused(t *testing.T) {
	got := ParseQuantity("-1")
	if got.Qty != nil {
		t.Fatalf("a negative quantity must be refused, got %g", *got.Qty)
	}
	if got.Note == "" {
		t.Fatal("expected a note")
	}
}

func TestParseQuantityFractionalIsKeptButFlagged(t *testing.T) {
	got := ParseQuantity("2.5")
	if got.Qty == nil || *got.Qty != 2.5 {
		t.Fatalf("expected 2.5 to be kept, got %v", got.Qty)
	}
	if got.Note == "" {
		t.Fatal("a fractional quantity should be flagged, not silently rounded")
	}
}

func TestParseQuantityGarbageIsRefused(t *testing.T) {
	for _, raw := range []string{"abc", "الصنف", "n/a", "-"} {
		got := ParseQuantity(raw)
		if got.Qty != nil {
			t.Errorf("%q: expected no quantity, got %g", raw, *got.Qty)
		}
		if got.Note == "" {
			t.Errorf("%q: expected a note", raw)
		}
	}
}

func TestDefaultQuantityOnlyFillsGaps(t *testing.T) {
	// FR-004 scenario 2: a default of 5 must not rewrite rows that already
	// carry a quantity.
	two := 2.0
	cfg := &Config{DefaultQuantity: 5}
	lines := []*Line{
		{ImportedQty: &two}, // has its own
		{ImportedQty: nil},  // blank
	}
	ApplyQuantities(cfg, lines)

	if lines[0].EffectiveQty != 2 {
		t.Errorf("a row with its own quantity must keep it, got %g", lines[0].EffectiveQty)
	}
	if lines[1].EffectiveQty != 5 {
		t.Errorf("a blank row must take the default, got %g", lines[1].EffectiveQty)
	}
}

func TestDefaultQuantityZeroLeavesLinesExcluded(t *testing.T) {
	// FR-004 scenario 1: the default is 0, so blank rows load at 0 and are
	// excluded from the order until the buyer raises them.
	cfg := &Config{DefaultQuantity: 0}
	lines := []*Line{{ImportedQty: nil}}
	ApplyQuantities(cfg, lines)

	if lines[0].EffectiveQty != 0 {
		t.Fatalf("expected 0, got %g", lines[0].EffectiveQty)
	}
	if lines[0].Outcome != OutcomeZeroQty {
		t.Fatalf("a zero-quantity line must be visibly excluded, got %s", lines[0].Outcome)
	}
}

func TestEditedQuantityBeatsImportedAndDefault(t *testing.T) {
	imported, edited := 2.0, 9.0
	cfg := &Config{DefaultQuantity: 5}
	l := &Line{ImportedQty: &imported, EditedQty: &edited}
	ApplyQuantities(cfg, []*Line{l})

	if l.EffectiveQty != 9 {
		t.Fatalf("the buyer's edit must win, got %g", l.EffectiveQty)
	}
}
