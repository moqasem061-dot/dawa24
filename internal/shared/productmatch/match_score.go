package productmatch

import (
	"strings"
)

// Per-candidate scoring.
//
// The name gets a candidate onto the list; the attributes decide whether it
// stays, and — since the rewrite this comment sits on top of — which of two
// candidates is chosen. That order matters because the name is the least
// reliable field a supplier writes and the dose is the most: "أوجمنتين 1 جم"
// and "أوجمنتين 625" differ by one token out of three and are different
// medicines, while "بانادول اكسترا سعر جديد" and "Panadol Extra" differ in
// every token and are the same one.
//
// What the name may do at all is decided in match_evidence.go, before any of
// this runs. What the attributes may do is decided in discriminate.go, and the
// division is the important part: a candidate that CONTRADICTS something the
// row states is ranked behind every candidate that does not, rather than merely
// scoring a little lower. Subtraction let a large name similarity buy its way
// past a small contradiction, and that is how the wrong strength, the wrong
// tube size and the wrong pack count were applied to real files.

// corroborationFloor is the share of an attribute bonus that survives when the
// names agree not at all, and corroborationFull is the name agreement above
// which corroboration counts in full.
const (
	corroborationFloor = 0.50
	corroborationFull  = 0.50
)

// corroborationFactor discounts attribute agreement when the names barely
// agree, and leaves it alone once they do.
//
// The shape matters more than the numbers. Above half a name in common the
// attributes are corroborating a real candidate and must count for what they
// are worth — clipping them there costs correct matches on every vendor file
// measured. Below it they are the only thing holding the match up, and an
// attribute is a poor thing to hold a match up with: a bottle size, a
// pharmaceutical form and a round figure are each true of a hundred products,
// and together they were worth enough to carry a 20% name agreement past the
// applied threshold.
func corroborationFactor(name float64) float64 {
	if name >= corroborationFull {
		return 1
	}
	return corroborationFloor + (1-corroborationFloor)*(name/corroborationFull)
}

// rate scores one catalogue product against one row.
func (idx *Index) rate(q *query, p *MasterProduct) (scoredProduct, bool) {
	ev := idx.nameEvidenceOf(q, p)
	if !ev.sufficient(q) {
		return scoredProduct{}, false
	}
	name := ev.similarity

	conflicts := idx.conflictsOf(q, p)
	mass := conflictMass(conflicts)

	weight := 0.75
	if !q.strength.known() && !p.strength.known() {
		// When neither side states a strength (cosmetics, OTC, supplies, devices),
		// the product name carries the full identity. Weighting it appropriately
		// prevents certain non-pharmaceutical matches from falling below the cutoff.
		weight = 0.88
	}
	score := name * weight

	// Corroboration is accumulated apart from the score and applied scaled by
	// the name, because an attribute agreeing is not evidence of identity on
	// its own.
	//
	// It used to be added flat, and that is how "ستار فيل غسول نسائى جيل مرطب
	// 200 مل" reached 0.50 against another company's "تيلير غسول نسائي 200 مل":
	// the names agreed on 0.20 and the bottle size, the pharmaceutical form and
	// the figure 200 added 0.32 between them. Every one of those three is true
	// of a hundred products.
	var agreed agreements
	bonus := idx.corroboration(q, p, &agreed)
	score += bonus * corroborationFactor(name)

	// The lifts, and the one rule that governs them: nothing may be lifted over
	// a contradiction.
	//
	// They used to run on a `noConflict` flag that covered the dose, the
	// modifier and the form and nothing else, so a row whose pack count, tube
	// size or identity letter disagreed was still clamped to 0.97 and applied
	// as an exact match. That clamp then defeated the ambiguity check as well,
	// because two candidates both pinned at 0.97 are not "close" — they are
	// equal, and the tie test excluded scores at the ceiling.
	exact := false
	if len(conflicts) == 0 {
		exact = name >= 0.98 || (name >= 0.90 && weight == 0.88)
		switch {
		case exact:
			score = maxF(score, 0.97)
		case name >= 0.88:
			score = maxF(score, name*0.95)
		}
	}

	// What the contradictions leave. Applied last and as a factor, so nothing
	// above it can outrun it: the lifts operate on uncontradicted evidence and
	// this is what remains of that evidence once the row's own attributes have
	// been consulted. See survival.
	evidence := clamp(score)

	return scoredProduct{
		product:   p,
		score:     clamp(evidence * survival(conflicts)),
		evidence:  evidence,
		conflicts: conflicts,
		mass:      mass,
		name:      name,
		agreed:    agreed,
		exact:     exact,
		blind:     ev.distinctHits == 0,
	}, true
}

// agreements is which attributes corroborated a candidate, as a bitmask.
//
// A bitmask rather than the list of Arabic phrases it renders to, because this
// is the hot loop. The scorer used to build a []string and strings.Join it for
// EVERY candidate — several hundred per row, a few hundred million on a large
// import — and then throw all but the five that reach the review screen away.
// The phrases are assembled in describeReason instead, on the handful that are
// actually shown.
type agreements uint8

const (
	agreedDose agreements = 1 << iota
	agreedForm
	agreedMaker
	agreedMolecule
	agreedPrice
)

// describeReason renders why a candidate scored what it did, for the review
// screen.
func (s scoredProduct) describeReason() string {
	parts := make([]string, 0, 8)
	parts = append(parts, "تشابه الاسم "+percent(s.name))
	if s.agreed&agreedDose != 0 {
		parts = append(parts, "تطابق التركيز")
	}
	if s.agreed&agreedForm != 0 {
		parts = append(parts, "تطابق الشكل الصيدلي")
	}
	if s.agreed&agreedMaker != 0 {
		parts = append(parts, "تطابق الشركة")
	}
	if s.agreed&agreedMolecule != 0 {
		parts = append(parts, "تطابق المادة الفعالة")
	}
	if s.agreed&agreedPrice != 0 {
		parts = append(parts, "تطابق سعر الجمهور")
	}
	for _, c := range s.conflicts {
		if label := conflictReason(c.kind); label != "" {
			parts = append(parts, label)
		}
	}
	return strings.Join(parts, " + ")
}

// corroboration is what the attributes AGREEING are worth.
//
// Agreement only. Disagreement is not the negative of this and is not scored
// here — it is a conflict, it orders the candidate behind every candidate that
// does not have one, and discriminate.go owns it.
func (idx *Index) corroboration(q *query, p *MasterProduct, agreed *agreements) float64 {
	var bonus float64

	if agree, comparable := compareStrengths(q.strengths, p.strengths); comparable && agree {
		bonus += doseBonus(q.strengths, p.strengths)
		*agreed |= agreedDose
	}

	// The form. The guard on name similarity is there because catalogue
	// metadata is not always consistent with the catalogue's own names: a
	// product called "فلاى بيبى طبق لوكس + معلقه" is filed under a dosage form
	// that reads as tablets, and crediting an otherwise identical name for its
	// own record's bookkeeping is not evidence about anything.
	if q.formKey != "" && q.formKey == p.formKey {
		bonus += 0.10
		*agreed |= agreedForm
	}

	// The manufacturer corroborates but never disqualifies: supplier files write
	// the agent, the licensee and the parent company interchangeably.
	if q.makerKey != "" && p.makerKey != "" {
		switch {
		case q.makerKey == p.makerKey:
			bonus += 0.10
			*agreed |= agreedMaker
		case strings.Contains(p.makerKey, q.makerKey) || strings.Contains(q.makerKey, p.makerKey):
			bonus += 0.06
		}
	}

	// The molecule, where the file names it.
	if q.sciKey != "" && p.sciKey != "" && q.sciKey == p.sciKey {
		bonus += 0.08
		*agreed |= agreedMolecule
	}

	// The printed price. See evidence_price.go: the strongest field both sides
	// state exactly, and the one this engine used to ignore.
	if pb := priceBonus(q.priceMinor, p.priceMinor); pb > 0 {
		bonus += pb
		*agreed |= agreedPrice
	}

	for i, n := 0, p.sideCount(); i < n; i++ {
		if countsAgree(q.qty.counts, p.sideAt(i).qty.counts) {
			bonus += 0.06
			break
		}
	}
	if q.packSize > 0 && q.packSize == p.packSize {
		bonus += 0.05
	}
	return bonus
}

// countsAgree reports whether the two names counted the same thing and got the
// same answer.
func countsAgree(a, b []countUnit) bool {
	for _, x := range a {
		for _, y := range b {
			if x.class == y.class && x.value == y.value {
				return true
			}
		}
	}
	return false
}

// conflictReason renders a disagreement for the review screen, in the vocabulary
// the vendor reads.
func conflictReason(kind string) string {
	switch kind {
	case "strength":
		return "اختلاف التركيز"
	case "dose_parts":
		return "اختلاف في عدد مكونات التركيز"
	case "modifier":
		return "اختلاف في صنف المنتج داخل نفس العلامة"
	case "modifier_unstated":
		return "الكتالوج يذكر صنفاً داخل العلامة لا يذكره الملف"
	case "letter":
		return "اختلاف في حرف التمييز بعد اسم العلامة"
	case "letter_unstated":
		return "الكتالوج يذكر حرف تمييز لا يذكره الملف"
	case "form":
		return "اختلاف الشكل الصيدلي"
	case "sub_form":
		return "اختلاف نوع المستحضر الموضعي"
	case "count":
		return "اختلاف عدد وحدات العبوة"
	case "figure":
		return "اختلاف الأرقام في الاسم"
	case "pack":
		return "اختلاف حجم العبوة"
	}
	return ""
}

// doseBonus is what an agreeing dose is worth, which depends on what was
// measured.
//
// A milligram is a dose: two products agreeing on 500 mg are very likely the
// same medicine, and that is the strongest corroboration in the row. A
// millilitre is usually a bottle, and "200 مل" is true of a hundred unrelated
// washes — counting it as a dose match is how a 20% name agreement reached the
// applied threshold on a cosmetics file. Percentages sit between: a topical's
// 1% is a real concentration, but round percentages are also common.
func doseBonus(rowDoses, prodDoses []strength) float64 {
	best := 0.0
	for _, a := range rowDoses {
		for _, b := range prodDoses {
			if a.unit != b.unit || !sameStrength(a, b) {
				continue
			}
			switch a.unit {
			case "mg", "iu":
				return 0.16
			case "%", "spf":
				best = maxF(best, 0.12)
			default:
				best = maxF(best, 0.07)
			}
		}
	}
	return best
}

// sameStrength compares two doses, tolerating the rounding a supplier applies
// when they write 0.5g for 500mg.
func sameStrength(a, b strength) bool {
	if a.unit != b.unit {
		return false
	}
	if a.value == b.value {
		return true
	}
	hi, lo := a.value, b.value
	if hi < lo {
		hi, lo = lo, hi
	}
	if lo <= 0 {
		return false
	}
	return (hi-lo)/hi <= 0.01
}

func percent(v float64) string {
	return itoa(int(v*100)) + "%"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
