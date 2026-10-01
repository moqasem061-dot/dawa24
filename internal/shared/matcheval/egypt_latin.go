package matcheval

import "strings"

// LatinShorthand rewrites an English catalogue name the way Egyptian
// distributors type English price lists: abbreviated forms and units, figures
// glued to units, one typo in the brand, and commercial noise.
func LatinShorthand(name string, seed int) string {
	if seed < 0 {
		seed = -seed
	}
	words := strings.Fields(strings.ToLower(name))
	for i, w := range words {
		if r, ok := latinForms[strings.Trim(w, ".,")]; ok {
			words[i] = r[seed%len(r)]
		}
	}
	for i, w := range words {
		if len(w) >= 5 && !hasDigitRune(w) {
			words[i] = latinTypo(w, seed)
			break
		}
	}
	s := strings.Join(words, " ")
	if (seed/5)%2 == 0 {
		s = glueFigures(s)
	}
	if n := latinNoise[(seed/11)%len(latinNoise)]; n != "" {
		s += " " + n
	}
	if seed%3 == 0 {
		s = strings.ToUpper(s)
	}
	return s
}

var latinForms = map[string][]string{
	"tablets": {"tab", "tabs", "tablets"}, "tabs": {"tab", "tabs"}, "tab": {"tab", "tabs"},
	"capsules": {"cap", "caps", "capsules"}, "caps": {"cap", "caps"}, "capsule": {"cap", "caps"},
	"syrup": {"syr", "syrup"}, "suspension": {"susp", "susp."}, "susp.": {"susp", "susp."},
	"ampoules": {"amp", "amps"}, "amps": {"amp", "amps"}, "amp.": {"amp", "amps"},
	"cream": {"cream", "crm"}, "ointment": {"oint", "oint."}, "oint.": {"oint", "oint."},
	"injection": {"inj", "inj."}, "sachets": {"sach", "sachets", "sachet"},
}

var latinNoise = []string{"", "new price", "n.p", "offer", "", "(imported)", "", "**", "old price", ""}

// latinTypo applies one realistic slip to an English brand: a doubled or
// dropped letter, or a vowel swapped (the transliteration varies).
func latinTypo(w string, seed int) string {
	r := []rune(w)
	switch (seed / 13) % 4 {
	case 0:
		i := len(r) / 2
		return string(r[:i]) + string(r[i+1:])
	case 1:
		i := len(r) / 2
		return string(r[:i+1]) + string(r[i:])
	case 2:
		for i, c := range r {
			if i > 0 && strings.ContainsRune("aeiou", c) {
				swap := map[rune]rune{'a': 'e', 'e': 'i', 'i': 'e', 'o': 'u', 'u': 'o'}[c]
				r[i] = swap
				return string(r)
			}
		}
	}
	return w
}
