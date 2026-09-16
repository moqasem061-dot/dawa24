package productmatch

// Diagnostics.
//
// The labelled benchmarks in test/corpus need to say WHY a wrong match was
// wrong — whether the two products differ by a strength, a pack figure, a
// line-extension word or a dosage form — and every one of those comparisons
// already exists in this package, unexported, because the scorer is the only
// thing that needed them.
//
// Exporting a thin, read-only layer is better than the alternatives: copying
// the comparisons into the benchmark makes the benchmark measure a second
// implementation, and moving them out of this package puts the scorer's own
// vocabulary somewhere it cannot see.
//
// It is two entry points rather than a dozen, deliberately. Every exported
// symbol here is unreachable from the running program by construction, so each
// one is a line on the dead-code ledger a reviewer has to read past.

// Explanation is what the engine can say about a row and two candidates for it.
type Explanation struct {
	// Conflicts names everything the row disagrees with the CHOSEN candidate
	// about. Empty means the chosen candidate contradicts nothing the row
	// states.
	Conflicts []string
	// The rest compare the two CANDIDATES with each other rather than with the
	// row. They answer "what separates the product that was chosen from the
	// product that was correct", which is the question a benchmark of wrong
	// matches asks.
	StrengthDiffers bool
	NumbersDiffer   bool
	ModifierDiffers bool
	FormDiffers     bool
	// ExtraWord is a word one candidate's name carries and the other's does
	// not, which is what separates them when no structured attribute does.
	ExtraWord string
	// GotName and WantName are the two candidates' catalogue labels.
	GotName  string
	WantName string
}

// Explain compares a row against one candidate, and that candidate against
// another.
//
// A product id the index does not hold yields the zero value for the
// comparisons that needed it rather than an error: a diagnostic that fails on
// stale input stops being run.
func Explain(idx *Index, row *Row, gotID, wantID int64) Explanation {
	var e Explanation
	if idx == nil {
		return e
	}
	got, hasGot := idx.byID[gotID]
	want, hasWant := idx.byID[wantID]

	if hasGot {
		e.GotName = catalogueLabel(got)
		if row != nil {
			for _, c := range idx.conflictsOf(idx.newQuery(row), got) {
				e.Conflicts = append(e.Conflicts, c.kind)
			}
		}
	}
	if hasWant {
		e.WantName = catalogueLabel(want)
	}
	if !hasGot || !hasWant {
		return e
	}

	e.StrengthDiffers = !doseSetsEqual(
		strengthSet(productText(got)), strengthSet(productText(want)))
	e.NumbersDiffer = !sameQuantities(got.factsAR.qty, want.factsAR.qty) ||
		!sameQuantities(got.factsEN.qty, want.factsEN.qty)
	_, e.ModifierDiffers = modifierSetsDiffer(
		got.mods, want.mods, productText(got), productText(want))
	e.FormDiffers = formClass(got.formKey) != "" && formClass(want.formKey) != "" &&
		formClass(got.formKey) != formClass(want.formKey)
	e.ExtraWord = firstDifferingWord(got, want)
	return e
}

// ConflictsWith names what a row disagrees with one catalogue product about.
//
// Separate from Explain because the benchmark that measures FALSE conflicts —
// how often a rule fires against the product a row genuinely is — has no second
// candidate to compare against, and passing the same id twice would read as a
// mistake rather than as the question being asked.
func ConflictsWith(idx *Index, row *Row, productID int64) []string {
	if idx == nil || row == nil {
		return nil
	}
	p, ok := idx.byID[productID]
	if !ok {
		return nil
	}
	cs := idx.conflictsOf(idx.newQuery(row), p)
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.kind)
	}
	return out
}

func catalogueLabel(p *MasterProduct) string {
	if p.NameAR != "" {
		return p.NameAR
	}
	return p.NameEN
}

func productText(p *MasterProduct) string {
	return p.NameAR + " " + p.NameEN + " " + p.Concentration
}

// sameQuantities compares what two names counted and what they left unexplained.
func sameQuantities(a, b quantities) bool {
	if len(a.counts) != len(b.counts) || len(a.residual) != len(b.residual) {
		return false
	}
	for i := range a.counts {
		if a.counts[i] != b.counts[i] {
			return false
		}
	}
	for i := range a.residual {
		if a.residual[i] != b.residual[i] {
			return false
		}
	}
	return true
}

// firstDifferingWord returns a word one name carries and the other does not.
func firstDifferingWord(got, want *MasterProduct) string {
	a := append(append([]string{}, got.coreAR...), got.coreEN...)
	b := append(append([]string{}, want.coreAR...), want.coreEN...)
	if w := firstMissing(b, a); w != "" {
		return w
	}
	return firstMissing(a, b)
}

func firstMissing(from, in []string) string {
	for _, w := range from {
		found := false
		for _, o := range in {
			if o == w {
				found = true
				break
			}
		}
		if !found {
			return w
		}
	}
	return ""
}
