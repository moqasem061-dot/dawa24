package productmatch

import "testing"

// Recall exists because Match's shortlist was useless as a second opinion: it
// came from the pool that had already defeated the scorer, and when the scorer
// found nothing there was no shortlist at all. Every test here is a case where
// Match returns nothing usable and the correct product is nevertheless in the
// catalogue.

func testRecallIndex() *Index {
	return NewIndex([]MasterProduct{
		{ID: 1, NameAR: "ابيليفاي 10مجم 10 اقراص", NameEN: "abilify 10 mg 10 tabs",
			DosageForm: "tablet", Concentration: "10 mg"},
		{ID: 2, NameAR: "اتاكاند بلس 32/25 ملجم 14 قرص", NameEN: "atacand plus 32/25 mg 14 tab",
			DosageForm: "tablet", Concentration: "32/25 mg"},
		{ID: 3, NameAR: "اتوموكستين 25مجم 10 كبسولات", NameEN: "atomoxetine 25 mg 10 caps",
			DosageForm: "capsule", Concentration: "25 mg"},
		{ID: 4, NameAR: "بانادول اكسترا 24 قرص", NameEN: "panadol extra 24 f.c. tab",
			DosageForm: "tablet"},
		{ID: 5, NameAR: "فولتارين 50مجم 20 قرص", NameEN: "voltaren 50 mg 20 tabs",
			Scientific: "diclofenac", DosageForm: "tablet", Concentration: "50 mg"},
	})
}

func topIDs(cands []MatchCandidate, n int) []int64 {
	out := make([]int64, 0, n)
	for i, c := range cands {
		if i == n {
			break
		}
		out = append(out, c.ProductID)
	}
	return out
}

func contains(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// Egyptian pharmacy files transliterate Latin brands freely. "ابليفاى" and the
// catalogue's "ابيليفاي" share no whole word, so the token pool comes back
// empty — this is the case the trigram strategy exists for.
func TestRecallFindsATransliterationVariant(t *testing.T) {
	idx := testRecallIndex()
	row := &Row{Name: "ابليفاى", Concentration: "10 مجم"}

	got := topIDs(idx.Recall(row, DefaultRecallOptions()), 3)
	if !contains(got, 1) {
		t.Fatalf("abilify was not retrieved for its Arabic spelling variant: %v", got)
	}
}

// A combination product written the way a pharmacy writes it: the strength
// stated once, the catalogue stating it as a ratio.
func TestRecallFindsACombinationProduct(t *testing.T) {
	idx := testRecallIndex()
	row := &Row{Name: "اتاكاند بلس", Concentration: "32 مجم", PackSize: 14}

	got := topIDs(idx.Recall(row, DefaultRecallOptions()), 3)
	if !contains(got, 2) {
		t.Fatalf("atacand plus was not retrieved: %v", got)
	}
}

// 🚦 The difference from Match, stated directly: a disagreeing strength must
// score a candidate DOWN without removing it. Deciding that question is the
// model's job, and the applier re-checks the answer before writing it.
func TestRecallStillReturnsCandidatesWhenTheStrengthDisagrees(t *testing.T) {
	idx := testRecallIndex()
	row := &Row{Name: "ابيليفاي", Concentration: "30 مجم"}

	if got := idx.Recall(row, DefaultRecallOptions()); len(got) == 0 {
		t.Fatal("a disagreeing strength removed every candidate")
	}
}

// The molecule strategy: the pharmacy wrote the generic name and the catalogue
// carries only the brand.
func TestRecallFindsByScientificName(t *testing.T) {
	idx := testRecallIndex()
	row := &Row{Name: "ديكلوفيناك صوديوم", Scientific: "diclofenac"}

	got := topIDs(idx.Recall(row, DefaultRecallOptions()), 5)
	if !contains(got, 5) {
		t.Fatalf("the brand was not retrieved from its molecule: %v", got)
	}
}

func TestRecallRespectsItsLimit(t *testing.T) {
	idx := testRecallIndex()
	opts := DefaultRecallOptions()
	opts.Limit = 2

	if got := idx.Recall(&Row{Name: "بانادول اكسترا"}, opts); len(got) > 2 {
		t.Fatalf("returned %d candidates, limit was 2", len(got))
	}
}

// Nothing related in the catalogue is information, not an error: the caller
// needs to know it has no question to ask.
func TestRecallReturnsNothingForAnUnrelatedRow(t *testing.T) {
	idx := testRecallIndex()
	if got := idx.Recall(&Row{Name: "zzzz qqqq"}, DefaultRecallOptions()); len(got) != 0 {
		t.Fatalf("an unrelated row retrieved %d candidates: %v", len(got), topIDs(got, 3))
	}
}

// 🚦 StrengthConflict is the guard that stops a confident model prescribing a
// different dose. It must fire on a real disagreement and stay silent when
// either side simply did not say.
func TestStrengthConflict(t *testing.T) {
	idx := testRecallIndex()

	if !idx.StrengthConflict(&Row{Name: "ابيليفاي", Concentration: "30 مجم"}, 1) {
		t.Error("30 mg against a 10 mg product was not reported as a conflict")
	}
	if idx.StrengthConflict(&Row{Name: "ابيليفاي", Concentration: "10 مجم"}, 1) {
		t.Error("10 mg against a 10 mg product was reported as a conflict")
	}
	if idx.StrengthConflict(&Row{Name: "ابيليفاي"}, 1) {
		t.Error("a row that states no dose was reported as conflicting")
	}
	if idx.StrengthConflict(&Row{Name: "بانادول اكسترا", Concentration: "500 مجم"}, 4) {
		t.Error("a product that states no dose was reported as conflicting")
	}
	// A combination stated as a ratio agrees with its leading figure, which is
	// how a pharmacy writes it.
	if idx.StrengthConflict(&Row{Name: "اتاكاند بلس", Concentration: "32 مجم"}, 2) {
		t.Error("32 mg against a 32/25 mg combination was reported as a conflict")
	}
}
