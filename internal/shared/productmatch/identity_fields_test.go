package productmatch

import "testing"

// The pairs below are the ones that were merging in the live catalogue. Each is
// a single brand whose members differ by one attribute, which is exactly the
// case a name comparison cannot see.
func TestDiscriminateSeparatesStrengths(t *testing.T) {
	cases := []struct {
		name     string
		a, b     ProductFields
		separate bool
	}{
		{
			name:     "same name, strength in its own column",
			a:        ProductFields{NameAR: "اوجمنتين", Concentration: "375 مجم"},
			b:        ProductFields{NameAR: "اوجمنتين", Concentration: "1 جم"},
			separate: true,
		},
		{
			name:     "same name, strength inside it",
			a:        ProductFields{NameAR: "اوجمنتين 375 مجم"},
			b:        ProductFields{NameAR: "اوجمنتين 1 جم"},
			separate: true,
		},
		{
			// One side states the dose in the name and the other in the column.
			// Reading fields rather than columns is what makes these agree.
			name:     "strength stated on different sides of the row",
			a:        ProductFields{NameAR: "اوجمنتين 375 مجم"},
			b:        ProductFields{NameAR: "اوجمنتين", Concentration: "375 مجم"},
			separate: false,
		},
		{
			name:     "catalogue silent about the dose",
			a:        ProductFields{NameAR: "اوجمنتين"},
			b:        ProductFields{NameAR: "اوجمنتين", Concentration: "1 جم"},
			separate: false,
		},
		{
			name:     "line extension",
			a:        ProductFields{NameAR: "بانادول"},
			b:        ProductFields{NameAR: "بانادول اكسترا"},
			separate: true,
		},
		{
			name:     "different pharmaceutical form",
			a:        ProductFields{NameAR: "بروفين", DosageForm: "شراب"},
			b:        ProductFields{NameAR: "بروفين", DosageForm: "اقراص"},
			separate: true,
		},
		{
			// Explicit tablets vs capsules conflict.
			name:     "tablets and capsules conflict",
			a:        ProductFields{NameAR: "افودارت", DosageForm: "اقراص"},
			b:        ProductFields{NameAR: "افودارت", DosageForm: "كبسولات"},
			separate: true,
		},
		{
			name:     "identical rows",
			a:        ProductFields{NameAR: "بانادول", Concentration: "500 مجم", DosageForm: "اقراص"},
			b:        ProductFields{NameAR: "بانادول", Concentration: "500 مجم", DosageForm: "اقراص"},
			separate: false,
		},
		{
			name:     "nothing stated on either side",
			a:        ProductFields{NameAR: "بانادول"},
			b:        ProductFields{NameAR: "بانادول"},
			separate: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Separable(tc.a, tc.b)
			if got != tc.separate {
				t.Fatalf("Separable = %v, want %v (conflict: %+v)",
					got, tc.separate, Discriminate(tc.a, tc.b))
			}
			// The comparison says the same thing whichever row is asked first.
			if Separable(tc.b, tc.a) != got {
				t.Fatalf("Separable is not symmetric for %q", tc.name)
			}
		})
	}
}

// Discriminate must agree with the indexed comparison the scorer and the
// adjudicator use, or the importer refuses pairs the rest of the platform
// accepts.
func TestDiscriminateAgreesWithIndexedConflict(t *testing.T) {
	products := []MasterProduct{
		{ID: 1, NameAR: "اوجمنتين", Concentration: "375 مجم"},
		{ID: 2, NameAR: "اوجمنتين", Concentration: "1 جم"},
		{ID: 3, NameAR: "بانادول اكسترا"},
	}
	idx := NewIndex(products)

	for _, p := range products {
		row := &Row{Name: "اوجمنتين", Concentration: "375 مجم"}
		fields := ProductFields{
			NameAR:        p.NameAR,
			Concentration: p.Concentration,
			DosageForm:    p.DosageForm,
		}
		rowFields := ProductFields{NameAR: row.Name, Concentration: row.Concentration}

		indexed := idx.IdentityConflict(row, p.ID)
		direct := Discriminate(rowFields, fields)

		// The indexed check also demands a shared distinctive word, which this
		// one deliberately leaves to the caller. Compare only the attribute
		// verdicts the two share.
		if indexed.Kind == "evidence" {
			continue
		}
		if (indexed.Kind == "") != (direct.Kind == "") {
			t.Fatalf("product %d: indexed conflict %q, direct conflict %q",
				p.ID, indexed.Kind, direct.Kind)
		}
	}
}
