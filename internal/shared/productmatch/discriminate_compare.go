package productmatch

// The individual comparisons a contradiction is made of.
//
// discriminate.go says what a disagreement IS and what it costs a candidate.
// This file is the arithmetic underneath: how two dose sets, two identity
// letters, two packaging counts and two stray figures are actually compared,
// and how two candidates are told apart when nothing the row states separates
// them.
//
// Split out because the two read at different levels and AGENTS.md caps a file
// at four hundred lines. Every function here is called only from there.

// conflictMass sums what a candidate contradicts.
func conflictMass(cs []conflict) float64 {
	var total float64
	for _, c := range cs {
		total += c.mass
	}
	return total
}

// doseComponentsDiffer reports whether two dose sets describe combinations of
// different width on a unit they both use.
//
// اتاكاند is 32 مجم and اتاكاند بلس is 32/25 مجم: the doses AGREE — 32 is in
// both — and the products are different, and this is the only thing that says
// so. It is asked per unit and only for units both sides state, so a row
// silent about a strength is never contradicted by one that spells it out.
func doseComponentsDiffer(a, b []strength) bool {
	for unit, wa := range widestPerUnit(a) {
		if wb, both := widestPerUnit(b)[unit]; both && wa != wb {
			return true
		}
	}
	return false
}

// widestPerUnit is the largest ratio a dose set states in each unit.
func widestPerUnit(set []strength) map[string]int {
	out := make(map[string]int, 2)
	for _, s := range set {
		parts := s.parts
		if parts < 1 {
			parts = 1
		}
		if parts > out[s.unit] {
			out[s.unit] = parts
		}
	}
	return out
}

// markSetsAgree compares the identity letters two names carry.
//
// Set equality, like the modifier check and for the same reason: the letter one
// side carries and the other does not IS the product. بتنوفيت against بتنوفيت
// ان is not a terser way of writing the same thing.
func markSetsAgree(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

// countsDisagree compares the packaging counts two names state.
//
// Per class, and only where both sides counted the same thing. A row saying "30
// قرص" against a catalogue entry that states no count at all has not been
// contradicted — the catalogue frequently omits it — but against one saying "20
// قرص" it has.
func countsDisagree(a, b []countUnit) bool {
	for _, x := range a {
		for _, y := range b {
			if x.class != y.class {
				continue
			}
			if x.value == y.value {
				return false // this class agrees; that is enough
			}
		}
	}
	// No agreeing pair. Disagreement only where some class is present on both.
	for _, x := range a {
		for _, y := range b {
			if x.class == y.class {
				return true
			}
		}
	}
	return false
}

// residualsDisagree compares the figures that named nothing.
//
// Weaker evidence than a count, and treated as such: an unlabelled figure could
// be a year, a batch, a price the supplier appended. But when BOTH names carry
// one and they are not the same figure, something differs — "دوركو ايف 3" and
// "دوركو ايف 6" are two razors and nothing else in either line says so.
//
// Silence on either side is not disagreement, which is what stops a catalogue
// entry that omits the pack count from contradicting a row that states it.
func residualsDisagree(a, b []float64) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return false
			}
		}
	}
	return true
}

// separated reports whether anything the ROW states picks one of two candidates
// over the other.
//
// This is the ambiguity test, and it asks a different question from "are their
// scores close". Two catalogue entries scoring 0.97 and 0.96 are not
// distinguishable by a hundredth of a point that came from a spelling; two
// entries scoring 0.97 and 0.80 are not distinguishable either if the whole
// difference between them is a word the row never mentioned. What settles it is
// evidence: does the row state an attribute that one of them agrees with and
// the other does not?
//
// When the answer is no, the honest result is "these two fit equally well",
// whatever the scores say — and that is a question for a person, not a coin
// toss made at four decimal places.
func (idx *Index) separated(q *query, a, b *MasterProduct) bool {
	if a == nil || b == nil {
		return true
	}
	ca, cb := idx.conflictsOf(q, a), idx.conflictsOf(q, b)
	if len(ca) != len(cb) {
		return true
	}
	if conflictMass(ca) != conflictMass(cb) {
		return true
	}
	// Neither contradicts the row, or both contradict it equally. The remaining
	// evidence is the row's own words: one candidate accounting for more of
	// them, letter for letter, is a real separation — a fold is not.
	ea, eb := idx.exactWordHits(q, a), idx.exactWordHits(q, b)
	return ea != eb
}

// exactWordHits counts the row's distinctive words the candidate carries
// spelled exactly the same way.
//
// The variant fold exists so "ابيكوبريد" can meet "ابيكوبرايد", and it earns
// its keep. It also makes سيفازون and سيفوزون — two different cephalosporins —
// the same key, and when both are in the catalogue and the row names one of
// them exactly, the fold is what lets the other one tie. Counting exact hits
// separately is how the tie is broken toward the spelling the supplier actually
// wrote.
func (idx *Index) exactWordHits(q *query, p *MasterProduct) int {
	hits := 0
	for _, side := range [][]string{p.coreAR, p.coreEN} {
		n := 0
		for _, t := range side {
			slot, ok := q.pos[t]
			if ok && q.distinct[slot] {
				n++
			}
		}
		if n > hits {
			hits = n
		}
	}
	return hits
}
