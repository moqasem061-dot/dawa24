package productmatch

import "testing"

// The screen of wrong matches this gate was written for.
//
// Every pair below was produced by the live engine against a real supplier
// file, offered to the vendor with a product name beside it and a percentage in
// the teens. They share a packaging noun and nothing else. None of them may
// come back with a product attached.
func TestMatchRefusesPackagingOnlyAgreement(t *testing.T) {
	// A catalogue with enough entries carrying the packaging vocabulary that it
	// reads as common, which is what the live catalogue looks like.
	catalogue := []MasterProduct{
		{ID: 1, NameAR: "نو كال 300 باكت"},
		{ID: 2, NameAR: "بولي فريش ادفانس قطرة عين"},
		{ID: 3, NameAR: "رابيدا اكياس للتخسيس"},
		{ID: 4, NameAR: "نيوروبيونا الأصلي 30 قرص"},
	}
	for i := 0; i < 60; i++ {
		catalogue = append(catalogue, MasterProduct{
			ID:     int64(100 + i),
			NameAR: "مستحضر رقم " + itoa(i) + " باكت",
		})
	}
	idx := NewIndex(catalogue)
	opts := DefaultMatchOptions()

	refused := []string{
		"اجارتوب نقط/باكت96",
		"ارياس اقراص باكت 160",
		"بنجرايد اقراص/باكت 45",
		"رامو صابون /باكت 36",
		"سولوسبت محلول/باكت 30",
	}
	for _, name := range refused {
		res := idx.Match(&Row{Name: name}, opts)
		if res.Matched() {
			t.Errorf("%q matched product %d at %.2f; a shared packaging noun is not evidence",
				name, res.ProductID, res.Score)
		}
		if len(res.Candidates) > 0 {
			t.Errorf("%q offered %d candidates; nothing plausible exists for it",
				name, len(res.Candidates))
		}
	}
}

// The other half of the requirement: tightening the gate must not cost the
// matches the tool exists to make.
func TestMatchStillFindsRealProducts(t *testing.T) {
	idx := NewIndex([]MasterProduct{
		{ID: 1, NameAR: "بانادول اكسترا 24 قرص"},
		{ID: 2, NameAR: "بانادول نايت 20 قرص"},
		{ID: 3, NameAR: "ابيكوبرايد 20مجم 20 قرص"},
		{ID: 4, NameAR: "اوجمنتين 1 جم 14 قرص"},
	})
	opts := DefaultMatchOptions()

	cases := []struct {
		name string
		want int64
	}{
		{"بانادول اكسترا 24 قرص سعر جديد", 1},
		{"ابيكوبريد 20مجم 20قرص", 3},
		{"أوجمنتين ١ جم ١٤ قرص", 4},
	}
	for _, c := range cases {
		res := idx.Match(&Row{Name: c.name}, opts)
		if res.ProductID != c.want {
			t.Errorf("%q matched %d (%.2f, %s), want %d",
				c.name, res.ProductID, res.Score, res.Level, c.want)
		}
	}
}

// A line extension is a different product, and the deterministic scorer must
// say so on its own rather than leaving it to the AI guard.
func TestModifierMismatchIsNotSettled(t *testing.T) {
	idx := NewIndex([]MasterProduct{
		{ID: 1, NameAR: "بانادول اكسترا 24 قرص"},
	})
	res := idx.Match(&Row{Name: "بانادول نايت 24 قرص"}, DefaultMatchOptions())
	if res.Level.Settled() {
		t.Fatalf("بانادول نايت settled onto بانادول اكسترا at %.2f (%s)", res.Score, res.Level)
	}
}
