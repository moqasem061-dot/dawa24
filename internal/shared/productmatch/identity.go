package productmatch

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Refusing a match the model was confident about.
//
// The failure this file exists to prevent is specific and was reported from
// live use: two products that share a couple of words being matched as one.
// Egyptian pharmacy catalogues are full of families that differ by a single
// word — بانادول / بانادول اكسترا / بانادول نايت / بانادول ادفانس, ديكلوفين /
// ديكلوفين بلس / ديكلوفين فاست — and a model reading "بانادول 24 قرص" against
// "بانادول اكسترا 24 قرص" sees five characters of difference and four words of
// agreement. It is a different medicine.
//
// The prompt says all of this. The prompt is not enough, and it never will be:
// an instruction is a tendency, and what a pharmacy receives should not rest on
// one. So every answer is re-checked here against the catalogue's own record
// before it is written.
//
// The guards can afford to be strict because of what failure means. This runs
// only on AI-proposed matches, and a refused match leaves the line exactly as
// the deterministic engine left it — reported honestly as unmatched or needing
// review. A false refusal costs one line of manual work. A false acceptance
// ships the wrong drug.

// MatchConflict is why a proposed match was refused. Its zero value means the
// match survived every check.
type MatchConflict struct {
	Kind   string // "", "strength", "modifier", "form", "evidence"
	Detail string // short Arabic explanation, for the audit trail
}

// None reports whether the match is allowed.
func (c MatchConflict) None() bool { return c.Kind == "" }

// IdentityConflict reports whether a row and a catalogue product cannot be the
// same product, on evidence a model is not permitted to overrule.
//
// The checks run cheapest-first and stop at the first conflict, because the
// first one found is the one worth telling an operator about.
func (idx *Index) IdentityConflict(row *Row, productID int64) MatchConflict {
	if idx == nil || row == nil {
		return MatchConflict{}
	}
	p, ok := idx.byID[productID]
	if !ok {
		// Not in the index at all. The caller's window check should have caught
		// this; refusing here too costs nothing and closes the gap.
		return MatchConflict{Kind: "evidence", Detail: "المنتج غير موجود في الكتالوج"}
	}

	rowText := row.Name + " " + row.NameEN + " " + row.Concentration
	prodText := p.NameAR + " " + p.NameEN + " " + p.Concentration

	if strengthsConflict(rowText, prodText) {
		return MatchConflict{Kind: "strength", Detail: "اختلاف التركيز"}
	}
	if mod, ok := modifierConflict(row, p); ok {
		return MatchConflict{Kind: "modifier", Detail: "اختلاف في صنف المنتج داخل نفس العلامة: " + mod}
	}
	if formsConflict(row, p) {
		return MatchConflict{Kind: "form", Detail: "اختلاف الشكل الصيدلي"}
	}

	if !idx.sharedProductWord(row, p) {
		return MatchConflict{Kind: "evidence", Detail: "لا توجد كلمة مميزة مشتركة بين الاسمين"}
	}
	return MatchConflict{}
}

// StrengthConflict reports a dose disagreement on its own.
//
// Kept separate from IdentityConflict because the dose is the one attribute
// worth checking in isolation: it is the check with no false positives to trade
// away, since two different strengths of one brand are always two products.
func (idx *Index) StrengthConflict(row *Row, productID int64) bool {
	if idx == nil || row == nil {
		return false
	}
	p, ok := idx.byID[productID]
	if !ok {
		return false
	}
	return strengthsConflict(
		row.Name+" "+row.NameEN+" "+row.Concentration,
		p.NameAR+" "+p.NameEN+" "+p.Concentration)
}

// strengthsConflict answers false whenever either side is silent, because that
// is missing information rather than disagreement — and a catalogue that records
// the dose only inside the product name, as much of this one does, is silent
// often.
//
// Silence is per unit, not per side. Product names carry milligrams and
// millilitres in the same breath — "امبريديل شراب 120 مل" states a bottle size
// and no dose at all — so comparing a row's 200 mg against a product's 120 ml
// and calling the difference a dose conflict is comparing two things that were
// never the same measurement. Only units present on BOTH sides can disagree.
func strengthsConflict(rowText, prodText string) bool {
	rowDoses := strengthSet(rowText)
	prodDoses := strengthSet(prodText)
	if len(rowDoses) == 0 || len(prodDoses) == 0 {
		return false
	}

	agree, comparable := compareStrengths(rowDoses, prodDoses)
	return comparable && !agree
}

// compareStrengths relates two dose sets on the units they have in common.
//
// comparable is false when no unit appears on both sides, which is missing
// information rather than agreement OR disagreement: a row stating a 120 ml
// bottle and a product stating 200 mg have not contradicted each other, and
// scoring that as a conflict cost a long list of correct matches half a point
// each.
//
// agree requires EVERY shared unit to match, and that is a correction rather
// than a tightening. It used to be true as soon as any shared unit matched,
// reasoning that a combination product states several doses and one of them
// agreeing identifies it. What that actually did was let one figure vouch for
// another: "اميكين 100مجم/2مل فيال" and "اميكين 250مجم/2مل فيال" are two
// different vials of amikacin, they agree on the 2 ml of solvent, and on the
// strength of that agreement the engine applied one as the other. So did
// "البوستيكس دي 16/12.5مجم" against "البوستيكس دي 32/12.5مجم", and every other
// family whose members share a vehicle and differ in the drug.
//
// Within a unit, agreement is set equality rather than overlap, for the same
// reason: 5/5 mg and 5/10 mg overlap on the 5 and are two products.
func compareStrengths(rowDoses, prodDoses []strength) (agree, comparable bool) {
	rows, prods := byUnit(rowDoses), byUnit(prodDoses)
	agree = true
	for unit, a := range rows {
		b, shared := prods[unit]
		if !shared {
			continue // this measurement is missing from the other side
		}
		comparable = true
		if !doseSetsEqual(a, b) {
			agree = false
		}
	}
	return agree && comparable, comparable
}

// byUnit groups a dose set by the unit each dose was measured in.
func byUnit(set []strength) map[string][]strength {
	out := make(map[string][]strength, 2)
	for _, s := range set {
		out[s.unit] = append(out[s.unit], s)
	}
	return out
}

// doseSetsEqual decides whether two dose lists of one unit describe the same
// product.
//
// Containment, not equality, except between two combinations. A pharmacy
// abbreviates: it writes "اتاكاند بلس 16 مجم" for a product the catalogue
// records as 16/12.5 mg, and refusing that would refuse the commonest way a
// combination is ordered. What it does NOT do is invent a component — so where
// BOTH sides state a ratio, the ratios have to be the same one: املوفيران
// 5/5 مجم and املوفيران 5/10 مجم each state two components, they share the 5,
// and they are two products.
//
// The same containment rule covers a name that measures more than it doses.
// "الفابونيد 1 مكجم/مل (1مل=2مكجم)" states the concentration twice over; a row
// that states it once has not disagreed.
func doseSetsEqual(a, b []strength) bool {
	if isCombination(a) && isCombination(b) {
		return doseSetCovers(a, b) && doseSetCovers(b, a)
	}
	return doseSetCovers(a, b) || doseSetCovers(b, a)
}

// isCombination reports whether a dose list came from a ratio — "32/25 مجم"
// rather than "32 مجم".
func isCombination(set []strength) bool {
	for _, s := range set {
		if s.parts > 1 {
			return true
		}
	}
	return false
}

func doseSetCovers(have, want []strength) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			if sameStrength(h, w) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// modifierConflict compares the line-extension words that separate one product
// in a brand family from another.
//
// This is the check that answers the reported failure directly. "بانادول" and
// "بانادول اكسترا" agree on every word one of them has; what separates them is
// the word only the other one carries, and that word is the product. So the
// comparison is set equality, not overlap: a modifier on either side that the
// other lacks is a conflict.
//
// It compares only the curated vocabulary below. A general "the candidate has a
// word the row lacks" rule would refuse almost everything, because catalogue
// names are verbose and pharmacy lines are terse — "بانادول" against "بانادول
// 500مجم 24 قرص" would fail it, and that is the same product.
func modifierConflict(row *Row, p *MasterProduct) (string, bool) {
	rowText := row.Name + " " + row.NameEN
	prodText := p.NameAR + " " + p.NameEN
	return modifierSetsDiffer(modifiersIn(rowText), p.modifiers(), rowText, prodText)
}

// modifierSetsConflict is the same comparison for callers that already hold
// both sets, which the scorer does: it derives the row's once per row and the
// catalogue's once per product, rather than re-tokenising two names inside a
// loop that runs three million times on a large file.
func modifierSetsConflict(rowMods, prodMods map[string]struct{}, rowText, prodText string) bool {
	_, differ := modifierSetsDiffer(rowMods, prodMods, rowText, prodText)
	return differ
}

// modifierSetsDiffer is the set comparison itself, and the texts are carried
// only so a missing modifier can be looked for unspaced before it is called a
// conflict. See modifierGlued.
func modifierSetsDiffer(
	rowMods, prodMods map[string]struct{}, rowText, prodText string,
) (string, bool) {
	if len(rowMods) == 0 && len(prodMods) == 0 {
		return "", false
	}
	for m := range rowMods {
		if _, both := prodMods[m]; !both && !modifierGlued(prodText, m) {
			return m, true
		}
	}
	for m := range prodMods {
		if _, both := rowMods[m]; !both && !modifierGlued(rowText, m) {
			return m, true
		}
	}
	return "", false
}

// modifiers is the catalogue product's line-extension keys, derived at index
// time. A nil map is a product whose name carries none, which is the common
// case and costs nothing to compare against.
func (p *MasterProduct) modifiers() map[string]struct{} { return p.mods }

// modifierGlued asks whether a modifier is present without a space around it.
//
// "اكوابلس شراب" and "اكوا بلس شراب 100 مل" are the same product; one side
// spaced the modifier and the other did not, and a token comparison alone reads
// that as two different products in a brand family. So before a conflict is
// declared, the missing side is searched with the spaces removed.
//
// It only ever *relaxes* a conflict, and only for the one modifier in question.
// The search is over a space-stripped name, so a hit means the letters are there
// in sequence — which for the multi-letter words in this vocabulary is enough.
func modifierGlued(text, key string) bool {
	joined := strings.ReplaceAll(sheet.NormalizeName(text), " ", "")
	for word, k := range variantModifiers {
		if k != key || len([]rune(word)) < 3 {
			// Two-letter release codes ("sr", "cr") are never searched this way:
			// those letters occur inside ordinary words constantly.
			continue
		}
		if strings.Contains(joined, word) {
			return true
		}
	}
	return false
}

// formsConflict compares dosage forms, but only when the pharmacy actually
// stated one.
//
// A row that names no form is silent, not agreeable: pharmacy lines routinely
// omit it. But a row that says شراب has said something, and a tablet is not it.
//
// Tablets and capsules are one class here, and that is a concession to how
// pharmacies actually write. Measured on the live catalogue, "افودارت اقراص"
// against "افودارت 0.5مجم 30 كبسولة" and "اورسوفالك 20كبسولة" against
// "اورسوفالك 250مجم 20 قرص" are both the same product written loosely — the
// word for the solid oral form is used interchangeably by the person typing the
// order. Vetoing on it refused more correct matches than it prevented wrong
// ones. Every other form stays distinct: a syrup, an injection and a cream are
// never each other.
func formsConflict(row *Row, p *MasterProduct) bool {
	rowForm := vetoableForm(formKeyOf(row.Name + " " + row.NameEN + " " + row.DosageForm))
	prodForm := vetoableForm(p.formKey)
	if rowForm == "" || prodForm == "" {
		return false
	}
	return rowForm != prodForm
}

// vetoableForm maps a form key onto the class the veto compares, and returns ""
// for the one key that must never veto.
//
// Tablets and capsules collapse together, because the person typing the order
// uses اقراص and كبسول interchangeably for any solid oral form: "افودارت اقراص"
// and the catalogue's "افودارت 0.5مجم 30 كبسولة" are the same product, and
// vetoing on that difference refused more correct matches than it prevented
// wrong ones.
//
// "wash" alone is neutral, because it describes a USE rather than a form and
// overlaps every other word: "الكا مصر اكياس" and "الكا-مصر غسول قلوي بودرة 12
// اكياس" are one product named once by its packaging and once by what it is for.
//
// Everything else vetoes, sachets included. A sachet of granules is not a
// tablet — "ادويفلام 50 مجم 2 شريط" is two strips of tablets and not the
// ten-sachet product, and letting that through was a real wrong match.
func vetoableForm(key string) string {
	switch key {
	case "tablet", "capsule":
		return "solid_oral"
	case "wash":
		return ""
	}
	return key
}
