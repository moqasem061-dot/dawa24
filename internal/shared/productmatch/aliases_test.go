package productmatch

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

func TestAliases_CloneIsolation(t *testing.T) {
	products := []MasterProduct{
		{ID: 1, NameAR: "بنادول ازرق 500 مجم", NameEN: "Panadol Blue 500mg"},
	}
	baseIdx := NewIndex(products)
	if len(baseIdx.aliases) != 0 {
		t.Fatalf("expected baseIdx to have no aliases, got %d", len(baseIdx.aliases))
	}

	aliases := map[string][]int64{
		"مسكن الصداع": {1},
	}
	idxWithAliases := baseIdx.WithAliases(aliases)

	if len(baseIdx.aliases) != 0 {
		t.Fatalf("baseIdx should remain unmodified, but got %d aliases", len(baseIdx.aliases))
	}
	if len(idxWithAliases.aliases) == 0 {
		t.Fatalf("idxWithAliases should have aliases")
	}
}

func TestAliases_RetrievalChannelAndGate(t *testing.T) {
	products := []MasterProduct{
		{
			ID:            10,
			NameAR:        "باراسيتامول 500 مجم 20 قرص",
			NameEN:        "Paracetamol 500mg 20 tablets",
			DosageForm:    "tablets",
			Concentration: "500mg",
		},
		{
			ID:            20,
			NameAR:        "باراسيتامول 1000 مجم 10 اقراص",
			NameEN:        "Paracetamol 1000mg 10 tablets",
			DosageForm:    "tablets",
			Concentration: "1000mg",
		},
	}
	idx := NewIndex(products)

	// A row using an obscure trade name / alias with 500mg
	row := &Row{
		Name: "علاج الصداع الخاص 500 مجم",
	}

	pol := DefaultPolicy()

	// 1. Without alias: the name shares zero tokens with "باراسيتامول", pool is empty or no match
	resBefore := idx.Decide(row, pol)
	if resBefore.State == matchflow.StateAccepted && resBefore.ProductID != nil && *resBefore.ProductID == 10 {
		t.Fatalf("unexpected match without alias: %+v", resBefore)
	}

	// 2. With alias pointing to product 10:
	aliasMap := map[string][]int64{
		"علاج الصداع الخاص": {10},
	}
	idxWithAliases := idx.WithAliases(aliasMap)

	resAfter := idxWithAliases.Decide(row, pol)
	// Candidate 10 must be retrieved and evaluated by the gate
	// Because 500mg matches and dosage form tablets matches, it should be accepted or in candidates
	if resAfter.ProductID == nil || *resAfter.ProductID != 10 {
		// At minimum it should be shortlisted as a candidate
		found := false
		for _, c := range resAfter.Candidates {
			if c.ProductID == 10 {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected product 10 to be retrieved via alias, got: %+v", resAfter)
		}
	}

	// 3. Gate enforcement: row states 1000mg, but alias points to product 10 (500mg)
	// The alias acts as a hint, but the hard conflict on strength MUST prevent acceptance!
	rowConflicting := &Row{
		Name: "علاج الصداع الخاص 1000 مجم",
	}
	resConflict := idxWithAliases.Decide(rowConflicting, pol)
	if resConflict.State == matchflow.StateAccepted && resConflict.ProductID != nil && *resConflict.ProductID == 10 {
		t.Fatalf("gate failed to reject conflicting alias candidate: %+v", resConflict)
	}
}
