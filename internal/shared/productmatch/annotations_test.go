package productmatch

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// mustAmount parses a price for a test, failing rather than returning zero.
func mustAmount(t *testing.T, raw string) money.Amount {
	t.Helper()
	amount, err := money.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return amount
}

// TestStripTradeAnnotations pins the shape of an Egyptian distributor's sale
// terms — and, more importantly, the names that look like them and must survive.
func TestStripTradeAnnotations(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// The price, written every way a live file writes it.
		{"price with figure", "اتوريزا 10/20 مجم 21 قرص س 141 ج", "اتوريزا 10/20 مجم 21 قرص"},
		{"price word", "اوفيوسيدك 5ملم قطرة سعر 28ج", "اوفيوسيدك 5ملم قطرة"},
		{"bare price letters", "افيتركس جيل س ج2", "افيتركس جيل"},
		{"pounds abbreviated", "ارجوبرينو امبول سعر 120ج.م", "ارجوبرينو امبول"},
		{"trailing zero", "اوكتاترون3شريط س ج 0", "اوكتاترون3شريط"},
		{"glued to name", "الوبانتين لوسيون النيل س100", "الوبانتين لوسيون النيل"},
		{"price new", "وان الفا 1 جم سعر جديد", "وان الفا 1 جم"},
		{"price with slash", "وايديبريكس 60 مجم شراب/سعر جديد", "وايديبريكس 60 مجم شراب"},

		// The carton.
		{"carton after slash", "بانتولوك 20 مج 14 قرص ج/باكت 10", "بانتولوك 20 مج 14 قرص"},
		{"carton glued", "دلتافيت ب 12 اقراص/باكت128", "دلتافيت ب 12 اقراص"},
		{"carton word only", "توسيفان شراب باكت 20", "توسيفان شراب"},
		{"baku spelling", "ايزيس ينسون 12 فلتر صغير ج/ باكو 96", "ايزيس ينسون 12 فلتر صغير"},
		{"carton keeps strength", "سانسو دى 3 بلس اقراص 4000/باكت90", "سانسو دى 3 بلس اقراص 4000"},

		// The bare ب, which is a carton after a form and a vitamin after a brand.
		{"carton b after form", "ريكوكسبرايت 60 مج اقراص 3 شريط ب12", "ريكوكسبرايت 60 مج اقراص 3 شريط"},
		{"carton b after slash", "شوجارلو بلاس 50/1000 مج اقراص/ب 10", "شوجارلو بلاس 50/1000 مج اقراص"},
		{"vitamin b survives", "دلتافيت ب 12 اقراص", "دلتافيت ب 12 اقراص"},
		{"vitamin b glued survives", "دلتافيت ب12 تحت اللسان 1مجم 30 قرص", "دلتافيت ب12 تحت اللسان 1مجم 30 قرص"},

		// Names that must not be touched.
		{"plain name", "بانادول اكسترا 24 قرص", "بانادول اكسترا 24 قرص"},
		{"identity letter kept", "بتنوفيت ان كريم 30 جم", "بتنوفيت ان كريم 30 جم"},
		{"letter mid name kept", "امبيزيم-ج 30 قرص", "امبيزيم-ج 30 قرص"},
		{"combination dose intact", "اتاكاند بلس 32/25 مجم 28 قرص", "اتاكاند بلس 32/25 مجم 28 قرص"},
		{"nothing but terms", "س ج", "س ج"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StripTradeAnnotations(tc.in); got != tc.want {
				t.Errorf("StripTradeAnnotations(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestTradeAnnotationsDoNotDiscriminate is the failure this file exists for: a
// row and its catalogue entry agreeing on everything, refused over the price
// shorthand typed after the name.
func TestTradeAnnotationsDoNotDiscriminate(t *testing.T) {
	idx := NewIndex([]MasterProduct{
		{ID: 1, NameAR: "بانتولوك 20مجم 14 قرص", PublicPrice: "56.00"},
		{ID: 2, NameAR: "دياميكرون 60مجم 30 قرص", PublicPrice: "156.00"},
	})
	opts := DefaultMatchOptions()

	rows := []struct {
		name  string
		price string
		want  int64
	}{
		{"بانتولوك 20 مج 14 قرص ج/باكت 10", "56.00", 1},
		{"دياميكرون 60 مج اقراص ج ب100", "156.00", 2},
	}
	for _, tc := range rows {
		row := &Row{Number: 1, Name: tc.name, PublicPrice: mustAmount(t, tc.price)}
		got := MatchAll(idx, []*Row{row}, opts, 0)[0]
		if !got.Level.Settled() || got.ProductID != tc.want {
			t.Errorf("%q: level=%s product=%d score=%.2f reason=%s",
				tc.name, got.Level, got.ProductID, got.Score, got.Reason)
		}
	}
}

// TestPrintedPriceCorroborates checks that the printed price is read as
// evidence, and that it does not settle a match on its own.
func TestPrintedPriceCorroborates(t *testing.T) {
	idx := NewIndex([]MasterProduct{
		{ID: 1, NameAR: "جليماديل 3مجم 30 قرص", PublicPrice: "45.00"},
		{ID: 2, NameAR: "شيء اخر تماما 30 قرص", PublicPrice: "45.00"},
	})

	row := &Row{Number: 1, Name: "جليماديل 3 مجم 30 قرص", PublicPrice: mustAmount(t, "45.00")}
	got := MatchAll(idx, []*Row{row}, DefaultMatchOptions(), 0)[0]
	if got.ProductID != 1 {
		t.Fatalf("printed price did not corroborate: product=%d level=%s reason=%s",
			got.ProductID, got.Level, got.Reason)
	}

	// A row sharing only the price with a product whose name it does not carry
	// must still be refused.
	stranger := &Row{Number: 2, Name: "دواء لا وجود له اطلاقا", PublicPrice: mustAmount(t, "45.00")}
	if res := MatchAll(idx, []*Row{stranger}, DefaultMatchOptions(), 0)[0]; res.Level.Settled() {
		t.Errorf("price alone settled a match: product=%d reason=%s", res.ProductID, res.Reason)
	}
}

// TestDosageFormInferredByWord pins the substring bug: a brand name containing a
// form word is not that form.
func TestDosageFormInferredByWord(t *testing.T) {
	cases := map[string]string{
		"جليماديل 3 مجم 30 قرص":   "أقراص",
		"جلوكوفاج 1000مجم 30 قرص": "أقراص",
		"جليبتس بلس 30 قرص":       "أقراص",
		"افيتركس جيل 100 جم":      "جل",
		"اوروفيكس غسول فم 250 مل": "غسول فم",
		"بروفين 400 مج 20ق":       "أقراص",
		"ماركة بلا شكل":           DefaultDosageForm,
	}
	for name, want := range cases {
		if got := InferDosageForm(name); got != want {
			t.Errorf("InferDosageForm(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestStripCountIsNotTabletCount pins that a pack counted in strips cannot
// contradict the same pack counted in tablets.
func TestStripCountIsNotTabletCount(t *testing.T) {
	idx := NewIndex([]MasterProduct{
		{ID: 1, NameAR: "ديكلاك 75مجم 30 قرص", PublicPrice: "135.00"},
	})
	row := &Row{Number: 1, Name: "ديكلاك 75 مج اقراص 3 شريط", PublicPrice: mustAmount(t, "135.00")}
	got := MatchAll(idx, []*Row{row}, DefaultMatchOptions(), 0)[0]
	if !got.Level.Settled() {
		t.Errorf("strip count contradicted a tablet count: level=%s reason=%s", got.Level, got.Reason)
	}
}

// TestDashWrittenCombination pins that a combination typed with a dash is the
// same combination the catalogue writes with a slash.
func TestDashWrittenCombination(t *testing.T) {
	for _, written := range []string{"10-10", "10 - 10"} {
		set := strengthSet(written + " مجم")
		if len(set) == 0 {
			t.Fatalf("%q: no strength read", written)
		}
		if set[0].parts != 2 {
			t.Errorf("%q: parts = %d, want 2", written, set[0].parts)
		}
	}
	// A dash between letters and a figure is a name separator, not a ratio.
	if got := FoldDoseText("اوست - ماب 60 مجم"); got != "اوست - ماب 60 مجم" {
		t.Errorf("name separator folded: %q", got)
	}
}
