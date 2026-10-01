package productmatch_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestSameSellableItem_IdenticalAndEquivalent(t *testing.T) {
	cases := []struct {
		name      string
		rowA      string
		rowB      string
		rels      []productmatch.PackRelationship
		wantSame  bool
		wantBasis string
	}{
		{
			name:      "identical name and pack",
			rowA:      "كتافلام 50 مجم 20 قرص",
			rowB:      "كتافلام 50 مجم 20 قرص",
			wantSame:  true,
			wantBasis: "source_equivalence",
		},
		{
			name:      "identical English with minor wording variations",
			rowA:      "Panadol Extra 24 tablets",
			rowB:      "Panadol Extra 24 tabs",
			wantSame:  true,
			wantBasis: "source_equivalence",
		},
		{
			name:      "bare number vs word count",
			rowA:      "بانادول اكسترا 24 قرص",
			rowB:      "بانادول اكسترا 24",
			wantSame:  true,
			wantBasis: "source_equivalence",
		},
		{
			name:      "brand conflict",
			rowA:      "فولتارين 50 مجم 20 قرص (نوفارتس)",
			rowB:      "كتافلام 50 مجم 20 قرص (نوفارتس)",
			wantSame:  false,
			wantBasis: "",
		},
		{
			name:      "strength conflict",
			rowA:      "أموكسيسيلين 500 مجم 12 كبسول",
			rowB:      "أموكسيسيلين 250 مجم 12 كبسول",
			wantSame:  false,
			wantBasis: "",
		},
		{
			name:      "modifier conflict",
			rowA:      "بانادول اكسترا 24 قرص",
			rowB:      "بانادول كولد اند فلو 24 قرص",
			wantSame:  false,
			wantBasis: "",
		},
		{
			name:      "modifier unstated in one",
			rowA:      "بانادول 24 قرص",
			rowB:      "بانادول اكسترا 24 قرص",
			wantSame:  false,
			wantBasis: "",
		},
		{
			name:      "strip vs box without relationship",
			rowA:      "كونجستال 10 اقراص شريط",
			rowB:      "كونجستال 20 قرص علبة",
			wantSame:  false,
			wantBasis: "",
		},
		{
			name: "strip vs box with valid relationship",
			rowA: "كونجستال 2 شريط",
			rowB: "كونجستال 1 علبة",
			rels: []productmatch.PackRelationship{
				{
					ID:       101,
					FromUnit: "علبة",
					ToUnit:   "شريط",
					Factor:   2,
				},
			},
			wantSame:  true,
			wantBasis: "relationship",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			factsA := productmatch.ExtractRowFacts(&productmatch.Row{Name: tc.rowA}, nil)
			factsB := productmatch.ExtractRowFacts(&productmatch.Row{Name: tc.rowB}, nil)

			same, basis := productmatch.SameSellableItem(factsA, factsB, tc.rels)
			if same != tc.wantSame {
				compAB := productmatch.CompareFacts(factsA, factsB)
				compBA := productmatch.CompareFacts(factsB, factsA)
				t.Errorf("SameSellableItem() same = %v, want %v (factsA: %+v, Pack: %+v; factsB: %+v, Pack: %+v; HardAB: %v; HardBA: %v; AttrAB: %v)",
					same, tc.wantSame, factsA, factsA.Pack, factsB, factsB.Pack, compAB.Hard, compBA.Hard, compAB.Attributes)
			}
			if basis != tc.wantBasis {
				t.Errorf("SameSellableItem() basis = %q, want %q", basis, tc.wantBasis)
			}
		})
	}
}
