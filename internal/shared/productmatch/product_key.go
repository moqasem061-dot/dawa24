package productmatch

import (
	"sort"
	"strconv"
	"strings"
)

// The identity key two supplier price lists are grouped by.
//
// Everything else in this package answers "is this ROW the same product as this
// CATALOGUE ENTRY". The compare tool asks a different question — "is this row
// from supplier A the same product as this row from supplier B" — and there is
// no catalogue in it at all. Both files are somebody's spreadsheet.
//
// That question was answered by a second matcher living inside
// internal/modules/compare: its own normaliser, its own noise-word list, its own
// strength regex, and a hand-written table of about sixty Arabic brand names
// mapped to their Latin spellings. Sixty, against a market of twenty thousand
// products. Everything outside that table failed to group at all:
//
//	"سيفيديم 500 مجم فيال"   and  "cefidime 500mg vial"     -> two rows
//	"زيرتك 10 مجم 20 قرص"     and  "zyrtec 10mg 20 tabs"     -> two rows
//	"ابيكوبريد 40 مجم"        and  "ابيكوبرايد 40 مجم"       -> two rows
//	"الكور بلس 10/20 مجم"     and  "الكور بلس 20/10 مجم"     -> two rows
//
// and one thing grouped that must not, because bare figures of three digits or
// fewer were discarded as noise:
//
//	"اتاكاند 16"              and  "اتاكاند 32"              -> ONE row
//
// The last is the worst of them. A pharmacist comparing prices was shown a
// single line carrying offers for two different strengths of a blood-pressure
// medicine, and the "best price" on it was the cheaper drug rather than the
// cheaper supplier.
//
// ProductKey answers the same question with the machinery the rest of the
// platform already uses: the consonant skeleton that folds both alphabets and
// most Egyptian spelling variation (translit.go, variants.go), the curated
// modifier vocabulary that separates a brand from its line extensions
// (identity_words.go), the identity letters (letters.go), and the dose and pack
// readers that understand a ratio (strength_set.go, quantity.go).
//
// It is a KEY, not a score: two rows group when their keys are equal. That is
// what the compare tool's index needs, and it is why this is stricter than
// Index.Match — a scorer may weigh a disagreement, a key cannot.

// ProductKey reduces a supplier's product name to the identity two files are
// grouped by.
//
// Returns "" for a name with nothing identifying in it, which callers must treat
// as "do not group this row" rather than as a key of its own — an empty key
// shared by every unreadable line would merge them all into one.
func ProductKey(name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}

	var parts []string

	// 1. The brand, folded so that two alphabets and two spellings agree.
	//
	// A token long enough to fold safely contributes its skeleton; one too
	// short contributes itself. minVariantKeyRunes is four consonants because
	// three collide freely — "دار" and "دور" both reduce to "dr" — and a brand
	// short enough to be excluded is short enough to be spelled the same way
	// twice.
	marks := identityMarks(name)
	brand := make([]string, 0, 4)
	for _, tok := range coreTokens(name) {
		if _, isModifier := variantModifiers[tok]; isModifier {
			continue // carried below, in canonical form
		}
		// An identity letter is carried below too, and must not also appear
		// here: "بتنوفيت ان" would contribute the raw token "ان" where
		// "betnovate n" contributes "n", and the two spellings of one product
		// would key differently on the very letter that identifies it. A letter
		// is too short to fold, so this is the only place it can be normalised.
		if key, named := letterNames[tok]; named {
			if _, isMark := marks[key]; isMark {
				continue
			}
		}
		if key := variantKeyOf(tok); key != "" {
			brand = append(brand, key)
			continue
		}
		brand = append(brand, tok)
	}
	if len(brand) == 0 {
		return ""
	}
	// Sorted, because one supplier writes "ليميتلس اولزايم" and the next writes
	// "اولزايم ليميتلس" and they are the same box.
	sort.Strings(brand)
	parts = append(parts, "n:"+strings.Join(brand, "-"))

	// 2. The line extension. "بانادول" and "بانادول اكسترا" are two products,
	//    and so are "plus" written in either alphabet.
	if mods := sortedKeys(modifiersIn(name)); len(mods) > 0 {
		parts = append(parts, "m:"+strings.Join(mods, "-"))
	}

	// 3. The identity letter. بتنوفيت ان and بتنوفيت سي are different medicines
	//    at different prices, and the whole difference is one letter.
	if list := sortedKeys(marks); len(list) > 0 {
		parts = append(parts, "l:"+strings.Join(list, "-"))
	}

	// 4. Every dose the name states, in a common base unit, sorted.
	//
	// Sorted rather than in written order, so "10/20 مجم" and "20/10 مجم" are
	// one product — and every component present, so "10/20" and "10/40" are not.
	// This is what the old regex could not do: it read one figure out of a ratio
	// and discarded the rest.
	if doses := doseSignature(strengthSet(name)); doses != "" {
		parts = append(parts, "d:"+doses)
	}

	// 5. What the pack holds. A strip of 20 and a box of 200 are priced
	//    separately and are not the same offer.
	qty := readQuantities(name)
	if counts := countSignature(qty); counts != "" {
		parts = append(parts, "c:"+counts)
	}

	// 6. A bare figure, but ONLY when the name states nothing else numeric.
	//
	// "اتاكاند 16" and "اتاكاند 32" are two strengths of a blood-pressure
	// medicine written without their unit, which every Egyptian price list does
	// somewhere. With the figure discarded they keyed identically, and the
	// compare screen showed ONE row carrying offers for two different medicines
	// with a "best price" that was simply the cheaper drug. That is the worst
	// thing this tool can do, because it is wrong rather than merely unhelpful.
	//
	// The condition is what keeps it safe. A residual is a figure whose noun
	// this engine could not read — a lot number, half a bonus notation, a wipe
	// count — and keying on one beside a real dose would split products that
	// two suppliers merely described with different amounts of detail. So it
	// speaks only when it is the sole numeric thing in the name, where it is
	// far more likely to be the strength than to be noise.
	//
	// The trade it accepts: "كريم 50" and "كريم 50 جم" key differently. That is
	// two rows for one product, which a reader can see and reconcile — as
	// against one row for two products, which a reader cannot.
	if len(qty.counts) == 0 && len(strengthSet(name)) == 0 && len(qty.residual) > 0 {
		figures := make([]string, 0, len(qty.residual))
		for _, v := range qty.residual {
			figures = append(figures, formatNumber(v))
		}
		sort.Strings(figures)
		parts = append(parts, "r:"+strings.Join(dedupeSorted(figures), "-"))
	}

	return strings.Join(parts, "|")
}

// doseSignature renders a dose set as a stable string.
func doseSignature(set []strength) string {
	if len(set) == 0 {
		return ""
	}
	out := make([]string, 0, len(set))
	for _, s := range set {
		if !s.known() {
			continue
		}
		out = append(out, formatNumber(s.value)+s.unit)
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	return strings.Join(dedupeSorted(out), "-")
}

// countSignature renders what a name counted, as a stable string.
//
// Only the counts, never the residual figures. A residual is a number whose
// noun this engine does not know — a wipe count, a lot number, half a bonus
// notation — and keying on one would split a product two suppliers describe
// with different amounts of detail.
func countSignature(q quantities) string {
	if len(q.counts) == 0 {
		return ""
	}
	out := make([]string, 0, len(q.counts))
	for _, c := range q.counts {
		out = append(out, c.class+":"+formatNumber(c.value))
	}
	sort.Strings(out)
	return strings.Join(dedupeSorted(out), "-")
}

// formatNumber renders a figure without a trailing ".0", so 500 and 500.0 are
// the same key.
func formatNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func sortedKeys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func dedupeSorted(in []string) []string {
	out := in[:0]
	var prev string
	for i, v := range in {
		if i > 0 && v == prev {
			continue
		}
		out = append(out, v)
		prev = v
	}
	return out
}
