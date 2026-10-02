package productmatch

import "strings"

// The brand is the product.
//
// The commonest wrong match on Egyptian files is not a wrong strength — the dose
// check catches that — but a wrong BRAND that agrees on everything else:
//
//	row        ابيمول 500 مجم 20 قرص          (a paracetamol brand)
//	accepted   بانادول 500 مجم 20 قرص          (another one)
//
// Same strength, same form, same count, and two words of packaging in common.
// The scorer sees four agreeing attributes and one disagreeing word; a pharmacy
// receives a different product at a different price. When the row's brand is
// not in the catalogue at all — the supplier stocks something we do not — the
// same arithmetic picks whichever other brand of that molecule and strength is
// nearest, which is the absent-product failure in the egypt suite.
//
// So the row's lead brand word is found first, and every candidate is asked
// whether it carries that word in ANY of the spellings this engine already
// understands: the same word, a variant key (ابيكوبريد/ابيكوبرايد), a typo or
// a long vowel dropped (بنادول/بانادول), one word or two (اكوابلس/اكوا بلس), or
// the other alphabet (panadol/بانادول). A candidate that carries it is not
// touched. One that does not:
//
//   - is REFUSED when the row's brand is a word the catalogue knows from other
//     products. That word identifies something else, and this candidate is not
//     it.
//   - is heavily PENALISED when the row's brand is a word the catalogue has
//     never seen. That is either a brand we do not carry — accepting a
//     neighbour would be wrong — or a spelling so far off that no channel
//     links it, which a person should look at. Either way it is not applied
//     silently, and it is still shown.

const (
	// massBrand refuses: the row names a brand the catalogue knows, and this is
	// not it.
	massBrand = 1.00
	// massBrandUnverified sends to review: the row names a brand the catalogue
	// has never seen and this candidate does not carry it.
	massBrandUnverified = 0.55
	// minBrandRunes is the shortest word read as a brand. Shorter words in
	// Egyptian lines are abbreviations — ق, ك, ش, امب — or line letters.
	minBrandRunes = 4
	// minCrossScriptSkeleton is how alike two skeletons must be for a Latin
	// and an Arabic word to be the same brand.
	minCrossScriptSkeleton = 0.75
	// minSoundSpelling is how alike two same-script words must still be
	// spelled for a shared skeleton to link them.
	minSoundSpelling = 0.60
	// minShortSkeleton is the shortest skeleton compared by equality.
	minShortSkeleton = 2
)

// leadBrand picks the row's identifying lead word and, when the next word
// exists, the two joined — a brand typed as two words.
func (idx *Index) leadBrand(nameTokens []string) (brand, joined string) {
	for i, t := range nameTokens {
		if len([]rune(t)) < minBrandRunes || hasDigit(t) {
			continue
		}
		if idx.commonWord(t) {
			continue
		}
		// A company word is skipped — price lists append the distributor —
		// except where it LEADS the row. Egyptian house brands are their
		// company: "نيو ويل ترمريك", "بريتير كريم مرطب". Skipping it there left
		// the brand check comparing "turmeric", which a dozen brands carry.
		if idx.isCompanyWord(t) && i > 1 {
			continue
		}
		if _, isMod := variantModifiers[t]; isMod {
			continue
		}
		if i+1 < len(nameTokens) {
			joined = t + nameTokens[i+1]
		}
		return t, joined
	}
	return "", ""
}

// How firmly a candidate carries the row's brand.
const (
	// brandLinkNone: the row states no brand word, or the candidate lacks it.
	brandLinkNone = iota
	// brandLinkFuzzy: a typo, a dropped letter, trigram overlap.
	brandLinkFuzzy
	// brandLinkCross: the same consonants in the other alphabet. Needed for
	// Latin supplier lines against Arabic-only records, and the loosest link
	// there is: "charchola" and "ثيروسيل" both reduce to s-r-s-l.
	brandLinkCross
	// brandLinkSound: the same consonant skeleton — a transliteration, or the
	// other alphabet. Real evidence, and also how unrelated brands collide.
	brandLinkSound
	// brandLinkExact: the same word, or the same word written as two.
	brandLinkExact
)

// brandLinkOf reports how firmly the candidate carries the row's brand.
func brandLinkOf(q *query, p *MasterProduct) int {
	if q.brand == "" {
		return brandLinkNone
	}
	link := productCarries(p, q.brand)
	if q.brandJoined != "" {
		link = max(link, productCarries(p, q.brandJoined))
	}
	return link
}

// brandConflict reports whether a candidate fails to carry the row's brand,
// and how heavily that counts.
func (idx *Index) brandConflict(q *query, p *MasterProduct) (conflict, bool) {
	if q.brand == "" || brandLinkOf(q, p) != brandLinkNone {
		return conflict{}, false
	}
	if idx.df[q.brand] > 0 || idx.knownVariant(q.brand) {
		return conflict{"brand", massBrand}, true
	}
	return conflict{"brand_unverified", massBrandUnverified}, true
}

// knownVariant reports whether the catalogue carries the word under another
// spelling, which makes it a known brand rather than an unseen one.
func (idx *Index) knownVariant(word string) bool {
	if key := variantKeyOf(word); key != "" {
		return len(idx.keyTokens[key]) > 0
	}
	return false
}

// productCarries reports how firmly any spelling of the product's name
// carries the word, in the ways the engine treats as the same word.
func productCarries(p *MasterProduct, word string) int {
	wordKey := variantKeyOf(word)
	wordSk := Skeleton(word)
	latin := isLatinWord(word)
	best := brandLinkNone
	// The other alphabet is consulted only where the product has no name in
	// the row's own. An Arabic row is judged against the Arabic name when
	// there is one: "رستين" and the English "ursotwin" reduce to the same four
	// consonants, and the product's own Arabic name, "اورسوتوين", says plainly
	// that it is not this.
	sameScript := p.coreAR
	if latin {
		sameScript = p.coreEN
	}
	crossOK := len(sameScript) == 0
	check := func(tokens []string) {
		lead := leadIdentifying(tokens)
		for i, cw := range withCompounds(tokens) {
			if cw == word {
				best = brandLinkExact
				return
			}
			// A loose link — by sound or by a typo — is evidence of a brand
			// only against the candidate's own brand word, which leads its
			// name (or the first two words joined). Anywhere else it is two
			// unrelated words that happen to share consonants: "blanka" and
			// the "بالنخاع" at the end of a hair mask.
			if i != lead && i != len(tokens)+lead {
				continue
			}
			cross := latin != isLatinWord(cw)
			if cross && !crossOK {
				continue
			}
			if wordKey != "" && wordKey == variantKeyOf(cw) {
				switch {
				case cross:
					best = max(best, brandLinkCross)
				case editSimilarity(word, cw) >= minSoundSpelling:
					best = max(best, brandLinkSound)
				}
				// Within one alphabet a shared skeleton with little else in
				// common is two different words: "celiovany" and "solvin" both
				// fold to s-l-v-n once the vowels and the soft c are gone.
				continue
			}
			if best < brandLinkCross && cross {
				// Across alphabets s and z are one letter: a Latin s between
				// vowels is written ز in Arabic ("acnesone", "اكنيزون").
				cwSk := foldSZ(Skeleton(cw))
				wordSk := foldSZ(wordSk)
				switch {
				case len([]rune(wordSk)) >= minSkeletonRunes &&
					skeletonSimilarity(wordSk, cwSk) >= minCrossScriptSkeleton:
					best = brandLinkCross
					continue
				case len([]rune(wordSk)) >= minShortSkeleton && wordSk == cwSk:
					// "abilia" and "ابيليا" both reduce to b-l. Two consonants
					// are too few for similarity, but equality on the lead word
					// is a link — and a cross-script link still needs a strong
					// confirmation before it is applied.
					best = brandLinkCross
					continue
				}
			}
			if best < brandLinkFuzzy && wordsCorrespond(word, cw) {
				best = brandLinkFuzzy
			}
		}
	}
	check(p.coreAR)
	if best < brandLinkExact {
		check(p.coreEN)
	}
	return best
}

func isLatinWord(w string) bool {
	for _, r := range w {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

// leadIdentifying is the index of a name's lead brand word: the first word of
// at least minBrandRunes letters without a figure. withCompounds appends the
// joined pairs after the words, so the joined pair starting at the lead sits
// at len(tokens)+lead.
func leadIdentifying(tokens []string) int {
	for i, t := range tokens {
		if len([]rune(t)) >= minBrandRunes && !hasDigit(t) {
			return i
		}
	}
	return 0
}

func foldSZ(sk string) string { return strings.ReplaceAll(sk, "z", "s") }

// carriesAnyRowWord reports whether the product carries any of the row's
// identifying words in a spelling the brand guard recognises.
func (idx *Index) carriesAnyRowWord(row *Row, p *MasterProduct) bool {
	for _, t := range coreTokens(row.Name + " " + row.NameEN) {
		if len([]rune(t)) < 3 || hasDigit(t) || idx.commonWord(t) || idx.isCompanyWord(t) {
			continue
		}
		if productCarries(p, t) != brandLinkNone {
			return true
		}
	}
	return false
}
