package productmatch

// Contradiction is not a smaller score.
//
// The scorer used to answer one question with one number: how alike are these?
// Everything went into it — the name, the dose, the form, the figures — as
// additions and subtractions, and the sum decided. That works while the
// candidates are unrelated. It fails on the case that matters, which is a
// catalogue holding the right answer AND three of its siblings:
//
//	row        اماريل ام 2/500 مجم 30 قرص
//	chosen     اماريل 2مجم 30 قرص            name 1.00, dose "agrees", applied
//	correct    اماريل ام 2/500 مجم 30 قرص    name 0.97
//
// The chosen row is a different medicine — Amaryl M is glimepiride with
// metformin — and it won because it is a shorter name that the row's words
// cover completely, while the thing separating them (a second dose component,
// a letter) was worth a fraction of a point against a name similarity worth
// three quarters of one.
//
// So contradiction is taken out of the sum. A candidate that disagrees with
// something the row STATES is ranked behind every candidate that does not,
// whatever the names look like; the score only orders candidates that
// contradict the row equally. That is the whole idea of this file, and it is
// what makes "the right size is in the catalogue and the wrong one was chosen"
// a case the engine can no longer produce: the wrong size contradicts a stated
// figure and the right size does not, so the right size ranks first even when
// the wrong one is spelled more like the row.
//
// Nothing here is a veto. A conflicting candidate can still be the best there
// is, and it is still offered — as a candidate for review, at a score that says
// what it is. Refusing to rank it first is different from refusing to show it.

// conflict is one thing a row and a candidate disagree about.
type conflict struct {
	kind string
	// mass is how much identity the disagreement carries. It orders candidates
	// against each other and scales the penalty; it is not a probability.
	mass float64
}

// Conflict masses.
//
// A mass is read two ways and both are deliberate. It orders candidates against
// each other — less contradiction first, always, whatever the names look like —
// and it says how much of a candidate's score survives the contradiction: the
// score is multiplied by (1 − mass), so a mass of 1 leaves nothing and a mass
// of 0.45 leaves a bit over half.
//
// That second reading is why the numbers sit where they do. Everything at or
// above massLetter takes a candidate below the applied threshold on its own, so
// a row whose ONLY plausible answer contradicts its strength, its brand
// extension, its form or its identity letter reaches a person rather than a
// price list. Below that line — a pack count, a stray figure, a cream against
// an ointment — the contradiction still decides which candidate wins, but it
// does not by itself refuse the only candidate there is.
//
// Ordered by what the disagreement proves rather than by how visible it is.
const (
	massDose      = 1.00 // 500 mg is not 1 g
	massDoseParts = 0.95 // 16 mg is not 16/12.5 mg — a combination is another product
	massModifier  = 0.95 // بانادول is not بانادول اكسترا
	// massModifierUnstated is that read the other way round: the CATALOGUE
	// names a line extension and the row says nothing.
	//
	// Same argument as massLetterUnstated, and the same evidence. A row reading
	// "ديجيكوماج اقراص" against "ديجيكوماج 20 قرص مضغ", or
	// "جليفلوزميت 25/1000 مج اقراص" against "جليفلوزاميت اكس ار 25/1000مجم 30
	// قرص", is one statement and one silence: the supplier wrote the brand and
	// the catalogue spelled out which line it is. Where the catalogue really
	// does hold a plain sibling, that sibling contradicts nothing and ranking
	// gives it the row — which is what makes it safe to let the extended one
	// through when it is the only member of its family.
	//
	// Higher than the letter's, because a line-extension WORD — بلس, اكسترا,
	// ريتارد — is a stronger statement than a single letter, and still below
	// the line at which a contradiction refuses a candidate on its own.
	massModifierUnstated = 0.55
	massForm             = 0.90 // a syrup is not a tablet
	massLetter           = 0.80 // بتنوفيت ان is not بتنوفيت سي
	// massLetterUnstated is the same disagreement read the other way round: the
	// CATALOGUE carries a line letter and the row says nothing.
	//
	// That is not the same evidence. A row reading بتنوفيت ان and a product
	// reading بتنوفيت سي are two statements that contradict; a row reading
	// توسيفان against توسيفان-ن is one statement and one silence, and silence
	// is how most suppliers write a family with only one member in it. Charged
	// at the full mass it refused a hundred and forty rows across nine live
	// files whose price, form and brand all agreed with the only توسيفان there
	// is.
	//
	// Below the line that refuses on its own, so the letterless sibling still
	// wins wherever the catalogue actually holds one — that is what ranking is
	// for — and the lettered product is applied only when it is the whole
	// family.
	massLetterUnstated = 0.45
	massCount          = 0.70 // 20 tablets is not 200
	massFigure         = 0.60 // a figure that named nothing, and differs anyway
	massSubForm        = 0.55 // a cream is not an ointment
	massPack           = 0.45 // the pack-size column disagreeing with the name
)

// survival is the share of a candidate's evidence that survives everything it
// contradicts.
//
// Multiplicative rather than subtractive, and that is the change that made the
// reported score mean something. Subtraction let a large name similarity buy
// its way past a contradiction — 0.97 minus a fifth is still applied — and it
// made the number printed on the review screen an arithmetic residue rather
// than a statement about the product. A factor cannot be outrun: whatever the
// names look like, a candidate that contradicts the dose keeps a fifth of
// whatever it had.
//
// Independent factors, because the conflicts are largely independent evidence:
// a product that differs in strength AND in form is further away than one that
// differs in either.
func survival(cs []conflict) float64 {
	f := 1.0
	for _, c := range cs {
		f *= 1 - c.mass
	}
	return f
}

// conflictsOf lists everything a candidate contradicts about a row.
//
// Only attributes the row actually states are compared. Silence is not
// disagreement: a pharmacy writes "بانادول" and means the only بانادول there
// is, and holding that against a catalogue entry which spells out its strength
// would refuse the commonest correct match in the file.
func (idx *Index) conflictsOf(q *query, p *MasterProduct) []conflict {
	var out []conflict

	switch doseVerdict(q, p) {
	case dosesConflict:
		out = append(out, conflict{"strength", massDose})
	case dosesPartsDiffer:
		out = append(out, conflict{"dose_parts", massDoseParts})
	}

	// Everything below is asked of each spelling separately and answered by the
	// spelling that agrees, if any exists.
	//
	// A spelling that says NOTHING about an attribute does not get a vote on
	// it, and that rule is load-bearing. The first version of this took the
	// whole side with the fewest conflicts, and the English half of a catalogue
	// record routinely states less than the Arabic half — "advochol 10mg 14
	// f.c. tabs" parsed to no pack count at all — so the uninformative side won
	// every comparison and switched the count check off for the entire
	// catalogue. Silence is not agreement.
	if conflictsOnEverySide(p, func(f *nameFacts) bool {
		return modifierSetsConflict(q.mods, f.mods, q.rawName, p.NameAR+" "+p.NameEN)
	}) {
		if len(q.mods) == 0 {
			out = append(out, conflict{"modifier_unstated", massModifierUnstated})
		} else {
			out = append(out, conflict{"modifier", massModifier})
		}
	}
	if conflictsOnEverySide(p, func(f *nameFacts) bool {
		return !markSetsAgree(q.marks, f.marks)
	}) {
		if len(q.marks) == 0 {
			out = append(out, conflict{"letter_unstated", massLetterUnstated})
		} else {
			out = append(out, conflict{"letter", massLetter})
		}
	}

	switch {
	case q.formKey == "":
		// The row named no form. It cannot be contradicted about one.
	case conflictsOnStatingSide(p,
		func(f *nameFacts) bool { return p.formOf(f) != "" },
		func(f *nameFacts) bool { return formClass(q.formKey) != formClass(p.formOf(f)) }):
		out = append(out, conflict{"form", massForm})
	case q.subForm != "" && conflictsOnStatingSide(p,
		func(f *nameFacts) bool { return f.subForm != "" },
		func(f *nameFacts) bool { return q.subForm != f.subForm }):
		out = append(out, conflict{"sub_form", massSubForm})
	}

	countConflict := conflictsOnStatingSide(p,
		func(f *nameFacts) bool { return sharesCountClass(q.qty.counts, f.qty.counts) },
		func(f *nameFacts) bool { return countsDisagree(q.qty.counts, f.qty.counts) })
	switch {
	case countConflict:
		out = append(out, conflict{"count", massCount})
	case conflictsOnStatingSide(p,
		func(f *nameFacts) bool { return len(f.qty.residual) > 0 },
		func(f *nameFacts) bool { return residualsDisagree(q.qty.residual, f.qty.residual) }):
		out = append(out, conflict{"figure", massFigure})
	}

	if q.packSize > 0 && p.packSize > 0 && q.packSize != p.packSize && !countConflict {
		// Only when the names did not already say so, or the same difference
		// would be charged twice.
		out = append(out, conflict{"pack", massPack})
	}
	return out
}

// The three answers the dose comparison can give.
const (
	dosesAgree = iota
	dosesPartsDiffer
	dosesConflict
)

// doseVerdict compares the row's doses against each spelling of the candidate
// and reports the best answer any of them gives.
//
// A spelling that measures nothing the row also measured does not vote, which
// is the same rule the other attributes follow: silence is missing information,
// not agreement and not contradiction.
func doseVerdict(q *query, p *MasterProduct) int {
	verdict := dosesAgree
	compared := false
	for i, n := 0, p.sideCount(); i < n; i++ {
		doses := p.dosesOf(p.sideAt(i))
		agree, comparable := compareStrengths(q.strengths, doses)
		if !comparable {
			continue
		}
		if agree {
			// This spelling agrees. Whether it agrees in the same NUMBER of
			// components is a separate question, and the one the combination
			// families turn on: اتاكاند is 32 مجم and اتاكاند بلس is 32/25 مجم.
			if !doseComponentsDiffer(q.strengths, doses) {
				return dosesAgree
			}
			verdict = dosesPartsDiffer
			compared = true
			continue
		}
		if !compared {
			verdict = dosesConflict
		}
		compared = true
	}
	if !compared {
		return dosesAgree
	}
	return verdict
}

// conflictsOnEverySide reports a disagreement no spelling of the candidate
// escapes.
//
// Used for the checks that are symmetric — a modifier or a letter present on
// one side and absent on the other is a difference either way round — where an
// empty side is a real answer rather than a missing one.
func conflictsOnEverySide(p *MasterProduct, differs func(*nameFacts) bool) bool {
	n := p.sideCount()
	for i := 0; i < n; i++ {
		if !differs(p.sideAt(i)) {
			return false
		}
	}
	return n > 0
}

// conflictsOnStatingSide reports a disagreement among the spellings that have
// something to say, ignoring the ones that do not.
//
// Used for the checks where absence is silence: a catalogue name that omits the
// pack count has not contradicted a row that states one.
func conflictsOnStatingSide(p *MasterProduct, states, differs func(*nameFacts) bool) bool {
	stated := false
	for i, n := 0, p.sideCount(); i < n; i++ {
		f := p.sideAt(i)
		if !states(f) {
			continue
		}
		stated = true
		if !differs(f) {
			return false
		}
	}
	return stated
}

// sharesCountClass reports whether the two names counted the same kind of thing,
// which is the precondition for their counts being able to disagree at all.
func sharesCountClass(a, b []countUnit) bool {
	for _, x := range a {
		for _, y := range b {
			if x.class == y.class {
				return true
			}
		}
	}
	return false
}

// formClass groups the dosage forms that a row and a catalogue entry may name
// differently while meaning the same shelf.
//
// Tablets and capsules are one class because the person typing an order uses
// اقراص and كبسول interchangeably for any solid oral form. Creams, ointments,
// gels and washes are one class because Egyptian names call the same bottle
// لوسيون and غسول — and what genuinely separates a cream from a shampoo is
// compared underneath, as a sub-form.
//
// Everything else stays distinct. A syrup is not an injection, and a sachet of
// granules is not a strip of tablets.
func formClass(key string) string {
	switch key {
	case "tablet", "capsule":
		return "solid_oral"
	case "topical", "wash":
		return "external"
	}
	return key
}
