package productmatch

import "strings"

// Comparing an Arabic name with a Latin one.
//
// Every product in the catalogue carries both names — 19,996 of 19,996 — and
// every supplier and pharmacy file is written in one of them. The scorer could
// not cross between: Arabic query tokens were compared against the catalogue's
// Arabic tokens and Latin against Latin, and the trigram fallback is built per
// token, so it shares nothing across scripts either. "بوباى صن سكرين" against
// "bobai sunscreen" scored exactly zero, and the two rows are the same product.
//
// The AI prompt is told about transliteration at length. The deterministic
// engine that runs first, on every row, was not told at all — so the cases the
// model was being paid to resolve were mostly cases arithmetic could have
// resolved for nothing.
//
// The bridge is a skeleton both scripts fold into: consonants only, one letter
// per sound, doubles collapsed. It is deliberately lossy. Egyptians write the
// same brand with and without long vowels ("بنادول" / "بانادول"), with ى or ي,
// with ه or ة, and transliterate the same Latin consonant several ways; a
// skeleton that survives all of that is worth more than one that is faithful.
//
// It is scored as a third, discounted channel — never above a shared whole word
// — because two different products can skeletonise alike and a shared word
// cannot happen by accident.

// arabicSkeleton maps each Arabic letter onto its Latin consonant sound.
//
// Vowel carriers (ا و ي) map to nothing: Arabic omits short vowels and Latin
// spells them, so the only way the two can agree is if neither is compared.
// That is also what makes "بنادول" and "بانادول" the same skeleton, which is
// the commonest spelling variation in real files.
var arabicSkeleton = map[rune]string{
	'ا': "", 'أ': "", 'إ': "", 'آ': "", 'ء': "", 'ؤ': "", 'ئ': "", 'ى': "", 'ي': "", 'و': "",
	'ب': "b", 'پ': "b",
	'ت': "t", 'ط': "t", 'ث': "s",
	'ج': "g", 'چ': "g",
	'ح': "h", 'ه': "h", 'ة': "h", 'خ': "k",
	'د': "d", 'ض': "d", 'ذ': "z",
	'ر': "r",
	'ز': "z", 'ظ': "z",
	'س': "s", 'ص': "s", 'ش': "s",
	'ع': "", 'غ': "g",
	'ف': "f", 'ڤ': "f",
	'ق': "k", 'ك': "k",
	'ل': "l", 'م': "m", 'ن': "n",
}

// latinSkeleton maps a Latin letter onto the same alphabet.
//
// The vowels drop for the same reason they do above. q and x are folded onto
// the sounds Arabic actually has, and latinFold handles the letters whose sound
// depends on what follows them — c and the digraphs ph, sh, ch, th, gh, kh —
// before this map is consulted. c is therefore absent here on purpose: an entry
// for it could only ever be one of its two sounds, and choosing either one
// unconditionally is the bug latinFold exists to fix.
var latinSkeleton = map[rune]string{
	'a': "", 'e': "", 'i': "", 'o': "", 'u': "", 'y': "", 'h': "h", 'w': "",
	'b': "b", 'd': "d", 'f': "f", 'g': "g", 'j': "g", 'k': "k",
	'l': "l", 'm': "m", 'n': "n", 'p': "b", 'q': "k", 'r': "r", 's': "s",
	't': "t", 'v': "f", 'x': "ks", 'z': "z",
}

// minSkeletonRunes is the shortest skeleton worth comparing.
//
// Two consonants match by coincidence far too often — "دار" and "دور" both
// reduce to "dr" — and a two-letter agreement is not evidence of anything.
const minSkeletonRunes = 3

// Skeleton reduces a name to the consonant sounds both scripts share.
//
// Exported because the recall index keys on it too: a query is retrieved by its
// skeleton as well as by its words, which is what puts a product back on the
// shortlist when the pharmacy wrote the brand in the other alphabet.
func Skeleton(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s) / 2)

	runes := []rune(strings.ToLower(s))
	last := byte(0)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		fold, known := arabicSkeleton[r]
		width := 1
		if !known {
			fold, width, known = latinFold(runes, i)
		}
		if !known {
			// Digits and punctuation carry no sound. Figures are compared
			// separately as a number signature, and dropping them here is what
			// lets "بروفين600" and "brufen 600" reduce alike.
			continue
		}
		last = writeSkeleton(&b, fold, last)
		i += width - 1
	}
	return b.String()
}

// latinFold reads one Latin sound, which may be spelled with one letter or two.
//
// It replaced a context-free digraph table, and the context is the whole point:
// c and ch each spell two different sounds in this vocabulary, and folding
// either one of them unconditionally merged brands that are not related.
//
//	cisplatin   folded to ksbltn and collided with اوكسابلاتين (oxaliplatin);
//	            سيسبلاتين, which is what it actually is, folds to sbltn.
//	cefidime    folded to kfdm and matched كيفاديم instead of سيفيديم.
//	chromax     folded to srmks and collided with سيرومكس;
//	            كروماكس, which is what it actually is, folds to krmks.
//
// Every one of those was an applied match in the labelled corpus — a wrong
// product, priced, with "86% name similarity" printed beside it as the reason.
//
// The rules are the ordinary English ones, which is what a transliterating
// pharmacist is following:
//
//   - c before e, i or y is soft (/s/); elsewhere it is hard (/k/).
//   - ch before a consonant is the Greek /k/ — chlor-, chrom-, chron- — and
//     before a vowel it is the everyday /ʃ/: charcoal, chocolate.
//
// width is how many runes the sound consumed, so the caller can advance.
func latinFold(runes []rune, i int) (fold string, width int, ok bool) {
	r := runes[i]
	next, after := rune(0), rune(0)
	if i+1 < len(runes) {
		next = runes[i+1]
	}
	if i+2 < len(runes) {
		after = runes[i+2]
	}

	switch r {
	case 'p':
		if next == 'h' {
			return "f", 2, true
		}
	case 's', 't':
		if next == 'h' {
			return "s", 2, true
		}
	case 'g':
		if next == 'h' {
			return "g", 2, true
		}
	case 'k':
		if next == 'h' {
			return "k", 2, true
		}
	case 'c':
		switch {
		case next == 'h' && !isLatinVowel(after):
			return "k", 2, true
		case next == 'h':
			return "s", 2, true
		case next == 'k':
			return "k", 2, true
		case isSoftening(next):
			return "s", 1, true
		}
		return "k", 1, true
	}

	f, known := latinSkeleton[r]
	return f, 1, known
}

// isSoftening reports whether a following letter makes a preceding c soft.
func isSoftening(r rune) bool { return r == 'e' || r == 'i' || r == 'y' }

// isLatinVowel reports whether r is a written vowel, which is what decides
// whether a "ch" is Greek or English. A word ending in "ch" counts as followed
// by a consonant, which is right: "-tech" is /k/.
func isLatinVowel(r rune) bool {
	switch r {
	case 'a', 'e', 'i', 'o', 'u', 'y':
		return true
	}
	return false
}

// writeSkeleton appends a fold, collapsing a repeat of the previous sound.
//
// Doubling is spelling, not sound: "أوجمنتين" and "augmentin" agree, and so do
// "ammox" and "amox", only once the repeat is gone.
func writeSkeleton(b *strings.Builder, fold string, last byte) byte {
	if fold == "" {
		return last
	}
	for i := 0; i < len(fold); i++ {
		c := fold[i]
		if c == last {
			continue
		}
		b.WriteByte(c)
		last = c
	}
	return last
}

// skeletonOf reduces a set of already-tokenised words to one skeleton.
//
// Joined without separators on purpose: a supplier who writes "صن سكرين" and a
// catalogue that writes "sunscreen" must agree, and they only can if word
// boundaries are not part of the comparison. That is the same reason the
// spaceless name key exists.
func skeletonOf(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	sk := Skeleton(strings.Join(tokens, ""))
	if len([]rune(sk)) < minSkeletonRunes {
		return ""
	}
	return sk
}

// skeletonSimilarity compares two skeletons on their character trigrams.
//
// Trigrams rather than equality, because a skeleton is long enough that one
// extra consonant — a supplier writing the manufacturer into the name — would
// otherwise lose the whole match.
func skeletonSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	return jaccardSorted(sortedStringTrigrams(a), sortedStringTrigrams(b))
}

// sortedStringTrigrams builds the sorted, de-duplicated trigram set of one
// string, in the shape jaccardSorted expects.
func sortedStringTrigrams(s string) []trigram {
	// A skeleton is one word by construction, so hashing its windows directly
	// is exactly right; there is nothing to join.
	return trigramsOf(s)
}
