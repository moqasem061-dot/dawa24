package productmatch

// SameSellableItem evaluates whether two items identified by their facts
// represent the same sellable product and package unit.
//
// Invariants:
// - Evaluates identity facts symmetrically for hard conflicts.
// - Requires positive identity agreement (matching brand or identifier).
// - Requires pack agreement (direct count/sub-count match, or bridged by an active PackRelationship).
// - Strip vs Box or differing pack sizes without an active bridging relationship return false.
// - When identical or equivalent at pack level, returns (true, "source_equivalence").
// - When bridged by an active pack relationship, returns (true, "relationship").
func SameSellableItem(a, b IdentityFacts, rels []PackRelationship) (same bool, basis string) {
	compAB := CompareFacts(a, b)
	compBA := CompareFacts(b, a)

	if len(compAB.Hard) > 0 || len(compBA.Hard) > 0 {
		return false, ""
	}

	brandAgreed := compAB.Attributes["brand"] == OutcomeAgree || compBA.Attributes["brand"] == OutcomeAgree
	idAgreed := compAB.Attributes["identifier"] == OutcomeAgree || compBA.Attributes["identifier"] == OutcomeAgree
	if !brandAgreed && !idAgreed {
		return false, ""
	}

	// Strengths: if one states a dose/strength and the other does not, they cannot be assumed identical.
	if (len(a.Strengths) > 0) != (len(b.Strengths) > 0) {
		return false, ""
	}
	if (a.Concentration != nil) != (b.Concentration != nil) {
		return false, ""
	}

	// Pack agreement or relationship
	if a.Pack != nil && b.Pack != nil {
		outcome, relID := PackOutcome(*a.Pack, *b.Pack, rels)
		if outcome != "agree" {
			return false, ""
		}
		if relID > 0 {
			return true, "relationship"
		}
		return true, "source_equivalence"
	}

	// Unstated pack on either side is compatible when all other identity facts agree without conflict
	return true, "source_equivalence"
}
