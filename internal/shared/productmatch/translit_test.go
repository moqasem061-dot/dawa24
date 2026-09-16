package productmatch

import "testing"

// TestSkeletonBridgesScripts is the case the bridge exists for: every catalogue
// product carries an English name, every supplier file is Arabic, and before
// this the two could not be compared at all.
func TestSkeletonBridgesScripts(t *testing.T) {
	pairs := []struct{ ar, en string }{
		{"بوباي صن سكرين", "bobai sunscreen"},
		{"بانادول اكسترا", "panadol extra"},
		{"اوجمنتين", "augmentin"},
		{"كونجستال", "congestal"},
		{"ابيليفاي", "abilify"},
		{"فولتارين", "voltaren"},
		{"زيثروماكس", "zithromax"},
	}
	for _, p := range pairs {
		a, b := Skeleton(p.ar), Skeleton(p.en)
		if got := skeletonSimilarity(a, b); got < 0.5 {
			t.Errorf("%q (%s) vs %q (%s): similarity %.2f, want >= 0.5",
				p.ar, a, p.en, b, got)
		}
	}
}

// TestSkeletonSurvivesArabicSpellingVariation covers the variation Egyptians
// actually produce: long vowels written or dropped, ى against ي, ة against ه,
// and the same word split or joined.
func TestSkeletonSurvivesArabicSpellingVariation(t *testing.T) {
	same := [][2]string{
		{"بنادول", "بانادول"},
		{"ابليفاى", "ابيليفاي"},
		{"صن سكرين", "صنسكرين"},
		{"ازرجا", "ازارجا"},
	}
	for _, p := range same {
		if a, b := Skeleton(p[0]), Skeleton(p[1]); a != b {
			t.Errorf("%q -> %q and %q -> %q should reduce alike", p[0], a, p[1], b)
		}
	}
}

// TestSkeletonKeepsDifferentBrandsApart is the guard that matters more than the
// matches: a lossy reduction that collapses two real products into one identity
// would price the wrong medicine.
func TestSkeletonKeepsDifferentBrandsApart(t *testing.T) {
	different := [][2]string{
		{"بانادول", "كونجستال"},
		{"اوجمنتين", "فولتارين"},
		{"زيثروماكس", "بروفين"},
	}
	for _, p := range different {
		a, b := Skeleton(p[0]), Skeleton(p[1])
		if a == b {
			t.Errorf("%q and %q both reduce to %q", p[0], p[1], a)
		}
		if got := skeletonSimilarity(a, b); got >= 0.5 {
			t.Errorf("%q vs %q: similarity %.2f is too high for different products",
				p[0], p[1], got)
		}
	}
}

// TestSkeletonIgnoresFiguresAndPunctuation checks that a strength glued to the
// brand does not change its identity. The figures are compared separately as a
// number signature, and a skeleton that carried them would make "بروفين600" a
// different product from "brufen 600 mg".
func TestSkeletonIgnoresFiguresAndPunctuation(t *testing.T) {
	if a, b := Skeleton("بروفين600"), Skeleton("بروفين"); a != b {
		t.Errorf("figures changed the skeleton: %q vs %q", a, b)
	}
	if a, b := Skeleton("panadol-extra"), Skeleton("panadol extra"); a != b {
		t.Errorf("punctuation changed the skeleton: %q vs %q", a, b)
	}
}

// TestSkeletonRefusesTooShort guards the coincidence rate. Two consonants agree
// far too often to be evidence of anything.
func TestSkeletonRefusesTooShort(t *testing.T) {
	if got := skeletonOf([]string{"دار"}); got != "" {
		t.Errorf("a two-consonant skeleton should not be offered, got %q", got)
	}
}
