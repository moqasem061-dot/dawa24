package productmatch

import "testing"

// The compare tool groups two SUPPLIERS' price lists against each other, with
// no catalogue involved. Every pair here is a real failure of the private
// matcher this key replaced.

// Two spellings of one product must produce one key.
//
// The matcher this replaced could only cross alphabets through a hand-written
// table of about sixty brands. Everything outside it arrived as two rows with
// one offer each — the one outcome a price comparison must never produce.
func TestProductKeyGroupsOneProductWrittenTwoWays(t *testing.T) {
	for _, p := range [][2]string{
		// The ratio, in either order.
		{"الكور بلس 10/20 مجم 14 قرص", "الكور بلس 20/10 مجم 14 قرص"},
		// Cross-script, for brands no hand-written table listed.
		{"سيفيديم 500 مجم فيال", "cefidime 500mg vial"},
		{"زيرتك 10 مجم 20 قرص", "zyrtec 10mg 20 tabs"},
		{"Cataflam 50 mg 20 tab", "كتافلام 50 مجم 20 قرص"},
		{"Augmentin 1gm 14 tab", "اوجمنتين 1 جم 14 قرص"},
		// Egyptian spelling variation within one alphabet.
		{"ابيكوبريد 40 مجم", "ابيكوبرايد 40 مجم"},
		// A line extension written in either alphabet.
		{"بانادول اكسترا 24 قرص", "panadol extra 24 tab"},
		// An identity letter written in either alphabet.
		{"بتنوفيت ان كريم", "betnovate n cream"},
		// Words in either order: it is the same box.
		{"ليميتلس اولزايم ماكس 20 قرص", "اولزايم ليميتلس ماكس 20 قرص"},
		// A pack written as its factors.
		{"allergyl 4 mg 20*10 tabs", "اليرجيل 4 مجم 200 قرص"},
	} {
		a, b := ProductKey(p[0]), ProductKey(p[1])
		if a == "" {
			t.Errorf("%q produced no key at all", p[0])
			continue
		}
		if a != b {
			t.Errorf("these are one product and must group:\n  %-34q => %s\n  %-34q => %s",
				p[0], a, p[1], b)
		}
	}
}

// Two different products must never produce one key.
//
// A wrong merge is worse than a missed one: the screen shows a single line
// carrying offers for two different medicines, and its "best price" is the
// cheaper drug rather than the cheaper supplier.
func TestProductKeySeparatesDifferentProducts(t *testing.T) {
	for _, p := range [][2]string{
		{"اتاكاند 16 مجم", "اتاكاند 32 مجم"},
		// Both strengths written WITHOUT their unit, which every Egyptian price
		// list does somewhere. The old key discarded any bare figure of three
		// digits or fewer, so these merged.
		{"اتاكاند 16", "اتاكاند 32"},
		// One component of the combination differs.
		{"الكور بلس 10/20 مجم", "الكور بلس 10/40 مجم"},
		// The identity letter is the whole difference.
		{"بتنوفيت ان كريم", "بتنوفيت سي كريم"},
		{"betnovate n cream", "betnovate c cream"},
		// A line extension is a different product.
		{"بانادول 24 قرص", "بانادول اكسترا 24 قرص"},
		// Same brand, different dose, written in different units.
		{"اوجمنتين 1 جم", "اوجمنتين 625 مجم"},
		// Same everything, different pack.
		{"اليرجيل 4 مجم 20 قرص", "اليرجيل 4 مجم 200 قرص"},
	} {
		if a, b := ProductKey(p[0]), ProductKey(p[1]); a == b {
			t.Errorf("these are two products and must not group: %q and %q both key as %s",
				p[0], p[1], a)
		}
	}
}

// A name with nothing identifying in it gets no key, and callers must not group
// on it — an empty key shared by every unreadable line would merge them all.
func TestProductKeyRefusesToKeyNothing(t *testing.T) {
	for _, name := range []string{"", "   ", "قرص", "24 قرص", "مجم"} {
		if got := ProductKey(name); got != "" {
			t.Errorf("ProductKey(%q) = %q, want an empty key", name, got)
		}
	}
}
