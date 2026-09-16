package pipeline

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

// Decomposition has to work on Arabic, which is the only script the files
// actually arrive in.
//
// It did not. Both patterns ended in `\b`, which Go defines on ASCII word
// characters only — after "مجم" there is no \w-to-\W transition, so the
// boundary never matched and neither pattern found anything in an Arabic line.
// The strength and the pack count stayed in the name, competing with the brand
// as ordinary tokens, and the strength veto that keeps 400 mg away from 600 mg
// never had a strength to compare.
//
// These cases are taken from a real pharmacy order file.
func TestArabicLinesAreDecomposed(t *testing.T) {
	tests := []struct {
		raw      string
		name     string
		strength string
		form     string
		pack     int
	}{
		{
			raw:      "بروفين 400 مجم 30 قرص",
			name:     "بروفين",
			strength: "400 مجم",
			form:     "أقراص",
			pack:     30,
		},
		{
			raw:      "بيتاسيرك 8 مجم 100 قرص",
			name:     "بيتاسيرك",
			strength: "8 مجم",
			form:     "أقراص",
			pack:     100,
		},
		{
			// "مج" for milligrams, written throughout Egyptian price lists.
			raw:      "ليبانتيل 300 مج اقراص",
			name:     "ليبانتيل",
			strength: "300 مج",
			form:     "أقراص",
			pack:     0,
		},
		{
			// Therapeutic prose is dropped: the catalogue never carries it, and
			// left in it drags a correct match below the floor.
			raw:      "بروفين مسكن ومضاد للالتهاب",
			name:     "بروفين ومضاد",
			strength: "",
			form:     "",
			pack:     0,
		},
		{
			// Cosmetics/OTC with Egyptian pack abbreviation 17ق
			raw:      "كيور ايد بلاستر كالو 17ق",
			name:     "كيور ايد كالو",
			strength: "",
			form:     "بلاستر",
			pack:     17,
		},
		{
			// Hair care / cosmetics product
			raw:      "سيروبايب بلسم للشعر 300 مل",
			name:     "سيروبايب",
			strength: "300 مل",
			form:     "بلسم",
			pack:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			row := BuildRow(&smartorder.Line{RawName: tc.raw})
			if row.Name != tc.name {
				t.Errorf("name = %q, want %q", row.Name, tc.name)
			}
			if row.Concentration != tc.strength {
				t.Errorf("concentration = %q, want %q", row.Concentration, tc.strength)
			}
			if row.DosageForm != tc.form {
				t.Errorf("dosage form = %q, want %q", row.DosageForm, tc.form)
			}
			if row.PackSize != tc.pack {
				t.Errorf("pack size = %d, want %d", row.PackSize, tc.pack)
			}
		})
	}
}
