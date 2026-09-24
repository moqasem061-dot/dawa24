package productmatch

import "testing"

func TestShortBrandKeyPairsShortBrandsAcrossScripts(t *testing.T) {
	for _, p := range [][2]string{
		{"Mobic 15mg", "موبيك 15 مجم"},
		{"Tareg 80mg", "تارج 80 مجم"},
	} {
		a, b := ShortBrandKey(p[0]), ShortBrandKey(p[1])
		if a == "" || a != b {
			t.Errorf("%q -> %q and %q -> %q; want one non-empty key", p[0], a, p[1], b)
		}
	}
}

// The dose is what tells two three-consonant brands apart, so a name without
// one must not produce a key at all.
func TestShortBrandKeyRequiresADose(t *testing.T) {
	if k := ShortBrandKey("Mobic"); k != "" {
		t.Errorf("a name with no dose keyed as %q", k)
	}
	if ShortBrandKey("Mobic 15mg") == ShortBrandKey("Mobic 7.5mg") {
		t.Error("two strengths of one brand share a key")
	}
}
