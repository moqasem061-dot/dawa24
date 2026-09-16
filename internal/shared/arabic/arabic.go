// Package arabic normalises Arabic text for search and fuzzy product matching.
//
// This is a faithful port of the legacy App\Services\ArabicNormalizer. The
// normalisation rules and — critically — the similarity scoring formula are
// reproduced exactly, including their quirks, because import runs are compared
// against the legacy system for parity and because import_sessions rows carry a
// stored min_similarity_score (default 0.85) that was tuned against this exact
// curve. Changing the formula would silently re-tune every vendor's import.
//
// Improvements to matching quality belong in a new scorer behind a feature flag,
// after cutover, with the parity suite as evidence.
package arabic

import (
	"strings"
	"unicode"
)

// arabicIndicDigits maps ٠-٩ onto 0-9. Vendor catalogues mix both freely, often
// within one product name ("باراسيتامول ٥٠٠" vs "باراسيتامول 500").
var arabicIndicDigits = map[rune]rune{
	'٠': '0', '١': '1', '٢': '2', '٣': '3', '٤': '4',
	'٥': '5', '٦': '6', '٧': '7', '٨': '8', '٩': '9',
}

// punctuation is replaced with a space rather than deleted, matching the legacy
// rule. Deleting would fuse "50/100" into "50100"; replacing keeps the tokens
// apart, which is what dosage strings need.
const punctuation = `%-_/\()[]{}.,`

// Normalize collapses the orthographic variation that makes Arabic catalogue
// text hard to match: hamza placement, taa marbuta vs haa, alif maqsura vs yaa,
// tatweel padding, and diacritics. Two spellings of the same medicine name
// converge on one form.
func Normalize(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(text))

	for _, r := range text {
		switch {
		// 1. Arabic-Indic digits to Latin.
		case arabicIndicDigits[r] != 0:
			b.WriteRune(arabicIndicDigits[r])

		// 2. Alif variants (أ إ آ ٱ) to bare alif.
		case r == 'أ' || r == 'إ' || r == 'آ' || r == 'ٱ':
			b.WriteRune('ا')

		// 3. Taa marbuta to haa. Word-final ة and ه are used interchangeably in
		//    practice, so collapsing them is what makes "شربة"/"شربه" match.
		case r == 'ة':
			b.WriteRune('ه')

		// 4. Alif maqsura to yaa, for the same reason.
		case r == 'ى':
			b.WriteRune('ي')

		// 5. Tatweel is a purely typographic stretch character; drop it.
		case r == 'ـ':

		// 6. Tashkeel (U+064B..U+065F) is optional vocalisation; drop it.
		case r >= 0x064B && r <= 0x065F:

		// 7. Catalogue punctuation becomes a space.
		case strings.ContainsRune(punctuation, r):
			b.WriteRune(' ')

		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}

	// 8. Collapse runs of whitespace.
	return strings.Join(strings.Fields(b.String()), " ")
}

// Similarity scores two strings from 0 to 1 after normalisation.
//
// The curve is the legacy one and is deliberately not a plain edit-distance
// ratio:
//
//	exact match after normalisation      -> 1.0
//	one string contains the other        -> 0.80 + 0.18 * (shorter/longer)
//	otherwise                            -> 1 - levenshtein/maxLen
//
// The containment branch exists because vendor files routinely carry a longer
// form of the same product ("بانادول اكسترا 500 مجم" vs "بانادول اكسترا"), and
// plain edit distance scores those far too low to clear the 0.85 threshold.
func Similarity(a, b string) float64 {
	na, nb := Normalize(a), Normalize(b)

	if na == nb {
		// Two empty strings normalise equal; the legacy code returns 1.0 here
		// before its emptiness check, so we match that ordering exactly.
		return 1.0
	}
	if na == "" || nb == "" {
		return 0.0
	}

	ra, rb := []rune(na), []rune(nb)

	if strings.Contains(na, nb) || strings.Contains(nb, na) {
		shorter, longer := len(ra), len(rb)
		if shorter > longer {
			shorter, longer = longer, shorter
		}
		return 0.80 + 0.18*(float64(shorter)/float64(longer))
	}

	maxLen := len(ra)
	if len(rb) > maxLen {
		maxLen = len(rb)
	}
	score := 1 - float64(levenshtein(ra, rb))/float64(maxLen)
	if score < 0 {
		return 0
	}
	return score
}

// Matches reports whether two strings are at least threshold similar.
func Matches(a, b string, threshold float64) bool {
	return Similarity(a, b) >= threshold
}

// levenshtein is the standard two-row dynamic programme over runes.
//
// Runes, not bytes: Arabic is multi-byte in UTF-8, and a byte-wise distance
// would count a single substituted letter as two or three edits, distorting
// every score. This is the bug the legacy code's mb_levenshtein existed to
// avoid, and Go's []rune conversion gives it to us for free.
func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 0; i < len(a); i++ {
		curr[0] = i + 1
		for j := 0; j < len(b); j++ {
			cost := 1
			if a[i] == b[j] {
				cost = 0
			}
			curr[j+1] = min3(prev[j+1]+1, curr[j]+1, prev[j]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
