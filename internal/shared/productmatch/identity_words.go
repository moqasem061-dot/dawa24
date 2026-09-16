package productmatch

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Deciding that two written words name the same thing.
//
// This is the half of the identity check that has to be GENEROUS. A pharmacy
// types "ازرجا" for the catalogue's "ازارجا" and "ارمو ويك" for its "ارموويك",
// and a comparison that calls those strangers throws away the matches the AI
// stage exists to find.
//
// It is also the half that has to be STRICT about one particular shape: a shared
// prefix. "امبريد" and "امبريديل" look alike by every overlap measure and are
// different medicines, and Egyptian brand names collide at the front far more
// often than by chance. So the two acceptance paths are tuned against each
// other — edit distance carries the spelling variants, and trigram overlap sits
// high enough that a prefix collision cannot use it.

// Thresholds for the shared-word evidence check. See sharedProductWord.
const (
	// minLinkRunes is how long a word must be to link two products. Below four
	// letters an Arabic word is usually a fragment or a unit.
	minLinkRunes = 4
	// minEditSimilarity accepts one transliteration variant as the other.
	//
	// Set from the pairs it has to separate. "ازرجا"/"ازارجا" is one inserted
	// letter at 0.83 and must pass; "امبريد"/"امبريديل" is two appended letters
	// at 0.75 and must not, because Ambrid and Ambridyl are different medicines.
	// A shared prefix is the commonest way two unrelated Egyptian brands look
	// alike, so the line sits above it.
	minEditSimilarity = 0.80
	// minTrigramLink is the looser path, for words that differ by more than a
	// letter or two but still share most of their shape.
	//
	// It sits high because trigram overlap is systematically generous to a
	// SHARED PREFIX, which is the commonest way two unrelated Egyptian brands
	// resemble each other: "امبريد" and "امبريديل" have every trigram of the
	// shorter inside the longer and score 0.67, yet Ambrid and Ambridyl are
	// different medicines. Genuine spelling variants are caught by edit distance
	// before this path is reached, so it can afford to be strict.
	minTrigramLink = 0.70
	// commonWordShare is the fraction of the catalogue above which a word stops
	// being able to link two products on its own.
	//
	// Expressed as a share of the catalogue rather than as an inverse-frequency
	// score, because a score is only meaningful against a catalogue size: a
	// floor of 2.0 excludes nothing in a catalogue of two million and everything
	// in a catalogue of three. It also has to tolerate brand families —
	// ابيليفاي names every strength of one brand, so its raw frequency is
	// several and its rarity score correspondingly lower, and a rarity floor
	// would refuse the brand for being sold in three doses.
	//
	// One product in twenty is a word like "فيتامين": shared by a whole
	// category and evidence of nothing. Below that, everything a brand or a
	// molecule is called still counts.
	commonWordShare = 20
)

// sharedProductWord is the check that answers "different products that happen to
// share two words".
//
// It asks for one thing: does some distinctive word of the pharmacy's line
// correspond to some distinctive word of the catalogue product? Not overall
// similarity — that measure is what the scorer already tried and lost on, and it
// is easily reached by two unrelated products that share a manufacturer and a
// dosage form.
//
// Manufacturer names are excluded from both sides, and that exclusion is the
// whole point. On the live catalogue "ابيفيناك حقن /ايبيكو" was matched to
// "سيفوتاكس 500مجم فيال ايبيكو" — two completely different medicines whose only
// shared word is the company that makes them both. The pharmacy wrote the
// distributor after a slash, as Egyptian price lists do, and it read as
// agreement. With makers excluded there is nothing left in common and the match
// is refused.
//
// Correspondence is deliberately generous about spelling, because that is the
// other half of the requirement: "ازرجا" must still find "ازارجا", and "بنادول"
// must still find "بانادول". Exact, near-exact by edit distance, or a strong
// trigram overlap all count.
func (idx *Index) sharedProductWord(row *Row, p *MasterProduct) bool {
	rowWords := withCompounds(coreTokens(row.Name + " " + row.NameEN))
	candWords := withCompounds(append(append([]string{}, p.coreAR...), p.coreEN...))

	for _, rw := range rowWords {
		if len([]rune(rw)) < minLinkRunes || idx.isCompanyWord(rw) {
			continue
		}
		for _, cw := range candWords {
			if len([]rune(cw)) < minLinkRunes || idx.isCompanyWord(cw) {
				continue
			}
			// The catalogue side is the one whose frequency is measured, since
			// it is the side drawn from the catalogue. A word carried by a
			// whole category links nothing.
			//
			// A compound has no frequency of its own — it was joined here, not
			// indexed — and so reads as zero, which is correct: a spelling the
			// catalogue never saw as one word is distinctive, not common.
			if idx.commonWord(cw) {
				continue
			}
			if wordsCorrespond(rw, cw) {
				return true
			}
		}
	}
	return false
}

// withCompounds adds each pair of adjacent words joined without a space.
//
// Whether a brand is one word or two is a typing decision, not a product
// difference: the catalogue says "ارموويك" and the pharmacy typed "ارمو ويك";
// the catalogue says "الفرين سبازم" and the pharmacy typed "الفيرينسبازم". A
// word-by-word comparison reads those as strangers. Joining adjacent pairs puts
// both spellings in the same vocabulary, at the cost of one extra string per
// word.
func withCompounds(words []string) []string {
	if len(words) < 2 {
		return words
	}
	out := make([]string, 0, len(words)*2)
	out = append(out, words...)
	for i := 0; i+1 < len(words); i++ {
		out = append(out, words[i]+words[i+1])
	}
	return out
}

// commonWord reports whether a catalogue word is too widely shared to identify
// a product on its own.
func (idx *Index) commonWord(word string) bool {
	ceiling := idx.total / commonWordShare
	if ceiling < 64 {
		// Below a few hundred products every word looks rare; a floor keeps a
		// small or freshly seeded catalogue from filtering out its whole
		// vocabulary.
		ceiling = 64
	}
	return idx.df[word] > ceiling
}

// wordsCorrespond decides whether two words name the same thing spelled two ways.
func wordsCorrespond(a, b string) bool {
	if a == b {
		return true
	}
	if editSimilarity(a, b) >= minEditSimilarity {
		return true
	}
	return jaccardSorted(sortedTrigrams([]string{a}), sortedTrigrams([]string{b})) >= minTrigramLink
}

// editSimilarity is Levenshtein distance normalised by the longer word.
//
// Trigram overlap is the wrong tool for short brand names: "ازرجا" and "ازارجا"
// share one trigram out of six and score 0.17, yet they differ by a single
// inserted letter. Edit distance says 0.83, which is what a reader sees.
func editSimilarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}
	longest := len(ra)
	if len(rb) > longest {
		longest = len(rb)
	}
	// Two words of very different lengths are not spelling variants of each
	// other, and skipping them keeps the quadratic work off the hot path.
	if longest-min(len(ra), len(rb)) > longest/2 {
		return 0
	}

	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(min(cur[j-1]+1, prev[j]+1), prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return 1 - float64(prev[len(rb)])/float64(longest)
}

// modifiersIn is the set of line-extension keys a text carries.
//
// Adjacent letters are folded first. A modified-release line is written "sr" by
// one supplier, "s.r." by the next and "اس ار" by the catalogue, and only the
// first of those is a token this vocabulary holds — so the same product
// contradicted itself across scripts, which is the worst possible outcome for a
// check whose whole purpose is to separate one product from another.
func modifiersIn(text string) map[string]struct{} {
	words := strings.Fields(sheet.NormalizeName(text))
	out := make(map[string]struct{}, 2)
	for i, w := range words {
		if key, ok := variantModifiers[w]; ok {
			out[key] = struct{}{}
			continue
		}
		if i+1 >= len(words) {
			continue
		}
		if key, ok := releaseCodePair(w, words[i+1]); ok {
			out[key] = struct{}{}
		}
	}
	return out
}

// releaseCodePair folds two adjacent letters onto the release code they spell.
//
// "اس"+"ار" is SR and "سي"+"ار" is CR, in a catalogue that writes both that way
// and a supplier file that writes them "sr" and "c.r.". Only the codes already
// in the modifier vocabulary are recognised, so this widens how a known
// extension may be spelled and never invents one.
func releaseCodePair(a, b string) (string, bool) {
	first, ok := letterNames[a]
	if !ok || first == "" {
		return "", false
	}
	second, ok := letterNames[b]
	if !ok || second == "" {
		return "", false
	}
	key, ok := variantModifiers[first+second]
	return key, ok
}

// releaseCodes are the two-letter runs that name a different product rather
// than abbreviating a dosage form. They are compared as modifiers, not as
// identity letters — see modifiersIn.
var releaseCodes = map[string]bool{
	"sr": true, "cr": true, "xr": true, "mr": true, "er": true,
	"xl": true, "la": true, "dr": true, "od": true,
}

// variantModifiers are the words that make one product in a brand family a
// different product from another, mapped onto a common key so that the Arabic
// and Latin spellings of one modifier compare equal.
//
// Every entry earns its place by separating two real products in this
// catalogue. The list is deliberately curated rather than generated: a word
// that is merely descriptive ("اوبتيزورب", "فيلم كوتد") must NOT be here, because
// its absence from a pharmacy's terser line is not evidence of anything.
//
// Single letters and two-letter Latin release codes are included because that
// is exactly how modified-release products are written — "50 SR" and "50" are
// different medicines — and they are matched as whole tokens, so they cannot
// fire on a fragment of a longer word.
var variantModifiers = map[string]string{
	// Strength-family extensions
	"بلس": "plus", "بلاس": "plus", "plus": "plus",
	"فورت": "forte", "فورتي": "forte", "forte": "forte", "fort": "forte",
	"اكسترا": "extra", "extra": "extra",
	"ادفانس": "advance", "advance": "advance", "advanced": "advance",
	"ماكس": "max", "max": "max", "maximum": "max",
	"الترا": "ultra", "ultra": "ultra",
	"توتال": "total", "total": "total",
	"بلص": "plus",

	// Indication extensions — the same brand sold for a different complaint
	"نايت": "night", "night": "night",
	"داي": "day", "day": "day",
	"كولد": "cold", "cold": "cold",
	"فلو": "flu", "flu": "flu",
	"ساينس": "sinus", "ساينوس": "sinus", "sinus": "sinus",
	"مايجرين": "migraine", "migraine": "migraine",
	"جوينت": "joint", "joint": "joint",

	// Orally disintegrating lines. "ابيليفاي 10مجم" and "ابيليفاي 10مجم ديسكملت"
	// are the same molecule at the same dose and are stocked, priced and ordered
	// as two products; the model matched one to the other until this was here.
	"ديسكملت": "odt", "discmelt": "odt", "odt": "odt",
	"ذائبه": "odt", "ذائبة": "odt", "زيديس": "odt", "zydis": "odt",

	// Release profile — a modified-release tablet is not an immediate one
	"ريتارد": "retard", "retard": "retard",
	"فاست": "fast", "fast": "fast",
	"sr": "sr", "cr": "cr", "xr": "xr", "mr": "mr", "xl": "xl", "la": "la",
	"er": "er", "od": "od",
	"ممتد": "sr", "المفعول": "sr",

	// Population — the paediatric line is a different product.
	//
	// Every paediatric word folds onto ONE key. The catalogue writes "اطفال"
	// where the supplier writes "baby" and "children" where it writes "بيبي",
	// and three separate keys made those contradict each other: a fifth of the
	// modifier conflicts measured against known-correct pairs were a
	// paediatric product disagreeing with itself in the other language.
	"بيبي": "paed", "بيبى": "paed", "baby": "paed", "infant": "paed",
	"جونيور": "paed", "junior": "paed", "بيديا": "paed", "pedia": "paed",
	"kids": "paed", "children": "paed", "child": "paed", "paed": "paed",
	"paediatric": "paed", "pediatric": "paed",
	"اطفال": "paed", "للاطفال": "paed", "الاطفال": "paed", "اطفل": "paed",
	"كبار": "adult", "للكبار": "adult", "adult": "adult", "adults": "adult",
	"مان": "men", "men": "men", "man": "men", "رجالي": "men", "للرجال": "men",
	"الرجال": "men", "رجالى": "men",
	"وومن": "women", "women": "women", "حريمي": "women", "للسيدات": "women",
	"السيدات": "women", "سيدات": "women", "نسائي": "women", "للنساء": "women",

	// Route and presentation lines sold beside the ordinary one
	"للمضغ": "chew", "مضغ": "chew", "للمص": "chew", "chewable": "chew", "chew": "chew",
	"فوار": "eff", "فوارة": "eff", "فواره": "eff", "effervescent": "eff", "eff": "eff",

	// Formulation families sold side by side
	"دوو": "duo", "duo": "duo",
	"تريو": "trio", "trio": "trio",
	"كومب": "comb", "comb": "comb", "combi": "comb",
	"زيرو": "zero", "zero": "zero",
	"لايت": "light", "light": "light",
	"جولد": "gold", "gold": "gold",
	"بلاتينيوم": "platinum", "platinum": "platinum",
}

// Not in the list, deliberately:
//
//   - "hct" and the other combination markers. The Arabic writes them as three
//     separate tokens ("اتش سي تي") and the Latin as one, so the check fired on
//     writing style rather than on identity and refused "اكسفورج اتش" against
//     "اكسفورج اتش سي تي" — the same medicine. Combinations are separated by the
//     strength rule and by the prompt instead.
//   - Descriptive formulation words ("اوبتيزورب", "فيلم كوتد", "اس ار" written
//     out). Their absence from a pharmacy's terser line is not evidence of
//     anything.
