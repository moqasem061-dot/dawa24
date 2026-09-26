package productmatch

import (
	"testing"
)

func conflictKinds(t *testing.T, row string, product MasterProduct) []string {
	t.Helper()
	idx := NewIndex([]MasterProduct{product})
	return ConflictsWith(idx, &Row{Name: row}, product.ID)
}

func hasKind(kinds []string, want string) bool {
	for _, k := range kinds {
		if k == want {
			return true
		}
	}
	return false
}

// Each case is a wrong match the labelled benchmark caught, or the correct
// match the fix must not break.
func TestIdentityRefinements(t *testing.T) {
	cases := []struct {
		name    string
		row     string
		product string
		kind    string // the conflict that must fire; "" means none may
	}{
		// Gluing across a word boundary invented modifiers.
		{"men is not energy", "centrum men 30 tabs", "سنتروم انرجي 30 قرص", "modifier"},
		{"woman is not man", "dozova woman max 30 f.c. tabs", "دوزوفا مان ماكس 30 قرص", "modifier"},
		{"women line in Arabic is not men", "ليميتلس وومان ماكس 30 قرص", "ليميتلس مان ماكس 30 قرص", "modifier"},
		// ...while a modifier genuinely fused into one token still agrees.
		{"glued plus", "اكوا بلس شراب 100 مل", "اكوابلس شراب 100 مل", ""},
		{"inflected advanced", "advanced omega 30 gummies", "ادفانسد اوميجا 30 قطعة", ""},
		{"article before modifier", "ليليبل 5مجم 20 قرص مضغ", "ليليبل 5مجم 20 قرص قابل المضغ", ""},

		// Route.
		{"iv is not im", "سيفاكسوتريز 1 جم وريد فيال", "سيفاكسوتريز 1 جم عضل فيال", "route"},
		{"latin iv is not im", "cefaxotrez 1 gm i.v. vial", "سيفاكسوتريز 1 جم عضل فيال", "route"},
		{"both routes agree with one", "cefozon 500mg i.m./i.v. vial", "سيفوزون 500مجم وريد فيال", ""},
		{"unstated route is silence", "cefozon 500mg i.m. vial", "سيفوزون 500مجم فيال", ""},

		// Size.
		{"sheet sizes differ", "سكار اف اكس سيليكون شيت 2.5*2.5سم", "سكار اف اكس سيليكون شيت 10*20سم", "size"},
		{"same size either order", "شيت سيليكون 20*10 سم", "شيت سيليكون 10*20سم", ""},

		// Sub-form without a main form.
		{"oil is not cream", "active hair oil 120 ml", "اكتيف هير كريم 100 جم", "sub_form"},
		{"serum is not cream", "greatier hair serum 120 ml", "جريتر هير كريم 100 جم", "sub_form"},
		{"wash spellings still agree", "aqua dent mouth wash 250 ml", "اكوا دنت مضمضة للفم 250 مل", ""},

		// Hamza in a unit word.
		{"hamza grams", "بيبي كير كريم 30 جرأم", "بيبي كير كريم 75 جرام", "strength"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kinds := conflictKinds(t, c.row, MasterProduct{ID: 1, NameAR: c.product})
			switch {
			case c.kind == "" && hasAny(kinds, "modifier", "route", "size", "sub_form", "strength"):
				t.Errorf("%q vs %q: unexpected conflict %v", c.row, c.product, kinds)
			case c.kind != "" && !hasKind(kinds, c.kind):
				t.Errorf("%q vs %q: conflicts %v, want %q", c.row, c.product, kinds, c.kind)
			}
		})
	}
}

func hasAny(kinds []string, wants ...string) bool {
	for _, w := range wants {
		if hasKind(kinds, w) {
			return true
		}
	}
	return false
}

// A carton is still a carton. The size rule must not stop "20*10 قرص" folding
// into the two hundred tablets the catalogue records.
func TestCartonsStillFold(t *testing.T) {
	q := readQuantities("اليرجيل 4مجم 20*10 قرص")
	found := false
	for _, c := range q.counts {
		if c.value == 200 {
			found = true
		}
	}
	if !found {
		t.Fatalf("20*10 tablets did not fold to 200: %+v", q.counts)
	}
	if q := readQuantities("شيت 10*20سم"); len(q.counts) != 0 {
		t.Fatalf("a size was read as counts: %+v", q.counts)
	}
}

// The AI stage's guard sees route and size too.
func TestIdentityConflictReadsRouteAndSize(t *testing.T) {
	idx := NewIndex([]MasterProduct{
		{ID: 1, NameAR: "سيفاكسوتريز 1 جم عضل فيال"},
		{ID: 2, NameAR: "سكار اف اكس سيليكون شيت 10*20سم"},
	})
	if idx.IdentityConflict(&Row{Name: "سيفاكسوتريز 1 جم وريد فيال"}, 1).None() {
		t.Error("the guard let an IV vial settle on an IM one")
	}
	if idx.IdentityConflict(&Row{Name: "سكار اف اكس سيليكون شيت 2.5*2.5سم"}, 2).None() {
		t.Error("the guard let a 2.5 cm sheet settle on a 10x20 one")
	}
}
