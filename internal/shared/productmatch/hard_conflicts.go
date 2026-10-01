package productmatch

// Which contradictions refuse a candidate outright.
//
// There is ONE model of what a row and a catalogue product disagree about, and
// it is conflictsOf in discriminate.go: measured against live Egyptian files,
// aware that a pharmacist writes اقراص and كبسول for any solid oral form, that
// لوسيون and غسول name the same bottle, that the English half of a catalogue
// record often states less than the Arabic half, and that silence is not
// disagreement. The scorer ranks with it.
//
// The gate used to answer the same question with a second, independent model
// (CompareFacts) that compared a row's form GROUP ("topical") with the
// catalogue's raw dosage_form column ("cream"), read brands from the Arabic name
// only, and treated every figure as a dose. The two disagreed on a third of all
// rows, and the gate refused the correct product for every one of them. So the
// gate now asks the scorer's model, and the two can no longer disagree.
//
// A conflict is HARD when it proves a different product whatever the names
// look like: another strength, another combination, another line extension,
// another form or sub-form, another identity letter, another route or size.
// The rest — a pack count, a stray figure, a line extension or letter the row
// merely leaves unstated — stay what they always were: penalties that decide
// between candidates and that, on the only candidate, send it to review.

// hardConflictKinds maps the scorer's conflict kinds onto the gate's.
var hardConflictKinds = map[string]ConflictKind{
	"strength":   ConflictStrength,
	"dose_parts": ConflictCombination,
	"modifier":   ConflictModifier,
	"form":       ConflictForm,
	"sub_form":   ConflictForm,
	"letter":     ConflictLetter,
	"route":      ConflictRoute,
	"size":       ConflictDimension,
	"brand":      ConflictBrand,
}

// softConflictReasons names the penalties that are not vetoes, for display.
var softConflictReasons = map[string]string{
	"count":             "pack_conflict",
	"pack":              "pack_conflict",
	"figure":            "figure_conflict",
	"modifier_unstated": "modifier_unstated",
	"letter_unstated":   "letter_unstated",
	"brand_unverified":  "brand_unverified",
	"container":         "container_conflict",
	"volume":            "pack_conflict",
	"word_unverified":   "word_unverified",
	"word_mismatch":     "word_mismatch",
	"maker":             "maker_conflict",
}

// hardOf returns the hard kinds among a candidate's conflicts, de-duplicated.
func hardOf(cs []conflict) []ConflictKind {
	var out []ConflictKind
	for _, c := range cs {
		k, ok := hardConflictKinds[c.kind]
		if !ok {
			continue
		}
		dup := false
		for _, have := range out {
			if have == k {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, k)
		}
	}
	return out
}

// hasPackConflict reports whether the name or the pack column contradicts the
// row's count.
func hasPackConflict(cs []conflict) bool {
	for _, c := range cs {
		if c.kind == "count" || c.kind == "pack" || c.kind == "volume" {
			return true
		}
	}
	return false
}

// HardConflicts reports the hard contradictions between a row and one
// catalogue product, using the scorer's own model plus the brand guard.
//
// It is what every path that did not come through scoring — an identifier hit,
// remembered knowledge, a reviewer's choice, a model's proposal — is checked
// with, so that no path can accept what the scorer would have refused.
func (idx *Index) HardConflicts(row *Row, productID int64) []ConflictKind {
	if idx == nil || row == nil {
		return nil
	}
	p, ok := idx.byID[productID]
	if !ok {
		return nil
	}
	q := idx.newQuery(row)
	return hardOf(idx.conflictsOf(q, p))
}

// candidateDiff renders a candidate's conflicts as the per-attribute view the
// review screen and the AI eligibility check read.
func candidateDiff(cs []conflict) (map[string]string, []string) {
	diff := map[string]string{}
	var reasons []string
	for _, c := range cs {
		attr := c.kind
		switch c.kind {
		case "dose_parts":
			attr = "strength"
		case "sub_form":
			attr = "form"
		case "count", "pack", "volume":
			attr = "pack"
		case "modifier_unstated":
			attr = "modifier"
		case "letter_unstated":
			attr = "letter"
		case "brand_unverified", "word_unverified", "word_mismatch":
			attr = "brand"
		case "container":
			attr = "form"
		case "maker":
			attr = "manufacturer"
		}
		if k, hard := hardConflictKinds[c.kind]; hard {
			diff[attr] = string(OutcomeConflict)
			reasons = append(reasons, string(conflictToReasonCode(k)))
			continue
		}
		if _, set := diff[attr]; !set {
			diff[attr] = string(OutcomeUncertain)
		}
		reasons = append(reasons, softConflictReasons[c.kind])
	}
	return diff, reasons
}

// hardConflictSource is what an index offers the gate: the scorer's own
// verdict on whether a row and a product can be the same item.
type hardConflictSource interface {
	HardConflicts(row *Row, productID int64) []ConflictKind
}

// hardFor resolves a proposal's hard conflicts from the best source available.
func (g *Gate) hardFor(rowFacts IdentityFacts, p Proposal, prodFacts IdentityFacts) []ConflictKind {
	if p.HardKnown {
		return p.Hard
	}
	if p.Row != nil {
		if src, ok := g.Catalog.(hardConflictSource); ok {
			return src.HardConflicts(p.Row, p.ProductID)
		}
	}
	return CompareFacts(rowFacts, prodFacts).Hard
}
