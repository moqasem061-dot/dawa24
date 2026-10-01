package matcheval

// The Egyptian shorthand suite.
//
// The tuning split this package inherited is generated from the catalogue with
// almost no noise and no negatives, so it could not see either of the two
// failures that matter on real Egyptian price lists: a correct product refused
// because the supplier wrote it the way pharmacists write ("كونكور 5مج 30ق"),
// and a WRONG product accepted because it shares the strength, the form and the
// pack with a brand the catalogue does not carry.
//
// EgyptianCases builds both from a catalogue snapshot, deterministically:
//
//   - positives: catalogue products rewritten with realistic shorthand —
//     form abbreviations, unit spellings, letter variants, vowel drops,
//     keyboard-neighbour typos, glued figures and commercial noise. Nothing
//     that identifies the product is removed, so a good engine accepts them.
//   - negatives: products withheld from the index (EgyptWithheld) and then
//     asked for. The right answer is to abstain; accepting anything is a wrong
//     match, most often a different brand of the same strength and form or a
//     sibling of a different strength.
//
// Products whose Arabic name is shared by another catalogue record are
// labelled inherently ambiguous rather than resolvable.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

// EgyptSnapshot is the CatalogSnapshot value the evaluator maps to the index
// built without the withheld products.
const EgyptSnapshot = "egypt-withheld"

// EgyptWithheld reports whether a product is withheld from the egypt index.
// One product in twenty, chosen by id so the split is stable across runs.
func EgyptWithheld(id int64) bool { return id%20 == 7 }

// egyptPositiveStride samples the positives: one product in four.
const egyptPositiveStride = 4

// EgyptianCases generates the suite from a catalogue snapshot.
func EgyptianCases(products []productmatch.MasterProduct) []Case {
	byName := make(map[string][]int64, len(products))
	byShape := make(map[string][]int64, len(products))
	byNameEN := make(map[string][]int64, len(products))
	byShapeEN := make(map[string][]int64, len(products))
	for i := range products {
		if k := sheet.NormalizeName(products[i].NameEN); k != "" {
			byNameEN[k] = append(byNameEN[k], products[i].ID)
		}
		if k := nameShape(products[i].NameEN); k != "" {
			byShapeEN[k] = append(byShapeEN[k], products[i].ID)
		}
		if k := sheet.NormalizeName(products[i].NameAR); k != "" {
			byName[k] = append(byName[k], products[i].ID)
		}
		if k := nameShape(products[i].NameAR); k != "" {
			byShape[k] = append(byShape[k], products[i].ID)
		}
	}

	var out []Case
	for i := range products {
		p := &products[i]
		name := strings.TrimSpace(p.NameAR)
		if name == "" || len([]rune(name)) < 4 {
			continue
		}
		withheld := EgyptWithheld(p.ID)
		if !withheld && i%egyptPositiveStride != 0 {
			continue
		}
		row := EgyptianShorthand(name, int(p.ID))
		if row == "" {
			continue
		}
		same := byName[sheet.NormalizeName(name)]

		c := Case{
			ID:               fmt.Sprintf("eg-%d", p.ID),
			Split:            "egypt",
			TemplateFamily:   "egypt-shorthand",
			ExpressionFamily: fmt.Sprintf("target-%d", p.ID),
			Workflow:         []string{"vendor", "savings", "smartorder", "compare"},
			Language:         "ar",
			Row: CaseRow{
				Cells:   map[string]string{"name": row},
				Mapping: map[string]int{"name": 0},
			},
			RequiredGranularity: "product",
			CatalogSnapshot:     EgyptSnapshot,
		}
		switch {
		case withheld:
			// Anything still in the index sharing this exact name is a true
			// duplicate record, and accepting it is right.
			var remaining []int64
			for _, id := range same {
				if id != p.ID && !EgyptWithheld(id) {
					remaining = append(remaining, id)
				}
			}
			for _, id := range byShape[nameShape(name)] {
				if id != p.ID && !EgyptWithheld(id) {
					remaining = append(remaining, id)
				}
			}
			if len(remaining) > 0 {
				// A duplicate record — the same words and figures spelled
				// differently — is still in the index, so accepting it is not
				// a wrong match and the case has no clean label.
				continue
			}
			c.ID = "egx-" + c.ID[3:]
			c.Label = Label{Resolvable: false, Expect: "abstain",
				Difficulty: []string{"absent_target"}, Granularity: "product"}
		case len(same) > 1:
			c.Label = Label{Resolvable: false, Targets: same, Expect: "ambiguous",
				Difficulty: []string{"duplicate_record"}, Granularity: "product"}
		default:
			c.Label = Label{Resolvable: true, Targets: []int64{p.ID}, Expect: "accept",
				Difficulty: []string{"egyptian_shorthand"}, Granularity: "product"}
		}
		out = append(out, c)

		// The same product as an English supplier line writes it, labelled by
		// the English name: two records sharing an Arabic name are often
		// told apart by their English one, and the reverse.
		if en := strings.TrimSpace(p.NameEN); en != "" {
			lc := c
			lc.ID = "en-" + c.ID
			sameEN := byNameEN[sheet.NormalizeName(en)]
			switch {
			case withheld:
				dup := false
				for _, id := range append(append([]int64{}, sameEN...), byShapeEN[nameShape(en)]...) {
					if id != p.ID && !EgyptWithheld(id) {
						dup = true
					}
				}
				if dup {
					continue
				}
				lc.Label = Label{Resolvable: false, Expect: "abstain",
					Difficulty: []string{"absent_target"}, Granularity: "product"}
			case len(sameEN) > 1:
				lc.Label = Label{Resolvable: false, Targets: sameEN, Expect: "ambiguous",
					Difficulty: []string{"duplicate_record"}, Granularity: "product"}
			default:
				lc.Label = Label{Resolvable: true, Targets: []int64{p.ID}, Expect: "accept",
					Difficulty: []string{"egyptian_shorthand"}, Granularity: "product"}
			}
			lc.Language = "en"
			lc.TemplateFamily = "egypt-latin"
			lc.Row = CaseRow{
				Cells:   map[string]string{"name": LatinShorthand(en, int(p.ID))},
				Mapping: map[string]int{"name": 0},
			}
			out = append(out, lc)
		}
	}
	return out
}

// EgyptianShorthand rewrites a catalogue name the way an Egyptian price list or
// pharmacy order writes it. seed selects the rewrites, so a product is always
// rewritten the same way.
func EgyptianShorthand(name string, seed int) string {
	if seed < 0 {
		seed = -seed
	}
	words := strings.Fields(name)
	if len(words) == 0 {
		return ""
	}

	// 1. Form words: abbreviated about half the time, as order lists do.
	if seed%2 == 0 {
		for i, w := range words {
			if r, ok := formShorthand[w]; ok {
				words[i] = r[seed%len(r)]
			}
		}
	}
	// 2. Units: spelled however the typist spells them.
	for i, w := range words {
		if r, ok := unitShorthand[w]; ok {
			words[i] = r[(seed/3)%len(r)]
		}
	}
	// 3. The brand word: one spelling change, the commonest source of misses.
	if b := brandIndex(words); b >= 0 {
		words[b] = respellBrand(words[b], seed)
	}
	s := strings.Join(words, " ")

	// 4. Figures glued to the word after them: "500 مجم" → "500مجم".
	if (seed/5)%2 == 0 {
		s = glueFigures(s)
	}
	// 5. Letters every Egyptian keyboard writes either way.
	switch (seed / 7) % 4 {
	case 0:
		s = strings.ReplaceAll(s, "ة", "ه")
	case 1:
		s = strings.ReplaceAll(s, "ي ", "ى ")
	case 2:
		s = strings.ReplaceAll(s, "ا", "أ")
	}
	// 6. Commercial noise and the distributor after a slash.
	if n := commercialNoise[(seed/11)%len(commercialNoise)]; n != "" {
		s += " " + n
	}
	return s
}

var formShorthand = map[string][]string{
	"قرص":     {"ق", "قرص", "اقراص"},
	"اقراص":   {"ق", "اقراص", "قرص"},
	"أقراص":   {"ق", "اقراص"},
	"كبسولة":  {"ك", "كبسوله", "كبسول"},
	"كبسولات": {"ك", "كبسول"},
	"كبسول":   {"ك", "كبسوله"},
	"شراب":    {"ش", "شرب", "شراب"},
	"امبول":   {"امب", "امبوله"},
	"امبولات": {"امب", "امبول"},
	"امبولة":  {"امب", "امبول"},
	"فيال":    {"ف", "فيال"},
	"اكياس":   {"كيس", "اكياس"},
	"نقط":     {"نقط", "نقطه"},
	"قطرة":    {"قطره", "نقط"},
}

var unitShorthand = map[string][]string{
	"مجم":       {"مج", "ملجم", "مجم", "mg"},
	"ملجم":      {"مج", "مجم", "ملجم"},
	"مل":        {"ملي", "مل", "مللي"},
	"جم":        {"جرام", "جم", "gm"},
	"جرام":      {"جم", "جرام"},
	"ميكروجرام": {"مكجم", "ميكروجرام"},
}

var commercialNoise = []string{
	"", "سعر جديد", "س.ج", "عرض", "/ايبيكو", "/ميباكو", "**", "", "سعر قديم", "",
}

// brandIndex is the first word that is neither a figure nor a form or unit
// word, which is where an Egyptian product name keeps its brand.
func brandIndex(words []string) int {
	for i, w := range words {
		if len([]rune(w)) < 4 || hasDigitRune(w) {
			continue
		}
		if _, ok := formShorthand[w]; ok {
			continue
		}
		if _, ok := unitShorthand[w]; ok {
			continue
		}
		return i
	}
	return -1
}

// Keyboard neighbours and sound-alikes Egyptian typists actually swap.
var letterSwaps = [][2]string{
	{"ج", "ح"}, {"س", "ش"}, {"ت", "ث"}, {"ص", "س"}, {"ض", "د"}, {"ظ", "ز"},
	{"ذ", "ز"}, {"ق", "ك"}, {"ط", "ت"},
}

// respellBrand applies one realistic change to a brand word: a long vowel
// dropped or doubled (the transliteration variance), or one sound-alike letter.
func respellBrand(w string, seed int) string {
	r := []rune(w)
	if len(r) < 5 {
		return w
	}
	switch (seed / 13) % 4 {
	case 0: // drop an inner long vowel: بانادول → بنادول
		for i := 1; i < len(r)-1; i++ {
			if r[i] == 'ا' || r[i] == 'و' || r[i] == 'ي' {
				return string(append(append([]rune{}, r[:i]...), r[i+1:]...))
			}
		}
	case 1: // insert a long vowel after the first letter: ابليفاي → ابيليفاي
		return string(r[:1]) + "ي" + string(r[1:])
	case 2: // one sound-alike letter
		for _, sw := range letterSwaps {
			if strings.Contains(w, sw[0]) {
				return strings.Replace(w, sw[0], sw[1], 1)
			}
		}
	}
	return w
}

func glueFigures(s string) string {
	words := strings.Fields(s)
	out := make([]string, 0, len(words))
	for i := 0; i < len(words); i++ {
		if i+1 < len(words) && isFigure(words[i]) && !isFigure(words[i+1]) {
			out = append(out, words[i]+words[i+1])
			i++
			continue
		}
		out = append(out, words[i])
	}
	return strings.Join(out, " ")
}

func isFigure(w string) bool {
	if w == "" {
		return false
	}
	for _, r := range w {
		if (r < '0' || r > '9') && r != '.' && r != '/' {
			return false
		}
	}
	return true
}

func hasDigitRune(w string) bool {
	for _, r := range w {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

var figures = regexp.MustCompile(`\d+(?:\.\d+)?`)

// nameShape is a name reduced to its identifying words and its figures, in a
// canonical order: two records with the same shape differ only in spacing,
// unit spelling or word order, and are duplicates of one product.
func nameShape(name string) string {
	toks := productmatch.DebugCoreTokens(name)
	nums := figures.FindAllString(sheet.NormalizeDigits(name), -1)
	if len(toks) == 0 {
		return ""
	}
	for i, t := range toks {
		// The catalogue itself spells one product two ways (فينال/فاينال).
		if sk := productmatch.Skeleton(t); sk != "" {
			toks[i] = sk
		}
	}
	sort.Strings(toks)
	sort.Strings(nums)
	return strings.Join(toks, " ") + "|" + strings.Join(nums, " ")
}
