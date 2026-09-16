package productmatch

import (
	"regexp"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Inference from the product name.
//
// The great majority of Egyptian supplier files carry three columns — name,
// price, discount — and nothing else. The pharmaceutical form and the strength
// are in the name, written the way a pharmacist would say them, and pulling
// them out is the difference between a catalogue that can be filtered and a
// catalogue that can only be scrolled.
//
// Only the form is a genuine inference. The concentration is quoted out of the
// name verbatim, so it states what the supplier wrote rather than guessing on
// their behalf.

// dosageKeywords maps words that appear in an Egyptian product name onto the
// pharmaceutical form. Longer, more specific entries come first: "غسول فم" must
// beat "غسول", and "معجون أسنان" must beat any bare match.
var dosageKeywords = []struct {
	words []string
	form  string
}{
	{[]string{"غسول فم", "مضمضة", "مضمضه", "mouthwash"}, "غسول فم"},
	{[]string{"معجون اسنان", "معجون أسنان", "toothpaste"}, "معجون أسنان"},
	{[]string{"غسول", "wash", "lotion", "لوشن"}, "غسول"},
	{[]string{"كريم", "cream"}, "كريم"},
	{[]string{"مرهم", "ointment", "oint"}, "مرهم"},
	{[]string{"جل", "جيل", "gel"}, "جل"},
	{[]string{"زيت", "oil", "اويل"}, "زيت"},
	{[]string{"سيروم", "serum"}, "سيروم"},
	{[]string{"شامبو", "shampoo"}, "شامبو"},
	{[]string{"صابون", "صابونة", "صابونه", "soap"}, "صابون"},
	{[]string{"رول اون", "roll on", "رول-اون"}, "رول اون"},
	{[]string{"اسبراي", "سبراي", "اسبراى", "سبراى", "بخاخ", "spray", "بدى ميست", "body mist"}, "بخاخ / اسبراي"},
	{[]string{"مناديل", "wipes"}, "مناديل مبللة"},
	{[]string{"صبغة", "صبغه", "hair color", "colour"}, "صبغة شعر"},
	{[]string{"اقراص", "أقراص", "قرص", "ق ", "tab", "tabs", "tablet", "tablets"}, "أقراص"},
	{[]string{"كبسول", "كبسولات", "كبسولة", "كبسوله", "cap", "caps", "capsule", "capsules"}, "كبسولات"},
	{[]string{"شراب", "syrup", "susp", "معلق"}, "شراب"},
	{[]string{"نقط", "drops", "قطرة", "قطره"}, "نقط"},
	{[]string{"حقن", "حقنة", "حقنه", "امبول", "أمبول", "امبولات", "فيال", "vial", "ampoule", "inj"}, "حقن وأمبولات"},
	{[]string{"فوار", "ساشيت", "sachet", "eff"}, "أكياس فوار"},
	{[]string{"لبوس", "تحاميل", "تحميلة", "supp", "suppository"}, "لبوس"},
	{[]string{"حفاضات", "حفاضه", "diapers"}, "مستلزمات عناية"},
	{[]string{"استيك", "ستيك", "stick"}, "ستيك"},
	{[]string{"بودرة", "بودره", "powder", "بودر"}, "بودرة"},
	{[]string{"شريط", "strip"}, "أقراص"},
}

// DefaultDosageForm labels a product whose name gives no clue about its form.
const DefaultDosageForm = "مستحضر صيدلاني"

// strengthPattern matches a dose written the way it is printed on a box — and
// the several ways an Egyptian distributor abbreviates it in a spreadsheet.
//
// "مج" and "مغ" are milligrams and appear throughout real supplier files
// ("بروفين 400 مج اقراص"); "محم" is the ج/ح keyboard slip for مجم and appears
// often enough in live data to be worth reading rather than discarding. Missing
// any of them does not merely lose the strength — it leaves the figure loose in
// the name, where it competes with the brand as an ordinary token and the
// strength veto that keeps 400 mg away from 600 mg never fires.
//
// The alternation is longest-first: Go's regexp prefers the leftmost-first
// branch, so "مج" listed before "مجم" would match the first two letters of
// "مجم" and leave a stray "م" behind.
//
// Two units were missing from it while being present in doseUnits, which is
// worse than either being absent from both:
//
//   - "ملجم" is the commonest Arabic spelling of a milligram in supplier files.
//     Absent from the alternation, it matched its first two letters as "مل" and
//     the engine read "اتاكاند بلس 32/25 ملجم" as thirty-two MILLILITRES. A
//     strength recorded in the wrong dimension cannot contradict anything: it
//     compares against bottle sizes and is invisible to every real dose.
//   - "لتر" was simply not readable, so "1 لتر" stated no strength at all.
//
// The list is kept in step with doseUnits deliberately: a unit in one and not
// the other is silently wrong rather than loudly missing.
//
// The numeric head repeats (`*`, not `?`) so that a whole combination reaches
// strengthSet in one piece. With `?` the pattern could hold two figures at
// most, and "املوسازايد 5/12.5/40مجم" matched from the middle — the engine read
// a three-component combination as a single figure and could not tell it from
// its 5/12.5/20 sibling. Every figure of the ratio is expanded by
// numericComponents; nothing here decides what they mean.
var strengthPattern = regexp.MustCompile(
	`(?i)(\d+(?:[./]\d+)*\s*(?:مليجرام|ملجرام|جرام|مكجم|ملجم|مللي|وحدة|وحده|مجم|محم|ملي|لتر|مج|مغ|جم|مل|mcg|spf[+\d]*|mg|gm|ml|iu|%|g|l))`)

// InferDosageForm reads the pharmaceutical form out of a product name.
//
// By WORD, never by substring. It used to ask strings.Contains of the whole
// name, and an Egyptian brand name contains a form word by coincidence far more
// often than one would guess: جليماديل carries جل, so every strength of a
// diabetes tablet was filed as a gel — and, because the row's inferred form is
// read back as evidence, contradicted the catalogue's own "30 قرص" and refused
// a row whose name, dose, pack count and printed price all agreed exactly.
// جلوكوفاج, جليمباكير and جليبتس failed the same way, as does anything holding
// كريم, زيت or شريط inside a longer word.
//
// Words are compared as letter runs so the Egyptian habit of gluing the count
// to the form still reads: "30قرص" and "20ق" are a figure and a form word, not
// one token.
func InferDosageForm(name string) string {
	if name == "" {
		return DefaultDosageForm
	}
	words := dosageWords(name)
	if len(words) == 0 {
		return DefaultDosageForm
	}
	for _, dk := range dosageKeywords {
		for _, w := range dk.words {
			if containsPhrase(words, strings.Fields(sheet.NormalizeName(w))) {
				return dk.form
			}
		}
	}
	return DefaultDosageForm
}

// dosageWords reduces a name to the words a form keyword can match, splitting
// the figures off the words they were typed against.
func dosageWords(name string) []string {
	var out []string
	for _, w := range strings.Fields(sheet.NormalizeName(name)) {
		if hasDigit(w) {
			out = append(out, letterRuns(w)...)
			continue
		}
		out = append(out, w)
	}
	return out
}

// containsPhrase reports whether a keyword's words appear consecutively among a
// name's words. A one-word keyword is the ordinary case; the multi-word entries
// — "غسول فم", "معجون اسنان", "رول اون" — are why this is a phrase search and
// not a set lookup, and why they must be tried before their shorter neighbours.
func containsPhrase(words, phrase []string) bool {
	if len(phrase) == 0 || len(phrase) > len(words) {
		return false
	}
	for i := 0; i+len(phrase) <= len(words); i++ {
		matched := true
		for j, w := range phrase {
			if words[i+j] != w {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// InferConcentration quotes the strength out of a product name, or returns
// empty when the name states none.
func InferConcentration(name string) string {
	if name == "" {
		return ""
	}
	if m := strengthPattern.FindString(FoldDoseText(sheet.NormalizeDigits(name))); m != "" {
		return sheet.CleanCell(m)
	}
	return ""
}

// packPattern matches the pack count Egyptian names carry: "30 قرص", "14كبسولة",
// "3 امبول", "20ق".
var packPattern = regexp.MustCompile(
	`(?i)(\d{1,4})\s*(قرص|اقراص|أقراص|ق|كبسول|كبسوله|كبسولة|كبسولات|امبول|أمبول|امبولة|امبولات|كيس|اكياس|شريط|لبوس|tab|tabs|cap|caps|amp)\b`)

// InferPackSize reads the number of units in the pack out of a product name.
//
// It is used only where the file has no pack-size column, and only for display
// and matching — never to derive a price. A pack count read wrongly out of a
// name would otherwise silently divide or multiply what a pharmacy pays.
func InferPackSize(name string) int {
	m := packPattern.FindStringSubmatch(sheet.NormalizeDigits(name))
	if len(m) < 2 {
		return 0
	}
	n, err := sheet.CoerceInt(m[1])
	if err != nil || n <= 0 || n > 5000 {
		return 0
	}
	return int(n)
}
