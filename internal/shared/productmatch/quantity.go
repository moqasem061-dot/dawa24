package productmatch

// The figures and the letters that separate one product from its neighbours.
//
// A brand family is written as one name plus one distinguishing mark, and the
// mark is almost never a word. It is a figure — 20 tablets against 200, a 25 g
// tube against 50 — or it is a single letter: بتنوفيت ان and بتنوفيت سي are
// Betnovate N and Betnovate C, two different medicines, and the whole
// difference between them is one letter that coreTokens discards for being too
// short to be a word.
//
// Both were invisible to the scorer. Figures reached it as an unordered bag in
// which a pack count and a bottle size and half a combination dose were the
// same kind of thing, compared by asking whether the shorter bag was contained
// in the longer one — which is true of "16" inside "16 و 12.5", so اتاكاند 16
// and اتاكاند دي 16/12.5 agreed on their numbers and differed on nothing else
// the engine could see. Letters reached it not at all.
//
// This file reads a name once and says what each figure was counting.

import (
	"sort"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// countUnit is a number and the thing it counts: 30 tablets, 5 suppositories,
// 10 sachets.
//
// The class is the canonical dosage-form key rather than the word, so "30 قرص",
// "30 اقراص" and "30 tabs" are one measurement written three ways.
type countUnit struct {
	class string
	value float64
}

// quantities is what one name's figures turned out to be.
type quantities struct {
	// counts are the figures that named what they counted.
	counts []countUnit
	// residual are the figures that named nothing — "20*10", "1+1", a wipe
	// count whose noun this engine does not know. They are compared only
	// against another name's residual figures, because that is the only
	// comparison they support.
	residual []float64
}

// readQuantities walks a name once and classifies every figure in it.
//
// A figure is a count when the word beside it names something countable, and a
// residual otherwise. It is never a count when the word beside it is a dose
// unit: "500 مجم" is a strength and is compared as one, by parseStrength, on a
// scale where 1 g and 1000 mg are the same number.
//
// Nothing is guessed. The old signature bagged every figure in the name
// together and compared the bags by containment, which made a 30-tablet pack
// and half a combination dose interchangeable evidence.
func readQuantities(text string) quantities {
	words := strings.Fields(sheet.NormalizeName(foldPackMultipliers(sheet.NormalizeDigits(text))))
	if len(words) == 0 {
		return quantities{}
	}

	var q quantities
	addCount := func(value float64, class string) {
		for i := range q.counts {
			if q.counts[i].class == class && q.counts[i].value == value {
				return
			}
		}
		q.counts = append(q.counts, countUnit{class: class, value: value})
	}
	addResidual := func(value float64) {
		for _, v := range q.residual {
			if v == value {
				return
			}
		}
		q.residual = append(q.residual, value)
	}

	place := func(value float64, next string) {
		if value <= 0 || value > 100000 {
			return
		}
		switch class := countClassOf(next); class {
		case classDose:
			// A strength. Compared elsewhere, on its own scale.
		case "":
			addResidual(value)
		default:
			addCount(value, class)
		}
	}

	for i, w := range words {
		// "30قرص" — the figure and the word glued, which is how most Egyptian
		// price lists are typed.
		if head, tail, ok := splitLeadingNumber(w); ok {
			place(head, tail)
			continue
		}
		if !isNumericLiteral(w) {
			continue
		}
		v, err := strconv.ParseFloat(strings.Trim(w, "."), 64)
		if err != nil {
			continue
		}
		place(v, lookahead(words, i))
	}

	sort.SliceStable(q.counts, func(i, j int) bool {
		if q.counts[i].class != q.counts[j].class {
			return q.counts[i].class < q.counts[j].class
		}
		return q.counts[i].value < q.counts[j].value
	})
	sort.Float64s(q.residual)
	return q
}

// lookahead finds the word that says what a bare figure was counting.
//
// Usually the very next one. Where the next token is itself a figure, the two
// are halves of a ratio — "10/20mg", "20/10 مجم", "45 - 60 ml" — and both
// halves are governed by the unit at the end of it. Reading only the immediate
// neighbour left the leading half unexplained, so a supplier writing 10/20 and
// a catalogue writing 20/10 were recorded as carrying different stray figures
// and contradicted each other over a combination they agree about.
//
// Bounded at four, which is longer than any real ratio and short enough that a
// name listing figures for unrelated reasons cannot chain them together.
func lookahead(words []string, i int) string {
	for n := 0; n < 4 && i+1 < len(words); n++ {
		i++
		next := words[i]
		if !hasDigit(next) {
			// An abbreviation between the figure and the noun it counts.
			// "14 f.c. tabs" is fourteen tablets; stopping at the "f" recorded
			// fourteen of nothing, so the English half of every catalogue
			// record stated no pack count at all and could not contradict one.
			if _, filler := abbreviationLetters[next]; filler {
				continue
			}
			if _, named := letterNames[next]; named {
				continue
			}
			return next
		}
		if _, tail, ok := splitLeadingNumber(next); ok && tail != "" {
			return tail
		}
	}
	return ""
}

// classDose marks a figure the strength comparison already owns, and classStrip
// the strips a pack is counted in.
const (
	classDose  = "\x00dose"
	classStrip = "\x00strip"
)

// stripWords are the ways a blister strip is written. They name a unit of
// packaging whose tablet count this engine does not know.
var stripWords = map[string]bool{
	"شريط": true, "شرائط": true, "اشرطه": true, "شرايط": true,
	"strip": true, "strips": true, "blister": true, "بليستر": true,
}

// countClassOf resolves the word after a figure onto the thing it counts.
//
// It reads the same form vocabulary the rest of the engine uses, so a count of
// "اكياس" and a count of "قرص" are recognised as counting different things —
// which is what makes "دوليبران 1جم 8 اكياس" and "دوليبران 1جم 8 اقراص"
// distinguishable at all. A form word with no canonical group keeps its own
// spelling as its class: this engine does not group wipes, and two products
// differing only in whether the packet holds 25 of them or 50 are still two
// products.
func countClassOf(word string) string {
	if word == "" || hasDigit(word) {
		return ""
	}
	if isDoseUnitWord(word) {
		return classDose
	}
	// A strip is not a tablet, as a COUNT.
	//
	// The FORM key maps شريط onto tablets and should: a strip is what tablets
	// come in, and a row reading "ادويفلام 50 مجم 2 شريط" is asking for a solid
	// oral product. But the strip is also the thing being COUNTED, and counting
	// it under the tablet class made "3 شريط" contradict the catalogue's
	// "30 قرص" — the same pack, described once by its strips and once by its
	// tablets, refused on a count check with the printed price agreeing to the
	// piastre on both sides.
	//
	// Given its own class it cannot contradict a tablet count, which is the
	// honest answer: this engine does not know how many tablets a strip holds.
	// Where two names both count strips they still compare.
	if stripWords[word] {
		return classStrip
	}
	if key := formKeyOf(word); key != "" {
		return key
	}
	if _, ok := formWords[word]; ok {
		return "form:" + word
	}
	if _, ok := unitWords[word]; ok {
		// A sales unit rather than a pharmaceutical form: "12 علبة". Grouped
		// under one class so counts of packaging compare with each other and
		// not with counts of tablets.
		return "pack"
	}
	return ""
}

// isDoseUnitWord reports whether a unit word states a strength rather than a
// quantity of packaging.
func isDoseUnitWord(word string) bool {
	_, ok := doseUnits[word]
	return ok
}

// foldPackMultipliers turns a written multiplication into the number it means.
//
// Egyptian price lists state a carton as its factors: "20*10 tabs" is twenty
// strips of ten and the catalogue records it as "200 قرص". Read literally the
// row states a pack of twenty AND a pack of ten, neither of which is true, and
// the catalogue's 200 contradicts both — so اليرجيل 4 مجم 20 قرص was applied to
// a row asking for اليرجيل 4مجم 200 قرص, with the pack-count check actively
// voting for the wrong one.
//
// Only the product is recorded. The factors are deliberately dropped rather
// than kept alongside it: a row stating 10, 20 and 200 agrees with every
// sibling in the family, which is the same as having no pack count at all.
//
// The separator may be *, × or x, and x is the delicate one — it is also a
// letter. It counts only between two bare figures, so "b12 x 30" and "vitax 10"
// are left alone: neither has a pure number on the left of the x.
func foldPackMultipliers(text string) string {
	if !strings.ContainsAny(text, "*×xX") {
		return text
	}
	runes := []rune(text)
	var b strings.Builder
	b.Grow(len(text))

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r != '*' && r != '×' && r != 'x' && r != 'X' {
			b.WriteRune(r)
			continue
		}
		left, leftStart := trailingNumber(runes[:i])
		right, rightEnd := leadingNumber(runes[i+1:])
		product := left * right
		if left <= 0 || right <= 0 || product > maxPackUnits {
			b.WriteRune(r)
			continue
		}
		// Drop the left factor already written, and skip the right one.
		out := []rune(b.String())
		b.Reset()
		b.WriteString(string(out[:leftStart]))
		b.WriteString(strconv.Itoa(product))
		i += rightEnd
	}
	return b.String()
}

// maxPackUnits bounds what a multiplication may be believed to produce. Above
// it the two figures were not a pack: they are a lot number, a barcode split
// across a separator, or a size in millimetres.
const maxPackUnits = 100000

// trailingNumber reads the figure a multiplication sign was applied to, and
// where in the buffer it began. It refuses a figure that is part of a word
// ("vitax"), which is what keeps the letter x from being read as an operator.
func trailingNumber(before []rune) (value, start int) {
	end := len(before)
	for end > 0 && before[end-1] == ' ' {
		end--
	}
	start = end
	for start > 0 && before[start-1] >= '0' && before[start-1] <= '9' {
		start--
	}
	if start == end {
		return 0, 0
	}
	if start > 0 {
		if prev := before[start-1]; isNameLetter(prev) {
			return 0, 0
		}
	}
	v, err := strconv.Atoi(string(before[start:end]))
	if err != nil {
		return 0, 0
	}
	return v, start
}

// leadingNumber reads the figure after a multiplication sign, and how many
// runes it consumed including any space before it.
func leadingNumber(after []rune) (value, width int) {
	i := 0
	for i < len(after) && after[i] == ' ' {
		i++
	}
	start := i
	for i < len(after) && after[i] >= '0' && after[i] <= '9' {
		i++
	}
	if i == start {
		return 0, 0
	}
	v, err := strconv.Atoi(string(after[start:i]))
	if err != nil {
		return 0, 0
	}
	return v, i
}

// isNameLetter reports whether a rune would make an adjacent figure part of a
// word rather than a standalone count.
func isNameLetter(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		return true
	case r >= 'ء' && r <= 'ي':
		return true
	}
	return false
}

// splitLeadingNumber separates "30قرص" into 30 and "قرص".
func splitLeadingNumber(w string) (float64, string, bool) {
	cut := 0
	for cut < len(w) && (w[cut] >= '0' && w[cut] <= '9' || w[cut] == '.') {
		cut++
	}
	if cut == 0 || cut == len(w) {
		return 0, "", false
	}
	v, err := strconv.ParseFloat(strings.Trim(w[:cut], "."), 64)
	if err != nil || v <= 0 {
		return 0, "", false
	}
	return v, w[cut:], true
}
