package catalog

import (
	"strings"
	"unicode"
)

// Text and number normalisation for spreadsheet import.
//
// Real supplier files are typed by humans across a decade of Windows machines.
// The same manufacturer arrives as i18n.TDefault("w4_mod.s_267_267"), i18n.TDefault("w4_mod.s_268_268") (with tatweel), i18n.TDefault("w4_mod.s_269_269")
// (with harakat) and i18n.TDefault("w4_mod.s_270_270"); the same header arrives as i18n.TDefault("w4_ui.s_54_54"), i18n.TDefault("w4_mod.s_271_271"),
// i18n.TDefault("w4_mod.s_252_252") and "Price ". Comparing those byte-for-byte is why the previous
// importer needed a hand-written contains() chain per field and still missed
// most files. Normalising once, here, lets every comparison downstream be an
// ordinary string equality.

// arabicLetterFolds maps the letter variants that Egyptian data entry treats as
// interchangeable onto a single canonical form. This is the same folding the
// database applies in platform.normalize_arabic, kept in sync deliberately so a
// name matched in Go matches in SQL too.
var arabicLetterFolds = map[rune]rune{
	'\u0623': '\u0627', // أ -> ا
	'\u0625': '\u0627', // إ -> ا
	'\u0622': '\u0627', // آ -> ا
	'\u0671': '\u0627', // ٱ -> ا
	'\u0649': '\u064A', // ى -> ي
	'\u0626': '\u064A', // ئ -> ي
	'\u0624': '\u0648', // ؤ -> و
	'\u0629': '\u0647', // ة -> ه
	'\u06A4': '\u0641', // ڤ -> ف
	'\u0686': '\u062C', // چ -> ج
	'\u06AF': '\u062C', // گ -> ج
	'\u067E': '\u0628', // پ -> ب
}

// isArabicMark reports whether r is a diacritic or tatweel that carries no
// lexical meaning for matching purposes.
func isArabicMark(r rune) bool {
	switch {
	case r >= '\u064B' && r <= '\u0652': // harakat
		return true
	case r == '\u0640': // tatweel ـ
		return true
	case r == '\u0670': // superscript alef
		return true
	case r >= '\u06D6' && r <= '\u06ED': // quranic marks
		return true
	case r == '\u200B', r == '\u200C', r == '\u200D', r == '\u200E', r == '\u200F':
		return true // zero-width and bidi controls
	case r == '\uFEFF':
		return true // BOM that survived as a character
	}
	return false
}

// foldDigit converts Arabic-Indic (٠-٩) and Extended Arabic-Indic (۰-۹) digits
// to ASCII. Excel writes these verbatim when the sheet was typed on an Arabic
// keyboard, and every numeric parse in Go rejects them.
func foldDigit(r rune) (rune, bool) {
	switch {
	case r >= '\u0660' && r <= '\u0669':
		return '0' + (r - '\u0660'), true
	case r >= '\u06F0' && r <= '\u06F9':
		return '0' + (r - '\u06F0'), true
	}
	return r, false
}

// NormalizeDigits converts any Arabic-Indic digits in s to ASCII and leaves the
// rest of the string untouched. Used on values that must stay human-readable,
// such as a concentration printed on the box.
func NormalizeDigits(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { _, ok := foldDigit(r); return ok }) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if d, ok := foldDigit(r); ok {
			b.WriteRune(d)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// CleanCellString trims a raw spreadsheet cell into a single-line display value.
//
// It preserves the original letters — this is what gets stored and shown to a
// pharmacist — and only removes the invisible damage: non-breaking spaces,
// embedded newlines from wrapped cells, bidi control characters, and runs of
// whitespace left by column padding.
func CleanCellString(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := false
	for _, r := range s {
		switch {
		case r == '\u00a0', r == '\u202f', r == '\t', r == '\r', r == '\n', r == '\v', r == '\f':
			r = ' '
		case r == '\u200B', r == '\u200C', r == '\u200D', r == '\u200E', r == '\u200F', r == '\uFEFF':
			continue // invisible; drop entirely rather than turning into a space
		case unicode.IsSpace(r):
			r = ' '
		}
		if r == ' ' {
			if lastSpace {
				continue
			}
			lastSpace = true
		} else {
			lastSpace = false
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// NormalizeKey reduces a header label or a matching key to its comparable core:
// folded Arabic letters, ASCII digits, lowercase Latin, and nothing else. All
// spaces and punctuation are removed, so "Item No.", "item_no" and "ITEM  NO"
// collapse to the same key.
//
// Removing spaces is what makes the synonym table tractable: a supplier writing
// i18n.TDefault("w4_mod.s_253_253"), i18n.TDefault("w4_mod.s_272_272") or i18n.TDefault("w4_mod.s_273_273") produces one key, not three.
func NormalizeKey(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isArabicMark(r) {
			continue
		}
		if d, ok := foldDigit(r); ok {
			b.WriteRune(d)
			continue
		}
		if f, ok := arabicLetterFolds[r]; ok {
			r = f
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		case r >= '\u0621' && r <= '\u064A':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// NormalizeName folds a product name for duplicate detection: same folding as
// NormalizeKey but single spaces are kept, so two genuinely different products
// whose names differ only by a word boundary stay distinct.
func NormalizeName(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := true // leading spaces are dropped
	for _, r := range s {
		if isArabicMark(r) {
			continue
		}
		if d, ok := foldDigit(r); ok {
			b.WriteRune(d)
			lastSpace = false
			continue
		}
		if f, ok := arabicLetterFolds[r]; ok {
			r = f
		}
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
			lastSpace = false
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r >= '\u0621' && r <= '\u064A':
			b.WriteRune(r)
			lastSpace = false
		default:
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}
