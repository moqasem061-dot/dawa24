package productmatch

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// coreTokens reduces a product name to the words that identify it.
//
// The pharmaceutical furniture is dropped — the form, the unit, the pack count,
// the marketing noise a distributor appends ("سعر جديد", "س ج", "**") — leaving
// the brand and the molecule. Without this, "بانادول اكسترا 24 قرص" and
// "بانادول اكسترا 24 قرص سعر جديد" score as different products, and every
// product ending in "أقراص" scores as similar to every other.
func coreTokens(name string) []string {
	if name == "" {
		return nil
	}
	words := strings.Fields(sheet.NormalizeName(name))
	out := make([]string, 0, len(words))
	for _, w := range words {
		if noiseWords[w] {
			continue
		}
		if _, ok := formWords[w]; ok {
			continue
		}
		if _, ok := unitWords[w]; ok {
			continue
		}
		// A bare number is a pack count or a dose already captured separately.
		//
		// Tested by shape rather than by strconv.ParseFloat. Parsing was only
		// ever being used as a predicate here, and a failed parse allocates an
		// error value — on the overwhelming majority of words, which are not
		// numbers. Across a hundred and fifty thousand products that was 17 MB
		// of errors constructed, examined for nil, and discarded.
		if isNumericLiteral(w) {
			continue
		}
		// So is a number glued to its unit. Arabic price lists write "20مجم"
		// and "20قرص" without a space, and normalisation does not split them,
		// so they arrived here looking like ordinary words. They are not: they
		// are the strength and the pack count, both already compared as
		// structured fields — and being common they carried more weight than
		// the brand beside them. That is how "اربييركس 20مجم 20قرص" scored 98%
		// against "ابيكوبريد 20مجم 20قرص": two of the three words agreed, and
		// the one that did not was the only one that identified the medicine.
		if hasDigit(w) {
			out = appendLetterRuns(out, w)
			continue
		}
		if len([]rune(w)) < 2 {
			continue
		}
		out = append(out, w)
	}
	return out
}

// isNumericLiteral reports whether a word is nothing but a number.
//
// Deliberately narrower than strconv: no exponents, no signs, no underscores.
// A product name does not contain "1e5", and accepting it would drop a token
// that might be a brand.
func isNumericLiteral(w string) bool {
	if w == "" {
		return false
	}
	digits, dots := 0, 0
	for _, r := range w {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '.' || r == ',':
			dots++
			if dots > 1 {
				return false
			}
		default:
			return false
		}
	}
	return digits > 0
}

func hasDigit(w string) bool {
	for _, r := range w {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

// appendLetterRuns splits a token that mixes digits and letters, keeping the
// letters and discarding the figures.
//
// Arabic price lists glue them in every direction: "20مجم", "ليبانتيل300",
// "اقراص3شريط", "بروفين600مجم". Left whole, each of those is a token no other
// spelling of the same product shares, so two entries for one medicine score as
// strangers — and, worse, the common ones like "20مجم" are shared by thousands
// of products and outvoted the brand beside them.
//
// The figures are not lost: the strength is parsed as a number and compared as
// one, and every other figure is in the row's number signature. What is dropped
// here is only their pretence to be words.
//
// A run of one letter is dropped with the rest, which costs the "b" of "b12" —
// acceptable, because the 12 survives in the number signature and the products
// that case distinguishes are separated by it anyway.
func appendLetterRuns(out []string, w string) []string {
	runes := []rune(w)
	start := -1
	flush := func(end int) {
		if start < 0 {
			return
		}
		run := string(runes[start:end])
		start = -1
		if len([]rune(run)) < 2 || noiseWords[run] || isMeasureWord(run) {
			return
		}
		out = append(out, run)
	}
	for i, r := range runes {
		if r >= '0' && r <= '9' || r == '.' || r == ',' {
			flush(i)
			continue
		}
		if start < 0 {
			start = i
		}
	}
	flush(len(runes))
	return out
}

// isMeasureWord reports whether a word is a unit or a pharmaceutical form —
// the two things a number in a product name is ever counting.
func isMeasureWord(w string) bool {
	if _, ok := unitWords[w]; ok {
		return true
	}
	_, ok := formWords[w]
	return ok
}

// noiseWords are the words Egyptian distributors append to a product name that
// say nothing about which product it is.
var noiseWords = map[string]bool{
	"سعر": true, "جديد": true, "س": true, "ج": true, "سج": true, "ض": true,
	"عرض": true, "خصم": true, "مجانا": true, "توفير": true, "بديل": true,
	"مستورد": true, "باكو": true, "جدبد": true, "الجديد": true,
	"new": true, "price": true, "offer": true, "free": true, "imported": true,
	// The rest of the marketing furniture a distributor appends. Every one of
	// these turned up in live price lists as a whole token beside the brand,
	// and every one of them is carried by enough products to outvote it.
	"الجديده": true, "الجديدة": true, "سعرجديد": true, "سعرالجديد": true,
	"تخفيض": true, "بونس": true, "هديه": true, "هدية": true, "اوفر": true,
	"خاص": true, "الاصلي": true, "الاصل": true, "اصلي": true, "جمله": true,
	"جملة": true, "نص": true, "بسعر": true, "قديم": true, "القديم": true,
	"bonus": true, "gift": true, "promo": true, "special": true, "sale": true,
	"original": true, "orig": true, "old": true, "discount": true,
}

// formKeyOf collapses the many ways a form is written onto one token, so
// "أقراص", "قرص", "tab" and "tablets" compare equal.
func formKeyOf(text string) string {
	words := strings.Fields(sheet.NormalizeName(text))
	if len(words) == 0 {
		return ""
	}
	// Letter runs rather than whole words, because Egyptian price lists glue the
	// count to the form: "20قرص", "3شريط", "24كبسوله".
	//
	// The extractor is local rather than appendLetterRuns, and that is not
	// duplication: appendLetterRuns exists to DROP measure and form words on its
	// way to the identifying vocabulary, which is precisely the opposite of what
	// is wanted here. Reusing it silently returned "" for every glued form —
	// "افوصويا 3شريط" and "اورسوفالك 20كبسولة" both came back with no form at all.
	var runs []string
	for _, w := range words {
		runs = append(runs, letterRuns(w)...)
	}
	for _, rule := range formGroups {
		for _, w := range rule.words {
			for _, run := range runs {
				if formWordMatches(run, w) {
					return rule.key
				}
			}
		}
	}
	return ""
}

// letterRuns splits a token into its maximal runs of non-digit characters,
// keeping every one of them.
func letterRuns(w string) []string {
	if !hasDigit(w) {
		return strings.Fields(w)
	}
	var out []string
	runes := []rune(w)
	start := -1
	flush := func(end int) {
		if start >= 0 && end > start {
			s := strings.TrimSpace(string(runes[start:end]))
			if s != "" {
				out = append(out, s)
			}
		}
		start = -1
	}
	for i, r := range runes {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' || r == '/' || r <= ' ' || unicode.IsSpace(r) {
			flush(i)
			continue
		}
		if start < 0 {
			start = i
		}
	}
	flush(len(runes))
	return out
}

// formWordMatches decides whether one word of a product name names a form.
//
// It used to be `strings.Contains` over the whole text, and that was a real
// defect rather than a nicety: "جل" is a form word, and "امباجليفورم",
// "اميجليسباير" and "جلوكوفاج" all contain it, so three ordinary oral products
// were filed as topical gels. That is invisible in the deterministic scorer —
// it just quietly subtracts 0.28 from a correct match — and it made the AI
// stage refuse correct answers outright.
//
// Short words must therefore match a whole letter run. Longer ones may match as
// a prefix, which is what carries "كبسولات" and "كبسوله" from "كبسول" and "tabs"
// from "tab" without listing every inflection.
func formWordMatches(run, word string) bool {
	if run == word {
		return true
	}
	if len([]rune(word)) < 4 {
		return false
	}
	return strings.HasPrefix(run, word)
}

// formGroups are ordered MOST SPECIFIC FIRST, and the order is load-bearing.
//
// The first group whose word appears wins, so a general term listed early
// swallows the specific term that actually names the product: with "liquid"
// ahead of "drops", the catalogue entry "افيميو قطرة عين معلق 10 مل" keys as a
// liquid because it says "معلق", and stops matching the pharmacy line that says
// "قطره". Eye drops suspended in a liquid are still eye drops.
var formGroups = []struct {
	key   string
	words []string
}{
	{"drops", []string{"نقط", "قطره", "قطرة", "نقطه", "drop", "drops"}},
	{"spray", []string{"بخاخ", "اسبراي", "سبراي", "رذاذ", "spray", "inhaler"}},
	{"suppository", []string{"لبوس", "تحاميل", "تحميله", "اقماع", "قمع", "supp"}},
	{"wash", []string{"غسول", "شامبو", "صابون", "مضمضه", "wash", "shampoo", "soap"}},
	{"topical", []string{"كريم", "مرهم", "جل", "جيل", "لوسيون", "دهان", "معجون",
		"cream", "oint", "ointment", "gel", "lotion", "paste"}},
	{"injectable", []string{"حقن", "حقنه", "امبول", "امبوله", "امبولات", "فيال",
		"amp", "amps", "vial", "inj", "injection"}},
	{"sachet", []string{"فوار", "كيس", "اكياس", "ساشيه", "اكياس", "sachet", "eff", "sachets"}},
	{"capsule", []string{"كبسول", "كبسوله", "cap", "caps", "capsule"}},
	{"tablet", []string{"اقراص", "قرص", "قرصين", "شريط", "كابلت", "tab", "tabs", "tablet", "caplet"}},
	{"liquid", []string{"شراب", "معلق", "محلول", "syr", "syrup", "susp", "solution", "suspension"}},
}

// doseUnits normalises the strength units onto a common base: milligrams for
// mass, millilitres for volume, and their own scale for the rest.
var doseUnits = map[string]struct {
	base  string
	scale float64
}{
	"mg": {"mg", 1}, "ملجم": {"mg", 1}, "مجم": {"mg", 1}, "مليجرام": {"mg", 1},
	// The abbreviations real supplier files actually use. "مج" and "مغ" are
	// everyday milligrams; "محم" is the ج/ح keyboard slip for "مجم" and turns
	// up in live pharmacy order files often enough to be worth reading.
	"مج": {"mg", 1}, "مغ": {"mg", 1}, "محم": {"mg", 1}, "ملجرام": {"mg", 1},
	"mcg": {"mg", 0.001}, "مكجم": {"mg", 0.001},
	"g": {"mg", 1000}, "gm": {"mg", 1000}, "جم": {"mg", 1000}, "جرام": {"mg", 1000},
	"ml": {"ml", 1}, "مل": {"ml", 1}, "مللي": {"ml", 1}, "ملي": {"ml", 1},
	"l": {"ml", 1000}, "لتر": {"ml", 1000},
	"iu": {"iu", 1}, "وحده": {"iu", 1}, "وحدة": {"iu", 1},
	"%": {"%", 1}, "spf": {"spf", 1},
}

// parseStrength reads the first dose written in a text.
func parseStrength(text string) strength {
	m := strengthPattern.FindString(FoldDoseText(sheet.NormalizeDigits(text)))
	if m == "" {
		return strength{}
	}
	m = strings.ToLower(strings.TrimSpace(m))

	// Split the number from the unit at the first non-numeric rune.
	cut := 0
	for i, r := range m {
		if (r >= '0' && r <= '9') || r == '.' || r == '/' {
			cut = i + 1
			continue
		}
		if r == ' ' && cut == i {
			cut = i + 1
			continue
		}
		break
	}
	numPart := strings.TrimSpace(m[:cut])
	unitPart := strings.TrimSpace(m[cut:])
	// "5/10mg" is a combination product; the first figure identifies it well
	// enough and the second would only add noise.
	if head, _, ok := strings.Cut(numPart, "/"); ok {
		numPart = head
	}
	value, err := strconv.ParseFloat(numPart, 64)
	if err != nil || value <= 0 {
		return strength{}
	}
	u, ok := doseUnits[unitPart]
	if !ok {
		return strength{}
	}
	return strength{value: value * u.scale, unit: u.base}
}
