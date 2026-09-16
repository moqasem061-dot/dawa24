package productmatch

import "testing"

// The spellings this channel exists for.
//
// Every pair below is one medicine written by two people in the Egyptian
// market. Before the variant channel each pair met only in the trigram or
// whole-name skeleton channels, both discounted, and arrived at the review
// screen needing a human to confirm what the arithmetic already knew.
func TestEgyptianSpellingVariantsFoldTogether(t *testing.T) {
	same := []struct {
		a, b, why string
	}{
		{"ابيكوبريد", "ابيكوبرايد", "an inserted long vowel"},
		{"بنادول", "بانادول", "a long vowel written or omitted"},
		{"كنكور", "كونكور", "a long vowel written or omitted"},
		{"زيثروماكس", "زيسروماكس", "the ث/س confusion"},
		{"جاسترو", "غاسترو", "the ج/غ class"},
		{"اوجمنتين", "أوجمنتين", "a hamza seat"},
		{"فولتارين", "فولتارن", "a dropped long vowel"},
		{"اموكسيسيللين", "اموكسيسيلين", "a doubled letter"},
		// Cross-script, which is the same fold doing a second job: the primary
		// channel could not see these at all before.
		{"بانادول", "panadol", "Arabic against Latin"},
		{"اوجمنتين", "augmentin", "Arabic against Latin"},
		{"فولتارين", "voltaren", "Arabic against Latin, with ف/v"},
	}
	for _, tc := range same {
		ka, kb := DebugVariantKey(tc.a), DebugVariantKey(tc.b)
		if ka == "" || kb == "" {
			t.Errorf("%q/%q (%s): one side produced no variant key (%q/%q)", tc.a, tc.b, tc.why, ka, kb)
			continue
		}
		if ka != kb {
			t.Errorf("%q/%q (%s): keys differ, %q vs %q", tc.a, tc.b, tc.why, ka, kb)
		}
	}
}

// Words that must NOT fold together, because folding them would link two
// different medicines — the mistake that is not recoverable by looking at the
// result.
func TestDifferentBrandsKeepDifferentVariantKeys(t *testing.T) {
	different := [][2]string{
		{"ابيكوبريد", "اربييركس"},
		{"امينوفيلين", "امينوسلين"},
		{"بانادول", "بروفين"},
		{"زيثروماكس", "زوفيراكس"},
		{"اوجمنتين", "اوجمنت"},
	}
	for _, tc := range different {
		ka, kb := DebugVariantKey(tc[0]), DebugVariantKey(tc[1])
		if ka != "" && ka == kb {
			t.Errorf("%q and %q fold onto the same key %q; that links two different medicines",
				tc[0], tc[1], ka)
		}
	}
}

// Short words get no key at all. A skeleton drops every vowel, so three
// consonants collide freely and a coincidence at that length is not evidence.
func TestShortWordsGetNoVariantKey(t *testing.T) {
	for _, w := range []string{"دار", "دور", "دير", "gel", "زيت"} {
		if key := DebugVariantKey(w); key != "" {
			t.Errorf("%q produced variant key %q; too short to fold safely", w, key)
		}
	}
}

// The end-to-end claim: a row whose brand is spelled differently now resolves
// through the scorer, not just through the fold in isolation.
func TestVariantSpellingResolvesAgainstTheCatalogue(t *testing.T) {
	idx := NewIndex([]MasterProduct{
		{ID: 1, NameAR: "ابيكوبرايد 20 مجم أقراص", NameEN: "Apecopride 20mg Tablets",
			DosageForm: "أقراص", Concentration: "20 مجم"},
		{ID: 2, NameAR: "اربييركس 20 مجم أقراص", NameEN: "Arbeerex 20mg Tablets",
			DosageForm: "أقراص", Concentration: "20 مجم"},
	})

	// The pharmacy's spelling, one letter short of the catalogue's.
	res := idx.Match(&Row{Name: "ابيكوبريد 20 مجم اقراص"}, DefaultMatchOptions())

	if res.ProductID != 1 {
		t.Fatalf("matched product %d, want 1 (%s)", res.ProductID, res.Reason)
	}
	if !res.Level.Settled() {
		t.Errorf("level = %s at score %.2f; a one-letter spelling difference should not need a human",
			res.Level, res.Score)
	}
}

func TestCrossScriptNameResolvesThroughTheTokenChannel(t *testing.T) {
	idx := NewIndex([]MasterProduct{
		{ID: 7, NameAR: "", NameEN: "Panadol Extra Tablets"},
		{ID: 8, NameAR: "", NameEN: "Brufen 600mg Tablets"},
	})

	res := idx.Match(&Row{Name: "بانادول اكسترا اقراص"}, DefaultMatchOptions())
	if res.ProductID != 7 {
		t.Fatalf("matched product %d, want 7 (%s)", res.ProductID, res.Reason)
	}
}
