package productmatch

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

// Identity rules for how Egyptian catalogues and price lists name products.
//
// Each rule here was found in the egypt evaluation suite as a wrong product
// accepted, and each states a distinction the rest of this package could not
// see. None of them refuses a candidate on its own unless the distinction is
// a product difference whatever the names look like; the rest are penalties
// that rank the right candidate first and send an only-candidate to review.

// ---------------------------------------------------------------------------
// Numbers that are part of the brand: مينا كيو-90, الترافيست-370, فيتالوكس 50+,
// B12, Q10, D3.
//
// They read as counts or stray figures, and a figure the catalogue does not
// repeat is silence — so "مينا كيو-90" settled on "مينا كيو-45", the same
// line at half the dose. They are identity marks, compared like the line
// letters: two names stating different ones are different products.

var (
	hyphenNumber = regexp.MustCompile(`\p{L}[-‐–](\d{1,4})(\+?)`)
	numberLed    = regexp.MustCompile(`(?:^|[^\d.])(\d{1,3})[-‐–](\p{L}{3,})`)
	plusNumber   = regexp.MustCompile(`(?:^|[\s(])(\d{1,3})\+`)
	latinGlued   = regexp.MustCompile(`(?:^|[^\p{L}\d])([a-z]{1,2})(\d{1,3})(?:$|[^\p{L}\d])`)
)

// brandNumberMarks extracts the numbers a name uses as part of its brand.
func brandNumberMarks(text string) []string {
	t := strings.ToLower(sheet.NormalizeDigits(text))
	var out []string
	for _, m := range hyphenNumber.FindAllStringSubmatch(t, -1) {
		out = append(out, "#"+m[1]+m[2])
	}
	for _, m := range numberLed.FindAllStringSubmatch(t, -1) {
		// "3-month", "5-fluorouracil": a figure leading a word by a hyphen
		// names the line. Keyed with the word, so 5-fluorouracil on both sides
		// agrees and 3-month against 1-month does not.
		out = append(out, "#"+m[1]+"-"+m[2])
	}
	for _, m := range plusNumber.FindAllStringSubmatch(t, -1) {
		out = append(out, "#"+m[1]+"+")
	}
	for _, m := range latinGlued.FindAllStringSubmatch(t, -1) {
		if _, unit := unitWords[m[1]]; unit || isDoseUnitWord(m[1]) {
			continue
		}
		out = append(out, "#"+m[1]+m[2])
	}
	return out
}

// ---------------------------------------------------------------------------
// The injection container: ampoule, vial, pre-filled syringe, pen.
//
// They share one form key, and that is right for retrieval — a row reading
// حقن finds all of them — but an ampoule and a 50 ml vial of lidocaine 2% are
// two products, and so are a vial and a pre-filled syringe of the same
// molecule. A name that mentions two containers (a vial with a solvent
// ampoule) names none, so it is never contradicted.

var injContainers = map[string]string{
	"امبول": "ampoule", "امبوله": "ampoule", "امبولة": "ampoule", "امبولات": "ampoule",
	"amp": "ampoule", "amps": "ampoule", "ampoule": "ampoule", "ampule": "ampoule",
	"فيال": "vial", "vial": "vial", "vials": "vial",
	"سرنجة": "syringe", "سرنجه": "syringe", "سرنجات": "syringe", "syringe": "syringe",
	"prefilled": "syringe", "pre-filled": "syringe", "معباة": "syringe", "معبأة": "syringe",
	"قلم": "pen", "pen": "pen", "pens": "pen", "بنفل": "pen", "penfill": "pen", "penfills": "pen",
}

// injContainerOf names the one container a name states, or "".
func injContainerOf(text string) string {
	found := ""
	for _, w := range strings.Fields(sheet.NormalizeName(text)) {
		for _, run := range letterRuns(w) {
			c, ok := injContainers[run]
			if !ok {
				continue
			}
			if found != "" && found != c {
				return "" // a kit: vial plus solvent ampoule
			}
			found = c
		}
	}
	return found
}

// ---------------------------------------------------------------------------
// Volume is content, not strength.
//
// "الترافيست 300مجم 50مل" against a catalogue record whose Arabic name says
// 370مجم and whose English name says only "50 ml": the strength comparison
// asks each spelling and takes the one that agrees, and the English spelling
// agreed — on the volume. A shared bottle size cannot overrule a contradicted
// strength, so volumes are compared only where neither side states anything
// but volumes.

func isVolumeUnit(u string) bool { return u == "ml" || u == "l" }

func hasNonVolume(m map[string][]strength) bool {
	for u := range m {
		if !isVolumeUnit(u) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// The identifying words beyond the brand.
//
// The lead brand is not always the product. A house brand or a molecule leads
// many names, and what separates them is the next word:
//
//	سانسو بريجنا بلس          against   سانسو بايفلكس بلس
//	بانثينول بانكس كريم       against   بانثينول 2% كريم
//	صوديوم بيكربونات 5%       against   صوديوم كلوريد 0.9% وجلوكوز 5%
//
// Two signals, both penalties rather than vetoes, because catalogue names are
// verbose and pharmacy lines terse, and a single unmatched word is often a
// description:
//
//   - wordUnverified: the row carries a distinctive word the catalogue has
//     never seen and the candidate does not carry in any spelling. Something
//     the engine cannot recognise names this row; a person should look.
//   - wordMismatch: each side carries a distinctive word the other lacks.
//     That is two names saying two different things, not one name saying less.

const (
	// rareLineWordDF is how few products may carry a word for it to read as a
	// line name rather than a description.
	rareLineWordDF     = 12
	massWordUnverified = 0.55
	massWordMismatch   = 0.55
	massMakerMismatch  = 0.55
	// massContainer only breaks ties. Measured on real supplier files, the
	// catalogue writes امبول where the supplier writes vial for the same
	// product often enough ("mabthera 100mg/10 ml 2 vials" is catalogued as
	// "2امبول") that a penalty refusing the only candidate cost far more
	// correct matches than it prevented wrong ones.
	massContainer = 0.15
)

// wordConflict compares the identifying vocabulary beyond the brand.
func (idx *Index) wordConflict(q *query, p *MasterProduct) (conflict, bool) {
	rowExtra, rowUnknown, knownExtra := false, false, false
	for _, t := range q.nameTokens {
		if !idx.identifyingWord(t) || t == q.brand {
			continue
		}
		if productCarries(p, t) != brandLinkNone {
			continue
		}
		if idx.df[t] == 0 {
			// An unrecognised word is as often English written in Arabic
			// letters ("انتميت سكير" for intimate skin) as a different
			// product, and is not held against the candidate.
			rowUnknown = true
			continue
		}
		rowExtra = true
		if idx.df[t] <= rareLineWordDF {
			// A word the catalogue uses for only a handful of products is a
			// line name — بينك, بانكس, زنك in a drops line — not a description.
			knownExtra = true
		}
	}
	if !rowExtra {
		return conflict{}, false
	}
	// A known line word on the row's side alone is not held against the
	// candidate either: measured, that refused 337 correct matches for 2 wrong
	// ones — pharmacy lines add words the catalogue spells elsewhere.
	_ = knownExtra
	// A row word the catalogue has never seen is NOT held against the
	// candidate on its own: measured, that refused 341 correct matches for 6
	// wrong ones, because Latin supplier lines carry English words an
	// Arabic-only record cannot echo. It only counts when the candidate also
	// says something the row does not (wordMismatch, below).
	_ = rowUnknown
	for _, side := range [][]string{p.coreAR, p.coreEN} {
		if len(side) == 0 || !sameScriptAsRow(q, side) {
			continue
		}
		for _, t := range side {
			// A company written into the catalogue name is part of what the
			// product is called (فوليك اسيد النيل), so it counts here.
			named := idx.identifyingWord(t) || (idx.isCompanyWord(t) && len([]rune(t)) >= minBrandRunes && !idx.commonWord(t))
			if !named {
				continue
			}
			if !q.carries(t) {
				return conflict{"word_mismatch", massWordMismatch}, true
			}
		}
	}
	return conflict{}, false
}

// identifyingWord is a word that can name a product: long enough, not a
// company, not a line-extension word, not a unit, form or figure, and not
// carried by a large part of the catalogue.
func (idx *Index) identifyingWord(t string) bool {
	if len([]rune(t)) < minBrandRunes || hasDigit(t) {
		return false
	}
	if _, isMod := variantModifiers[t]; isMod {
		return false
	}
	if descriptorWords[t] {
		return false
	}
	return !idx.isCompanyWord(t) && !idx.commonWord(t)
}

// descriptorWords are what pharmacists and catalogues add to a name to say
// what it is for or how it is sold. They never identify a product.
var descriptorWords = map[string]bool{
	"مستورد": true, "محلي": true, "اصلي": true, "اصلى": true, "جديد": true, "جديده": true,
	"للاطفال": true, "اطفال": true, "للكبار": true, "كبار": true, "للرضع": true, "رضع": true,
	"للشعر": true, "للوجه": true, "للجسم": true, "للبشره": true, "للبشرة": true,
	"مساج": true, "تفتيح": true, "مرطب": true, "منظف": true, "ملطف": true, "طبي": true,
	"وريدي": true, "عضلي": true, "للحقن": true, "للتسريب": true, "للاستنشاق": true,
	"محلول": true, "مائي": true, "زجاجه": true, "زجاجة": true, "علبه": true, "عبوه": true, "عبوة": true,
	"imported": true, "medical": true, "kids": true, "baby": true, "adult": true,
}

// carries reports whether the row carries a catalogue word in any spelling.
func (q *query) carries(word string) bool {
	probe := &MasterProduct{coreAR: q.nameTokens}
	return productCarries(probe, word) != brandLinkNone
}

func sameScriptAsRow(q *query, side []string) bool {
	if len(q.nameTokens) == 0 || len(side) == 0 {
		return false
	}
	return isLatinWord(q.nameTokens[0]) == isLatinWord(side[0])
}

// ---------------------------------------------------------------------------
// Generics are identified by their company.
//
// "فوليك اسيد ممفيس" and "فوليك اسيد النيل" are the same molecule from two
// companies, and two products with two prices. Company words are demoted
// everywhere else in this package because price lists append the
// distributor — "/ايبيكو" — but a company written INTO the name, before any
// slash, is part of what the product is called.

func (idx *Index) nameCompanies(text string) []string {
	if i := strings.IndexAny(text, "/\\"); i >= 0 {
		text = text[:i]
	}
	var out []string
	for _, t := range coreTokens(text) {
		if len([]rune(t)) >= minBrandRunes && idx.isCompanyWord(t) {
			out = append(out, t)
		}
	}
	return out
}

func (idx *Index) makerConflict(q *query, p *MasterProduct) (conflict, bool) {
	if len(q.nameMakers) == 0 {
		return conflict{}, false
	}
	// Companies are compared within one script: an English supplier line
	// naming the company in Latin against the Arabic half of a record is not a
	// disagreement, it is two alphabets.
	latin := isLatinWord(q.nameMakers[0])
	var theirs []string
	for _, b := range p.nameMakers {
		if isLatinWord(b) == latin {
			theirs = append(theirs, b)
		}
	}
	if len(theirs) == 0 {
		return conflict{}, false
	}
	for _, a := range q.nameMakers {
		for _, b := range theirs {
			if a == b || wordsCorrespond(a, b) || variantKeyOf(a) == variantKeyOf(b) {
				return conflict{}, false
			}
		}
	}
	return conflict{"maker", massMakerMismatch}, true
}

// statesMass reports a dose set that measures something other than volume.
func statesMass(set []strength) bool {
	for _, s := range set {
		if !isVolumeUnit(s.unit) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// The bottle size is a different item.
//
// An Egyptian catalogue lists each size of a syrup, a suspension or a vial as
// its own item with its own price: سبراكس 100مجم/5مل معلق 30 مل and 60 مل are
// two products. Volumes were compared only as strengths, and a volume is not a
// strength (see compareStrengths), so the sizes were never compared at all and
// the 60 ml order settled on the 30 ml bottle.
//
// totalVolumes is the TOTAL content a name states: a millilitre or litre
// figure that is not the denominator of a concentration ("/5مل").

var totalVolumeRe = regexp.MustCompile(`(^|[^/\d.])\s*(\d+(?:\.\d+)?)\s*(مللي|ملي|مل|ml|لتر|l)(?:$|[^\p{L}])`)

func totalVolumes(text string) []float64 {
	t := strings.ToLower(sheet.NormalizeDigits(text))
	var out []float64
	for _, m := range totalVolumeRe.FindAllStringSubmatch(t, -1) {
		v, err := strconv.ParseFloat(m[2], 64)
		if err != nil || v <= 0 {
			continue
		}
		if m[3] == "لتر" || m[3] == "l" {
			v *= 1000
		}
		out = append(out, v)
	}
	return out
}

// volumesDisagree reports two names that both state a total volume and share
// none.
func volumesDisagree(a, b []float64) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return false
			}
		}
	}
	return true
}

const massVolume = 0.70

// ---------------------------------------------------------------------------
// A misspelt line extension is still a line extension.
//
// "ADOL EXRA 24 CAPLETS" asks for Adol Extra. Read literally it names no
// modifier, so the plain Adol — which contradicts nothing the row states —
// ranked first and was applied. Only words the catalogue has never seen are
// considered: a brand one letter from a modifier ("fortex", "forte") is a word
// the catalogue knows, and must not be read as one.

const minModifierTypoRunes = 4

func (idx *Index) typoModifiers(nameTokens []string, mods map[string]struct{}) {
	for _, w := range nameTokens {
		if len([]rune(w)) < minModifierTypoRunes || idx.df[w] > 0 || idx.knownVariant(w) {
			continue
		}
		if _, exact := variantModifiers[w]; exact {
			continue
		}
		for k, key := range variantModifiers {
			if len([]rune(k)) >= minModifierTypoRunes && editSimilarity(w, k) >= minEditSimilarity {
				mods[key] = struct{}{}
				break
			}
		}
	}
}
