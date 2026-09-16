package productmatch

import "github.com/muhiya/dawa24-store/internal/shared/sheet"

// Vocabularies.
//
// Some columns are identified by what their values *are* rather than by their
// shape. A column holding "علبة", "شريط" and "زجاجة" is a unit column whatever
// its header says; a column holding forty distinct manufacturer names that all
// already exist as brands in the catalogue is a manufacturer column.
//
// The built-in lists cover the Egyptian pharmaceutical vocabulary. The
// catalogue-derived ones are handed in by the ingest service, which is what
// makes the mapping schema-aware rather than merely rule-based.

// Vocabulary is the set of known values the resolver can recognise.
type Vocabulary struct {
	// Brands and Categories come from the live catalogue.
	Brands     map[string]struct{}
	Categories map[string]struct{}
	// Warehouses and Branches come from the vendor's own organisation, so a
	// column of their warehouse names is recognised as such.
	Warehouses map[string]struct{}
	Branches   map[string]struct{}
}

// NewVocabulary builds a vocabulary from raw catalogue values, folding each
// through the same normaliser the cells go through.
func NewVocabulary(brands, categories, warehouses, branches []string) *Vocabulary {
	return &Vocabulary{
		Brands:     keySet(brands),
		Categories: keySet(categories),
		Warehouses: keySet(warehouses),
		Branches:   keySet(branches),
	}
}

func keySet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		if key := sheet.NormalizeKey(v); key != "" {
			out[key] = struct{}{}
		}
	}
	return out
}

// hit is the share of a column's distinct sample present in a set.
func (v *Vocabulary) hit(sample []string, set map[string]struct{}) float64 {
	if v == nil || len(set) == 0 || len(sample) == 0 {
		return 0
	}
	n := 0
	for _, s := range sample {
		if _, ok := set[sheet.NormalizeKey(s)]; ok {
			n++
		}
	}
	return float64(n) / float64(len(sample))
}

// unitWords are the sales units an Egyptian supplier writes.
var unitWords = keySet([]string{
	"علبة", "علبه", "شريط", "زجاجة", "زجاجه", "كيس", "أمبول", "امبول", "عبوة", "عبوه",
	"قطعة", "قطعه", "برطمان", "أنبوبة", "انبوبه", "تيوب", "باكو", "دستة", "دسته",
	"كرتونة", "كرتونه", "فيال", "لفة", "طبة", "وحدة", "وحده", "جرام", "مللي", "لتر",
	// The pack nouns Egyptian price lists actually type, and the reason a
	// whole screen of wrong matches was possible: "باكت" is carried by
	// thousands of lines, and two products whose only shared word was that
	// scored as relatives. A word that names the packaging is not a word that
	// names the medicine.
	"باكت", "باكيت", "بكت", "باكتة", "باكته", "بوكس", "كرتون", "درزن", "درزينة",
	"ظرف", "اظرف", "أظرف", "طبق", "اطباق", "أطباق", "شرائط", "علب", "قطع",
	"حبة", "حبه", "حبات", "كجم", "جم", "سم", "cc", "kg", "لفه", "عبوات",
	// Dose units left loose in a name are not identifying words: "مجم" is
	// carried by half the catalogue, so counting it as vocabulary dilutes the
	// brand it sits next to. The strength itself is parsed separately and
	// compared as a number, which is the only comparison that means anything.
	"مجم", "مج", "مغ", "محم", "ملجم", "ملجرام", "مليجرام", "مكجم", "ملي", "مل",
	"mg", "mcg", "gm", "ml", "iu",
	"box", "strip", "bottle", "pack", "packet", "piece", "pcs", "vial", "tube",
	"sachet", "carton", "unit", "ea", "each", "amp", "ampoule", "jar", "can", "tin",
})

// formWords are the pharmaceutical forms.
var formWords = keySet([]string{
	"أقراص", "اقراص", "قرص", "كبسول", "كبسولات", "كبسولة", "شراب", "معلق", "نقط",
	"قطرة", "قطره", "حقن", "حقنة", "أمبول", "امبول", "فيال", "كريم", "مرهم", "جل",
	"جيل", "لبوس", "تحاميل", "تحميلة", "بخاخ", "اسبراي", "سبراي", "فوار", "ساشيه",
	"أكياس", "اكياس", "غسول", "شامبو", "صابون", "لوسيون", "زيت", "سيروم", "مسحوق",
	"بودرة", "بودره", "رذاذ", "دهان", "محلول", "مضمضة", "معجون", "مناديل", "صبغة",
	"tablet", "tablets", "tab", "tabs", "capsule", "capsules", "cap", "caps",
	"syrup", "syr", "suspension", "susp", "drops", "drop", "injection", "inj",
	"vial", "ampoule", "cream", "ointment", "oint", "gel", "suppository", "supp",
	"spray", "sachet", "lotion", "powder", "solution", "sol", "shampoo", "soap",
	"serum", "oil", "wipes", "mouthwash", "toothpaste",
})

// statusWords are the lifecycle words a status column holds.
var statusWords = keySet([]string{
	"مفعل", "معطل", "نشط", "غير نشط", "متاح", "غير متاح", "موقوف", "ملغي",
	"معلق", "قيد المراجعة", "مرفوض", "نعم", "لا",
	"active", "inactive", "enabled", "disabled", "available", "unavailable",
	"pending", "rejected", "yes", "no", "true", "false", "on", "off",
})

// yesWords and noWords resolve a boolean cell.
var (
	yesWords = keySet([]string{"نعم", "مفعل", "نشط", "متاح", "صح", "موجود", "yes", "y", "true", "1", "active", "enabled", "available", "on"})
	noWords  = keySet([]string{"لا", "معطل", "غير نشط", "غير متاح", "موقوف", "خطأ", "no", "n", "false", "0", "inactive", "disabled", "unavailable", "off"})
)

// inSet reports whether a raw value's folded key is in a set.
func inSet(set map[string]struct{}, raw string) bool {
	if raw == "" {
		return false
	}
	_, ok := set[sheet.NormalizeKey(raw)]
	return ok
}

// vocabRate is the share of a sample whose values are short enough to be a
// single vocabulary term and present in the set.
//
// The length guard is what stops a product-name column scoring as a dosage-form
// column: "بانادول إكسترا 24 قرص" contains "قرص", but a form column holds
// "أقراص" on its own and nothing else.
func vocabRate(sample []string, set map[string]struct{}) float64 {
	if len(sample) == 0 {
		return 0
	}
	n := 0
	for _, v := range sample {
		if len([]rune(v)) > 22 {
			continue
		}
		if inSet(set, v) {
			n++
		}
	}
	return float64(n) / float64(len(sample))
}
