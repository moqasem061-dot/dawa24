package productmatch

import "testing"

// The reported failure was "the AI matches two products that share two words".
// These are the pairs that caused it, taken from the live Egyptian catalogue,
// alongside the pairs that must keep matching — because a guard that refuses
// everything is not a fix.

func identityIndex() *Index {
	return NewIndex([]MasterProduct{
		// A brand family separated only by a line-extension word.
		{ID: 1, NameAR: "بانادول 500مجم 24 قرص", NameEN: "panadol 500 mg 24 tabs",
			DosageForm: "tablet", Concentration: "500 mg", Manufacturer: "gsk"},
		{ID: 2, NameAR: "بانادول اكسترا 24 قرص", NameEN: "panadol extra 24 f.c. tab",
			DosageForm: "tablet", Manufacturer: "gsk"},
		{ID: 3, NameAR: "بانادول نايت 20 قرص", NameEN: "panadol night 20 tabs",
			DosageForm: "tablet", Manufacturer: "gsk"},

		// One brand at three strengths.
		{ID: 10, NameAR: "اكسامايد 10مجم 30 قرص", NameEN: "examide 10 mg 30 tabs",
			DosageForm: "tablet", Concentration: "10 mg"},
		{ID: 11, NameAR: "اكسامايد 100مجم 20 قرص", NameEN: "examide 100 mg 20 tabs",
			DosageForm: "tablet", Concentration: "100 mg"},

		// A combination written as a ratio.
		{ID: 20, NameAR: "اتاكاند بلس 12.5/16مجم 14 قرص", NameEN: "atacand plus 16/12.5 mg",
			DosageForm: "tablet", Concentration: "16/12.5 mg"},

		// Two unrelated medicines from one house.
		{ID: 30, NameAR: "سيفوتاكس 500مجم فيال ايبيكو", NameEN: "cefotax 500 mg vial",
			DosageForm: "vial", Manufacturer: "epico"},
		{ID: 31, NameAR: "ابيفيناك 75 مجم 3 امبولات", NameEN: "epifenac 75 mg 3 amps",
			DosageForm: "ampoule", Concentration: "75 mg", Manufacturer: "epico"},

		// Spelling and spacing variants of one product.
		{ID: 40, NameAR: "ازارجا قطرة عين معلق 5 مل", NameEN: "azarga eye drops 5 ml",
			DosageForm: "eye drops"},
		{ID: 41, NameAR: "ارموويك 50مجم 10 اقراص", NameEN: "armowic 50 mg 10 tabs",
			DosageForm: "tablet", Concentration: "50 mg"},
		{ID: 42, NameAR: "اكوا بلس شراب 100 مل", NameEN: "aqua plus syrup 100 ml",
			DosageForm: "syrup"},

		// Same product, solid oral form written loosely on each side.
		{ID: 50, NameAR: "افودارت 0.5مجم 30 كبسولة", NameEN: "avodart 0.5 mg 30 caps",
			DosageForm: "capsule", Concentration: "0.5 mg"},

		// Two brands sharing a prefix.
		{ID: 60, NameAR: "امبريديل شراب 120 مل", NameEN: "ambridyl syrup 120 ml",
			DosageForm: "syrup"},
	})
}

func mustRefuse(t *testing.T, idx *Index, row *Row, id int64, wantKind string) {
	t.Helper()
	c := idx.IdentityConflict(row, id)
	if c.None() {
		t.Errorf("%q was allowed to match product %d; expected a %s conflict",
			row.Name, id, wantKind)
		return
	}
	if c.Kind != wantKind {
		t.Errorf("%q against %d was refused as %q, want %q (%s)",
			row.Name, id, c.Kind, wantKind, c.Detail)
	}
}

func mustAllow(t *testing.T, idx *Index, row *Row, id int64) {
	t.Helper()
	if c := idx.IdentityConflict(row, id); !c.None() {
		t.Errorf("%q was refused against product %d as %q (%s) — it is the same product",
			row.Name, id, c.Kind, c.Detail)
	}
}

// 🚦 The reported failure, directly. A brand family shares every word but one,
// and that one word is the product.
func TestLineExtensionIsADifferentProduct(t *testing.T) {
	idx := identityIndex()

	// Plain Panadol is not Panadol Extra and not Panadol Night.
	mustRefuse(t, idx, &Row{Name: "بانادول", Concentration: "500 مجم"}, 2, "modifier")
	mustRefuse(t, idx, &Row{Name: "بانادول", Concentration: "500 مجم"}, 3, "modifier")
	mustAllow(t, idx, &Row{Name: "بانادول", Concentration: "500 مجم"}, 1)

	// And the reverse: a line that asks for Extra must not take the plain one.
	mustRefuse(t, idx, &Row{Name: "بانادول اكسترا"}, 1, "modifier")
	mustAllow(t, idx, &Row{Name: "بانادول اكسترا"}, 2)

	// The pharmacy's own spelling of the brand does not change the rule.
	mustRefuse(t, idx, &Row{Name: "بنادول نايت"}, 2, "modifier")
	mustAllow(t, idx, &Row{Name: "بنادول نايت"}, 3)
}

// Three strengths of one brand are three products.
func TestStrengthSeparatesOneBrand(t *testing.T) {
	idx := identityIndex()
	mustRefuse(t, idx, &Row{Name: "اكسامايد", Concentration: "100 مجم"}, 10, "strength")
	mustRefuse(t, idx, &Row{Name: "اكسامايد", Concentration: "5 مجم"}, 10, "strength")
	mustAllow(t, idx, &Row{Name: "اكسامايد", Concentration: "10 مجم"}, 10)

	// A combination states its strengths as a ratio; the pharmacy states the
	// leading one. 16 agrees with 16/12.5 and 32 does not.
	mustAllow(t, idx, &Row{Name: "اتاكاند بلس", Concentration: "16 مجم", PackSize: 14}, 20)
	mustRefuse(t, idx, &Row{Name: "اتاكاند بلس", Concentration: "32 مجم", PackSize: 14}, 20, "strength")
}

// 🚦 Two different drugs from one manufacturer share exactly one word, and it is
// the word that names the company. This is the pair that was matched in
// production.
func TestASharedManufacturerIsNotEvidence(t *testing.T) {
	idx := identityIndex()
	row := &Row{Name: "ابيفيناك", DosageForm: "حقن", Manufacturer: "ايبيكو"}

	mustRefuse(t, idx, row, 30, "evidence")
	mustAllow(t, idx, row, 31)
}

// Two brands beginning with the same letters are usually unrelated.
func TestASharedPrefixIsNotEvidence(t *testing.T) {
	idx := identityIndex()
	mustRefuse(t, idx, &Row{Name: "امبريد", Concentration: "200 مجم"}, 60, "evidence")
}

// The other half of the requirement: one product written two ways must still
// match, or the stage stops being worth running.
func TestSpellingAndSpacingVariantsStillMatch(t *testing.T) {
	idx := identityIndex()

	// A single inserted letter — trigram overlap calls these strangers, edit
	// distance does not.
	mustAllow(t, idx, &Row{Name: "ازرجا", DosageForm: "قطره"}, 40)

	// One word against two, in both directions.
	mustAllow(t, idx, &Row{Name: "ارمو ويك", Concentration: "50 مجم"}, 41)
	mustAllow(t, idx, &Row{Name: "اكوابلس", DosageForm: "شراب"}, 42)
}

// Pharmacies use اقراص and كبسول interchangeably for any solid oral form, so
// that difference must not veto. Every other route still does.
func TestSolidOralFormsDoNotVetoButOtherRoutesDo(t *testing.T) {
	idx := identityIndex()

	mustAllow(t, idx, &Row{Name: "افودارت", DosageForm: "اقراص", Concentration: "0.5 مجم"}, 50)
	mustRefuse(t, idx, &Row{Name: "ازارجا", DosageForm: "اقراص"}, 40, "form")
}

// A row that names no form has said nothing, and silence is not disagreement.
func TestAnUnstatedFormNeverVetoes(t *testing.T) {
	idx := identityIndex()
	mustAllow(t, idx, &Row{Name: "ازارجا"}, 40)
}

// An orally disintegrating line is a separate product from the ordinary tablet
// of the same brand and dose. The model matched one to the other until the
// vocabulary said otherwise.
func TestOrallyDisintegratingIsADifferentProduct(t *testing.T) {
	idx := NewIndex([]MasterProduct{
		{ID: 1, NameAR: "ابيليفاي 10مجم 10 اقراص", NameEN: "abilify 10 mg 10 tabs",
			DosageForm: "tablet", Concentration: "10 mg"},
		{ID: 2, NameAR: "ابيليفاي 10مجم ديسكملت 10 اقراص ذائبة بالفم",
			NameEN: "abilify discmelt 10 mg", DosageForm: "tablet", Concentration: "10 mg"},
	})
	row := &Row{Name: "ابليفاى", Concentration: "10 مجم"}

	mustAllow(t, idx, row, 1)
	mustRefuse(t, idx, row, 2, "modifier")
}

// A sachet of granules is not a strip of tablets, however the pharmacy wrote it.
func TestASachetIsNotASolidOralForm(t *testing.T) {
	idx := NewIndex([]MasterProduct{
		{ID: 1, NameAR: "ادويفلام 50مجم 10 اكياس", NameEN: "adwiflam 50 mg 10 sachets",
			DosageForm: "sachet", Concentration: "50 mg"},
		{ID: 2, NameAR: "ادويفلام 50مجم 20 قرص", NameEN: "adwiflam 50 mg 20 tabs",
			DosageForm: "tablet", Concentration: "50 mg"},
	})
	row := &Row{Name: "ادويفلام", Concentration: "50 مجم", DosageForm: "أقراص"}

	mustRefuse(t, idx, row, 1, "form")
	mustAllow(t, idx, row, 2)
}

// The vocabulary is curated, so a descriptive word must not behave like a
// line-extension word.
func TestModifiersInReadsOnlyTheCuratedVocabulary(t *testing.T) {
	got := modifiersIn("بانادول اكسترا اوبتيزورب 24 قرص فيلم كوتد")
	if _, ok := got["extra"]; !ok {
		t.Error("اكسترا was not read as a line extension")
	}
	if len(got) != 1 {
		t.Errorf("descriptive words were read as line extensions: %v", got)
	}
}

func TestEditSimilarity(t *testing.T) {
	cases := []struct {
		a, b string
		want bool // at or above the acceptance threshold
	}{
		{"ازرجا", "ازارجا", true},
		{"ابليفاي", "ابيليفاي", true},
		{"بنادول", "بانادول", true},
		{"اباندروكير", "ايباندروكير", true},
		{"اريكتامكس", "سياليس", false},
		{"امبريد", "امبريديل", false},
		{"سيفوتاكس", "ابيفيناك", false},
	}
	for _, c := range cases {
		got := editSimilarity(c.a, c.b) >= minEditSimilarity
		if got != c.want {
			t.Errorf("editSimilarity(%q,%q) = %.2f, wanted acceptance=%v",
				c.a, c.b, editSimilarity(c.a, c.b), c.want)
		}
	}
}

// 🚦 The substring defect that filed three ordinary oral products as topical
// gels, because their Arabic names happen to contain the letters of "جل".
func TestFormKeyReadsWordsNotSubstrings(t *testing.T) {
	cases := map[string]string{
		"امباجليفورم 12.5/500مجم 30 قرص": "tablet",
		"اميجليسباير 500مجم 30 قرص":      "tablet",
		"جلوكوفاج 500مجم 30 قرص":         "tablet",
		"فولتارين ايمولجل 50 جم":         "", // "ايمولجل" is a brand, not a gel
		"ديرموفيت جل 30 جم":              "topical",
		"افيميو قطرة عين معلق 10 مل":     "drops",
		"انيماكس حقنة شرجية 120 مل":      "injectable",
		"افوصويا 3شريط":                  "tablet",
		"اورسوفالك 20كبسولة":             "capsule",
	}
	for in, want := range cases {
		if got := formKeyOf(in); got != want {
			t.Errorf("formKeyOf(%q) = %q, want %q", in, got, want)
		}
	}
}
