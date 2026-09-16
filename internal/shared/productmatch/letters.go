package productmatch

// The letters that separate one product from its neighbours.
//
// quantity.go reads the FIGURES out of a name. This file reads the LETTERS: the
// N of Betnovate N, the S of Blanka-S, the CR of a modified-release line — and,
// just as importantly, the F and C of "f.c.tabs", which are film-coating and
// name nothing.
//
// The distinction between those two is the whole content of this file, and it
// is worth its own page: reading an abbreviation as an identity raised a false
// conflict on one labelled row in five, and reading an identity as an
// abbreviation is how بتنوفيت ان came to be matched to بتنوفيت سي.

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// identityMarks is the set of single letters a name carries as identity.
//
// Egyptian pharmacy names distinguish whole product lines by one letter:
// بتنوفيت ان (Betnovate N, with neomycin) against بتنوفيت سي (Betnovate C, with
// clioquinol) against plain بتنوفيت; بلانكا اس against بلانكا; مازيمال سي ار,
// which is the modified-release line. Every one of those is a different
// medicine at a different price, and every one of them was invisible: coreTokens
// drops a word shorter than two runes outright, and rarity weighting reduces
// what is left to noise.
//
// Both alphabets fold onto one key, because the same product is written "N" by
// one supplier and "ان" by the next.
//
// A mark counts only where the name also carries a real word, and only after
// one. A name that is nothing but short tokens has no brand for them to be
// modifying, and a leading article is not an identity.
func identityMarks(text string) map[string]struct{} {
	words := strings.Fields(sheet.NormalizeName(text))
	if len(words) < 2 {
		return nil
	}
	var out map[string]struct{}
	seenWord := false
	for i := 0; i < len(words); i++ {
		w := words[i]
		if hasDigit(w) {
			continue
		}
		// A letter the engine already calls noise cannot also be an identity.
		//
		// س and ج are both: they name product lines in principle, and in
		// practice they are how an Egyptian price list writes "سعر ... جنيه"
		// beside the product. coreTokens has dropped them as words since
		// noiseWords existed; reading them as identity marks here meant the
		// same two characters were meaningless in one comparison and decisive
		// in the next, and the decisive one refused hundreds of correct matches
		// per file. StripTradeAnnotations removes most of them before this
		// runs; this is the guard for the ones it cannot see, mid-name.
		if noiseWords[w] {
			continue
		}
		if _, named := letterNames[w]; !named {
			// Anything that is not a letter is a word this letter could be
			// modifying. The bar used to be four runes, which made "ادو جي"
			// carry no mark where "addo-g" carried one — the Arabic brand is
			// three letters long and the English is four, and that is not a
			// difference between two products.
			if len([]rune(w)) >= 2 && !noiseWords[w] && !isMeasureWord(w) {
				seenWord = true
			}
			continue
		}
		if _, ok := letterNames[w]; !ok {
			continue
		}
		// A RUN of letters is an abbreviation, not an identity.
		//
		// "f.c.tabs" is film-coated tablets, "i.v." is intravenous, "u.s.p" is
		// the pharmacopoeia, and the Arabic "اس بي اف" is SPF. Normalisation
		// turns every one of those into consecutive single-letter tokens, and
		// reading them as line extensions raised a false conflict on a fifth of
		// every labelled row measured: the English file writes "abilia 15mg 30
		// f.c.tabs" where the catalogue writes "ابيليا 15مجم 30 قرص", and that
		// is one product which looked like Abilia F C against plain Abilia.
		//
		// The exception is the release codes, which are genuinely written as
		// two letters and genuinely name another product: "200mg c.r." is not
		// "200mg".
		start, end := letterRun(words, i)
		i = end - 1
		if end-start > 1 {
			// Two or more letters together are an abbreviation — film-coated,
			// intravenous, USP — or a release code, and a release code is a
			// line extension rather than an identity letter, so the modifier
			// vocabulary owns it. Either way, nothing to record here.
			continue
		}
		if !seenWord {
			continue // nothing yet for this letter to be an extension of
		}
		key := letterNames[words[start]]
		if key == "" {
			continue
		}
		if out == nil {
			out = make(map[string]struct{}, 2)
		}
		out[key] = struct{}{}
	}
	return out
}

// letterRun returns the bounds of the maximal run of single-letter tokens
// containing i.
//
// Both directions, and the backward half is what makes it work. The tokens of
// an abbreviation are not all identity letters — "i.v." is an i this engine
// does not name and a v it does not name either, "u.s.p" is a u it ignores
// beside an s and a p it does not — so a forward-only scan starting at the
// first letter it RECOGNISES saw "m" alone in "i.m." and read Abilify Maintena
// I.M. as a product line called M.
func letterRun(words []string, i int) (start, end int) {
	isLetter := func(w string) bool {
		if hasDigit(w) || len([]rune(w)) >= minLinkRunes {
			return false
		}
		if _, named := letterNames[w]; named {
			return true
		}
		_, filler := abbreviationLetters[w]
		return filler
	}
	start, end = i, i+1
	for start > 0 && isLetter(words[start-1]) {
		start--
	}
	for end < len(words) && isLetter(words[end]) {
		end++
	}
	return start, end
}

// abbreviationLetters are short tokens that belong to an abbreviation run
// without being identity letters themselves.
//
// They exist so "i.v.", "u.s.p" and "e.c." are recognised as runs and suppress
// the letters beside them. Without them "u s p" would be read as a run of one
// ("s"), and Albumin USP would carry an identity letter it does not have.
var abbreviationLetters = map[string]struct{}{
	"a": {}, "e": {}, "i": {}, "j": {}, "l": {}, "o": {}, "u": {}, "v": {},
	"w": {}, "y": {}, "اي": {}, "ايه": {}, "او": {}, "يو": {}, "في": {},
	"ال": {}, "ام": {},
}

// letterNames folds the ways one letter is written onto a single key.
//
// Only the letters that actually name a product line are listed, and the
// ambiguous ones are deliberately absent: "ال" is the Arabic article and "في" a
// preposition, and reading either as an identity mark would find one in half
// the catalogue. A letter left out of this map is simply not compared, which is
// the treatment every letter received before this file existed — the failure
// mode is a missed distinction, never an invented one.
var letterNames = map[string]string{
	"b": "b", "بي": "b",
	"c": "c", "سي": "c",
	"d": "d", "دي": "d",
	"f": "f", "اف": "f",
	"g": "g", "جي": "g",
	"h": "h", "اتش": "h", "اش": "h",
	"k": "k", "كيه": "k", "كي": "k",
	"m": "m", "ام": "m",
	"n": "n", "ان": "n", "ن": "n",
	"p": "p",
	"q": "q", "كيو": "q",
	"r": "r", "ار": "r",
	"s": "s", "اس": "s", "س": "s",
	"t": "t", "تي": "t",
	"x": "x", "اكس": "x",
	"z": "z", "زد": "z",
	// The bare Arabic letters. Every one of them occurs fewer than thirty times
	// in a twenty-thousand-product catalogue, which is what makes them safe:
	// a letter standing alone in a pharmacy name is a product line, not a word.
	//
	// "و" and "ا" are the exceptions and are left out. The first is the
	// conjunction — "ليمون و عسل" — and the second is a bare alef that carries
	// no sound of its own.
	"ب": "b", "ت": "t", "ج": "g", "د": "d", "ر": "r", "ز": "z",
	"ش": "s", "ص": "s", "ط": "t", "ع": "a", "ف": "f", "ق": "q",
	"ك": "k", "ل": "l", "ه": "h", "ي": "y",
}

// topicalSubForm separates the semi-solid topicals the form key groups together.
//
// formKeyOf maps كريم, مرهم and جل onto one "topical" key, and that grouping is
// right for the veto: a supplier writes كريم loosely and refusing a match over
// it costs more than it saves. It is wrong for CHOOSING between two catalogue
// entries of the same brand, which is exactly what a family like كليروفات كريم
// and كليروفات مرهم requires — those are two products, stocked and priced
// separately, and the row says which one it wants.
//
// So the distinction lives here rather than in vetoableForm: it never refuses a
// match on its own, and it decides between candidates that are otherwise equal.
func topicalSubForm(text string) string {
	for _, w := range strings.Fields(sheet.NormalizeName(text)) {
		for _, run := range letterRuns(w) {
			if sub, ok := topicalWords[run]; ok {
				return sub
			}
		}
	}
	return ""
}

var topicalWords = map[string]string{
	"كريم": "cream", "cream": "cream", "crm": "cream",
	"مرهم": "ointment", "ointment": "ointment", "oint": "ointment",
	"جل": "gel", "جيل": "gel", "gel": "gel",
	"معجون": "paste", "paste": "paste",
	"رغوة": "foam", "فوم": "foam", "foam": "foam",
	"بلسم": "balm", "balm": "balm",
	"زيت": "oil", "oil": "oil",
	"سيروم": "serum", "serum": "serum",
	"بودرة": "powder", "بودره": "powder", "باودر": "powder", "powder": "powder",
	// The wash family. formKeyOf groups a lotion under "topical" and a wash
	// under "wash", and merging those two at the class level — which the
	// discrimination form does, because اكنيل لوسيون and اكنيل غسول are one
	// bottle — would otherwise let a shampoo settle a row asking for a cream.
	// The sub-form is what keeps them apart.
	//
	// Lotion sits with the washes rather than with the creams for the same
	// reason: in this catalogue the two words name the same products.
	"غسول": "wash", "wash": "wash", "لوسيون": "wash", "لوشن": "wash",
	"lotion": "wash", "cleanser": "wash", "مطهر": "wash",
	"شامبو": "shampoo", "shampoo": "shampoo",
	"صابون": "soap", "صابونة": "soap", "صابونه": "soap", "soap": "soap",
	"مضمضة": "mouthwash", "مضمضه": "mouthwash", "mouthwash": "mouthwash",
}
