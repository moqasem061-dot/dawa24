package pipeline

import (
	"regexp"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// Preparing a pharmacy's line for the matcher.
//
// The shared matcher was tuned for **vendor price lists**, where a supplier's
// line is terse and already close to the catalogue name. A pharmacy's purchase
// list is not that. It is descriptive:
//
//	بروفين مسكن ومضاد للالتهاب        ("Brufen painkiller and anti-inflammatory")
//	كونجستال أقراص للبرد والاحتقان    ("Congestal tablets for cold and congestion")
//
// The catalogue records those products as "بروفين 400مج 20ق" — the therapeutic
// description appears nowhere in its name. Handed the raw line, the scorer
// divides matched weight by the whole query's weight, so five words of prose
// against two matching ones drags a correct match below the floor. Measured on
// the live catalogue it did worse than that: "بانادول إكسترا 500 مجم أقراص"
// matched a **Sensodyne toothbrush**, because the only surviving shared token
// was "اكسترا".
//
// So the line is decomposed before it reaches the scorer: the strength, the
// form and the pack size move into the structured fields the matcher already
// compares separately, and the therapeutic prose is dropped. What is left is
// the brand and its distinguishing words — which is what the catalogue's name
// actually contains.

// strengthPattern captures a dose and its unit, in either script.
//
// The terminator is `(?:$|[^\p{L}])` and NOT `\b`. Go's `\b` is defined on
// ASCII word characters only: after "مجم" there is no \w-to-\W transition,
// because no Arabic letter is a \w — so the boundary never matched and this
// pattern found nothing whatsoever in an Arabic line. Every Arabic row was
// therefore reaching the scorer with its strength and its pack count still
// glued into the name, which is the exact failure the decomposition described
// above exists to prevent. The pattern looked correct and did nothing.
//
// The alternatives are ordered longest-first, because Go prefers the
// leftmost-first branch: "مل" ahead of "مللي" would match two letters and leave
// "لي" behind as a stray token.
var strengthPattern = regexp.MustCompile(
	`(?i)(\d+(?:[.,]\d+)?)\s*(ميكروجرام|مليجرام|ملجرام|مجم|ملجم|مكجم|محم|جرام|مللي|وحدة|وحده|ملل|ملي|مغ|مج|جم|مل|mcg|mg|gm|ml|iu|g|kg|l|%)(?:$|[^\p{L}])`)

// packPattern captures a pack count: "20 قرص", "24قرص", "17ق", "20 tabs", "10 sachets".
var packPattern = regexp.MustCompile(
	`(?i)(\d+)\s*(كبسولات|كبسولة|أقراص|اقراص|كبسول|أمبول|امبول|قرص|قروص|اكياس|أكياس|كيس|ق|قطعة|قطعه|capsules|capsule|tablets|tablet|tabs|caps|tab|cap|sachets|sachet|vials|vial|amps|amp|pcs|pc)(?:$|[^\p{L}])`)

// formWords map a dosage form written in a line onto a canonical term.
var formWords = map[string]string{
	"أقراص": "أقراص", "اقراص": "أقراص", "قرص": "أقراص", "قروص": "أقراص",
	"كبسول": "كبسولات", "كبسولة": "كبسولات", "كبسولات": "كبسولات",
	"شراب": "شراب", "شرب": "شراب",
	"حقن": "حقن", "امبول": "حقن", "أمبول": "حقن", "حقنة": "حقن", "فيال": "حقن",
	"بخاخ": "بخاخ", "سبراى": "بخاخ", "سبراي": "بخاخ",
	"كريم": "كريم", "مرهم": "مرهم", "جل": "جل",
	"نقط": "نقط", "قطرة": "نقط", "قطره": "نقط",
	"لبوس": "لبوس", "تحاميل": "لبوس",
	"أكياس": "أكياس", "اكياس": "أكياس", "كيس": "أكياس",
	"بلسم": "بلسم", "شامبو": "شامبو", "سيروم": "سيروم", "سيرم": "سيروم",
	"صابونة": "صابون", "صابون": "صابون", "لوشن": "لوشن", "زيت": "زيت",
	"ماسك": "ماسك", "ستيك": "ستيك", "بلاستر": "بلاستر", "غسول": "غسول",
	"فوم": "فوم", "رول": "رول", "بودرة": "بودرة", "باودر": "بودرة",
	"tab": "أقراص", "tabs": "أقراص", "tablet": "أقراص", "tablets": "أقراص",
	"capsule": "كبسولات", "caps": "كبسولات", "capsules": "كبسولات",
	"syrup": "شراب", "syr": "شراب", "suspension": "شراب", "susp": "شراب",
	"cream": "كريم", "crm": "كريم", "gel": "جل", "ointment": "مرهم", "oint": "مرهم",
	"drops": "نقط", "drop": "نقط", "spray": "بخاخ", "lotion": "لوشن", "shampoo": "شامبو",
	"soap": "صابون", "serum": "سيروم", "oil": "زيت", "mask": "ماسك", "balm": "بلسم",
	"vial": "حقن", "amp": "حقن", "ampoule": "حقن", "ampoules": "حقن", "injection": "حقن", "inj": "حقن",
	"supp": "لبوس", "suppository": "لبوس", "sachet": "أكياس", "sachets": "أكياس",
	"stick": "ستيك", "plaster": "بلاستر",
}

// therapeuticFiller is prose that describes what a medicine or cosmetic does.
var therapeutic = map[string]bool{
	"مسكن": true, "مسكنة": true, "مضاد": true, "مضادة": true, "للالتهاب": true,
	"التهاب": true, "الالتهاب": true, "للبرد": true, "برد": true, "والاحتقان": true,
	"احتقان": true, "الاحتقان": true, "مطهر": true, "معوي": true, "معوى": true,
	"للأنف": true, "للانف": true, "للكبار": true, "للأطفال": true, "للاطفال": true,
	"سريع": true, "سريعة": true, "قوي": true, "قوى": true, "قوية": true,
	"خافض": true, "للحرارة": true, "حرارة": true, "فيتامين": true, "مكمل": true,
	"غذائي": true, "غذائى": true, "تجريبي": true, "تجريبى": true, "جديد": true,
	"توفير": true, "عرض": true, "خصم": true,
	"للشعر": true, "لتساقط": true, "تساقط": true, "الشعر": true,
	"للبشرة": true, "للبشره": true, "بشرة": true, "بشره": true,
	"تفتيح": true, "ترطيب": true, "حريمي": true, "حريمى": true, "رجالي": true, "رجالى": true,
	"and": true, "for": true, "with": true, "the": true,
	"hair": true, "skin": true, "loss": true, "care": true, "new": true, "free": true,
}

// BuildRow turns an imported line into the row the matcher scores.
func BuildRow(l *smartorder.Line) *productmatch.Row {
	raw := strings.TrimSpace(l.RawName)
	raw = productmatch.StripTradeAnnotations(raw)

	row := &productmatch.Row{
		Number:  l.RowNumber,
		SKU:     l.RawSKU,
		Barcode: l.RawBarcode,
	}

	rest := raw

	// Strength: "500 مجم" becomes a structured comparison rather than two more
	// words competing with the brand.
	if m := strengthPattern.FindStringSubmatch(rest); m != nil {
		row.Concentration = strings.TrimSpace(m[1] + " " + m[2])
		rest = strings.Replace(rest, m[0], " ", 1)
	}

	// The distributor, which Egyptian price lists write after a slash:
	// "ابيفيناك حقن /ايبيكو", "ال كارنتين شراب/ميباكو". Left in the name it is
	// not merely dead weight — it is actively wrong, because two unrelated
	// medicines from the same house share it and nothing else. On the live
	// catalogue that is exactly how "ابيفيناك حقن /ايبيكو" came to be matched to
	// "سيفوتاكس 500مجم فيال ايبيكو": one shared word, two different drugs.
	//
	// Moved into Manufacturer it becomes what it is — weak corroboration the
	// scorer may add to a match but never decide one on.
	if name, maker, ok := splitDistributor(rest); ok {
		rest = name
		cleanMaker := strings.ToLower(strings.TrimSpace(maker))
		if !strings.HasPrefix(cleanMaker, "سعر") && !therapeutic[cleanMaker] {
			row.Manufacturer = maker
		}
	}

	// Pack size, same reasoning.
	if m := packPattern.FindStringSubmatch(rest); m != nil {
		row.PackSize = atoiSafe(m[1])
		if form, ok := formWords[strings.ToLower(m[2])]; ok {
			row.DosageForm = form
		}
		rest = strings.Replace(rest, m[0], " ", 1)
	}

	// Whatever survives: keep the identifying words, drop the description.
	var kept []string
	for _, tok := range strings.Fields(rest) {
		clean := strings.Trim(tok, "()[]{}،,.؛;:-_/\\")
		if clean == "" {
			continue
		}
		lower := strings.ToLower(clean)

		if form, ok := formWords[lower]; ok {
			if row.DosageForm == "" {
				row.DosageForm = form
			}
			continue
		}
		if therapeutic[lower] || lower == "ص" || lower == "ك" {
			continue
		}
		kept = append(kept, clean)
	}

	row.Name = strings.Join(kept, " ")
	if row.Name == "" {
		// Everything was filler. Better to match on the original than on
		// nothing — a poor candidate the buyer can correct beats an unmatched
		// line with no explanation.
		row.Name = raw
	}
	return row
}

// splitDistributor separates a trailing "/distributor" from a product name.
//
// Only a trailing segment counts, and only one that does not begin with a digit.
// Slashes are load-bearing elsewhere in these lines — "افيفافاسك 5/160مجم" is a
// combination strength and "250مجم/5مل" is a concentration — and mistaking
// either for a company name would throw away the figure that identifies the
// product.
func splitDistributor(text string) (name, maker string, ok bool) {
	cut := strings.LastIndex(text, "/")
	if cut < 0 {
		return "", "", false
	}
	head := strings.TrimSpace(text[:cut])
	tail := strings.TrimSpace(text[cut+1:])
	if head == "" || tail == "" {
		return "", "", false
	}
	if r := []rune(tail)[0]; r >= '0' && r <= '9' {
		return "", "", false
	}
	return head, tail, true
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
