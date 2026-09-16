package productmatch

// The trade annotation an Egyptian distributor types after the product name.
//
// A price list is not a catalogue. The name column of a real distributor file
// carries the product AND the terms of sale, run together in one cell, because
// the person maintaining it reads the whole line at once:
//
//	بانتولوك 20 مج 14 قرص ج/باكت 10     the product, wholesale, ten to a carton
//	اتوريزا 10/20 مجم 21 قرص س 141 ج    the product, priced at 141 pounds
//	افيتركس جيل س ج2                     the product, price in pounds
//
// Everything after the form in each of those is bookkeeping. The catalogue says
// بانتولوك 20مجم 14 قرص and means the same medicine at the same price — and the
// engine refused all three, because the annotation is made of exactly the two
// things it reads as identity: single letters and figures.
//
// The س and the ج became identity marks — the letters that separate بتنوفيت ان
// from بتنوفيت سي — so a row agreeing with its catalogue entry on the brand, the
// strength and the form was ranked behind everything that did not carry them.
// The carton count became a stray figure the catalogue's pack count
// contradicted. Measured on nine live supplier files, that one reading was the
// largest single cause of an unmatched row: more than the retrieval misses, the
// form conflicts and the strength conflicts put together.
//
// So the tail is cut before anything reads the name. Cutting rather than
// filtering, and only a TAIL, because that is the shape the convention actually
// has: the annotation is appended after the product is fully described, never
// interleaved with it. That shape is what makes the rule safe on the names it
// must not touch — دلتافيت ب 12 اقراص is a vitamin whose B12 sits BEFORE a form
// word, so no suffix of it is all annotation and nothing is cut, while
// ريكوكسبرايت 60 مج اقراص 3 شريط ب12 ends in a carton of twelve and loses it.

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// annotationStarters are the words that can begin a trade annotation.
//
// Each one is already treated as noise where the engine reads WORDS (see
// noiseWords); what this set adds is that they also END the product name, so
// the figures beside them stop being read as the product's own.
//
// باكت is the carton — "/باكت 60" is sixty units to a box — and it is the one
// that was missing everywhere: it is not in noiseWords either, so it also
// reached the scorer as ordinary vocabulary.
var annotationStarters = map[string]bool{
	"باكت": true, "الباكت": true, "باكيت": true, "بكت": true,
	"باكو": true, "الباكو": true, "باكه": true,
	"سعر": true, "السعر": true, "بسعر": true, "س": true,
	"جنيه": true, "ج": true, "جمله": true,
}

// annotationFillers may appear inside an annotation but never begin one.
//
var annotationFillers = map[string]bool{
	"م":     true,
	"جديد":  true,
	"قديم":  true,
	"جديدة": true,
	"قديمة": true,
}

// conditionalStarter is the carton "ب": "ب100", "/ب 10", "3 شريط ب12".
//
// It cannot be trusted on its own, because the same letter names the vitamin B
// lines — دلتافيت ب12, ميثيل تكنو ب 12 — and stripping those leaves a brand with
// no strength at all. It counts only where the word before it has already
// finished describing the product: a form, a unit, or another annotation. In
// "3 شريط ب12" the strip count comes first and the ب is a carton; in
// "دلتافيت ب 12" the brand comes first and the ب is the vitamin.
const conditionalStarter = "ب"

// StripTradeAnnotations removes a distributor's trailing sale terms from a
// product name, returning the name as written up to the cut.
//
// A prefix of the ORIGINAL string is returned rather than a rebuilt one, so a
// combination dose written "10/20 مجم" survives intact. Rebuilding would have to
// decide what to do with the separator, and this function has no business
// making that decision.
func StripTradeAnnotations(name string) string {
	if name == "" {
		return name
	}
	toks := annotationTokens(name)
	if len(toks) < 2 {
		return name
	}

	// Walk from the end. The cut is the leftmost token from which everything
	// remaining is annotation, so the scan stops at the first token that is
	// part of the product — which is where the product name ends.
	cut := -1
	for i := len(toks) - 1; i >= 0; i-- {
		t := toks[i]
		switch {
		case t.starter:
			cut = i
		case t.conditional:
			if i > 0 && closesProduct(toks[i-1]) {
				cut = i
				continue
			}
			return cutAt(name, toks, cut)
		case t.filler:
			// A figure carries on an annotation that a starter to its left
			// will open, and says nothing on its own.
		default:
			return cutAt(name, toks, cut)
		}
	}
	// Every token was annotation. A name that is nothing but sale terms is not
	// one this function can improve, so it is left alone.
	return name
}

// cutAt truncates the name at a token boundary, or returns it whole when no
// annotation was found. A cut at the first token is refused: whatever else the
// name is, it is not entirely bookkeeping.
func cutAt(name string, toks []annotationToken, cut int) string {
	if cut <= 0 || cut >= len(toks) {
		return name
	}
	// The separator that introduced the annotation goes with it. Cutting on the
	// token boundary alone leaves "اقراص/" behind, and a trailing slash is one
	// more thing for every reader downstream to have an opinion about.
	return strings.TrimRightFunc(name[:toks[cut].start], isAnnotationBreak)
}

// annotationToken is one piece of a name, classified, and remembered by where
// it starts in the original string.
type annotationToken struct {
	norm  string
	start int

	starter     bool
	conditional bool
	filler      bool
	// closer marks a token that finishes a product description — a dosage form
	// or a sales unit — which is what licenses a conditional carton after it.
	closer bool
}

// closesProduct reports whether a token is the kind that ends a product
// description, which is what licenses the conditional carton marker after it.
func closesProduct(t annotationToken) bool {
	return t.closer || t.starter || t.filler || t.conditional
}

// annotationTokens splits a name the way the convention writes it: on
// whitespace and on the punctuation that separates the product from its terms.
// The slash is the important one — "اقراص/باكت 60" is written without a space,
// and a whitespace-only split leaves the carton glued to the form.
func annotationTokens(name string) []annotationToken {
	var out []annotationToken
	start := -1
	for i, r := range name {
		if isAnnotationBreak(r) {
			if start >= 0 {
				out = append(out, classifyToken(name[start:i], start))
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, classifyToken(name[start:], start))
	}
	return out
}

// isAnnotationBreak reports the separators the convention uses.
//
// The full stop is included for "ج.م", and it costs nothing on a decimal: the
// cut always lands on a marker's own offset, so no figure is ever truncated
// through its point.
func isAnnotationBreak(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '/', '\\', '|', ',', '،', ';', '؛', '.':
		return true
	}
	return false
}

// classifyToken says what one token is: the start of an annotation, something
// that can only continue one, or a word of the product itself.
//
// A token is read as its letter runs and its figures rather than whole, because
// the two are typed against each other in every direction: "س100", "باكت90",
// "28ج", "120ج", "ب12". Whole-token lookups caught half of those and left the
// other half looking like product words, which blocked the cut for the entire
// name behind them.
func classifyToken(raw string, start int) annotationToken {
	t := annotationToken{norm: sheet.NormalizeName(raw), start: start}
	if t.norm == "" {
		t.filler = true
		return t
	}
	words := letterRuns(t.norm)
	if len(words) == 0 {
		// Nothing but figures.
		t.filler = isNumericLiteral(t.norm)
		return t
	}
	starter, conditional := false, false
	for _, w := range words {
		switch {
		case annotationStarters[w]:
			starter = true
		case w == conditionalStarter:
			conditional = true
		case annotationFillers[w]:
		default:
			// A real word. Whatever else the token holds, it is part of the
			// product.
			t.closer = isMeasureWord(t.norm) || formKeyOf(t.norm) != ""
			return t
		}
	}
	switch {
	case starter:
		t.starter = true
	case conditional:
		t.conditional = true
	default:
		t.filler = true
	}
	return t
}
