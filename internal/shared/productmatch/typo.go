package productmatch

import "sort"

// Misspelt brands.
//
// Egyptian lists swap letters that sound alike or sit side by side on the
// keyboard — ت/ث, س/ص, ج/ح, ذ/ز, a long vowel dropped or doubled — and the
// consonant skeleton absorbs only some of them: ث folds to s, so "اكثينون"
// and "اكتينون" are different skeletons, share no word, and overlap in too
// few trigrams for the fuzzy channels. The correct product was never
// retrieved, so it could not be offered, let alone accepted.
//
// The typo channel covers that gap and only that gap. It considers the row's
// DISTINCTIVE words that the catalogue has never seen as written — a word the
// catalogue knows is not a misspelling — and of at least five letters, below
// which one edit is a different word. Those words retrieve the products that
// share enough of their trigrams, and a catalogue word within one or two edits
// of one of them is credited at typoWeight: less than a variant spelling,
// because an edit is weaker evidence than a shared skeleton.
//
// Nothing here can apply a match on its own. A typo link is a fuzzy brand link,
// and evidenced() requires the dose, the form or the count to confirm it.

const (
	minTypoRunes = 5
	typoWeight   = 0.75
	// typoPoolLimit bounds what one misspelt word may add to the pool.
	typoPoolLimit = 60
	// minTypoTrigramShare is the share of the word's trigrams a product must
	// carry to be retrieved for it.
	minTypoTrigramShare = 0.34
)

// typoSlots lists the weight slots of the query's unseen distinctive words.
func (idx *Index) typoSlots(q *query) []int {
	var out []int
	for t, slot := range q.pos {
		if slot >= len(q.distinct) || !q.distinct[slot] {
			continue
		}
		if len([]rune(t)) < minTypoRunes || idx.df[t] > 0 || idx.knownVariant(t) {
			continue
		}
		out = append(out, slot)
	}
	sort.Ints(out)
	return out
}

// typoSlot finds the unseen query word a catalogue word misspells, if any.
func (q *query) typoSlot(word string) (int, bool) {
	if len([]rune(word)) < minTypoRunes {
		return 0, false
	}
	for _, slot := range q.typoSlots {
		if editSimilarity(q.tokens[q.tokenAt(slot)], word) >= minEditSimilarity {
			return slot, true
		}
	}
	return 0, false
}

// tokenAt maps a weight slot back to its token's index in q.tokens.
func (q *query) tokenAt(slot int) int {
	for i, t := range q.tokens {
		if q.pos[t] == slot {
			return i
		}
	}
	return 0
}

// typoCandidates retrieves the products that share enough trigrams with one
// of the query's unseen words.
func (idx *Index) typoCandidates(q *query) []*MasterProduct {
	if len(q.typoSlots) == 0 {
		return nil
	}
	postings := idx.recall().tri
	var out []*MasterProduct
	for _, slot := range q.typoSlots {
		word := q.tokens[q.tokenAt(slot)]
		tris := sortedTrigrams([]string{word})
		if len(tris) == 0 {
			continue
		}
		need := int(float64(len(tris))*minTypoTrigramShare + 0.999)
		hits := make(map[*MasterProduct]int, 64)
		for _, t := range tris {
			for _, p := range postings[t] {
				hits[p]++
			}
		}
		type hit struct {
			p *MasterProduct
			n int
		}
		ranked := make([]hit, 0, len(hits))
		for p, n := range hits {
			if n >= need {
				ranked = append(ranked, hit{p, n})
			}
		}
		sort.Slice(ranked, func(i, j int) bool {
			if ranked[i].n != ranked[j].n {
				return ranked[i].n > ranked[j].n
			}
			return ranked[i].p.ID < ranked[j].p.ID
		})
		for i := 0; i < len(ranked) && i < typoPoolLimit; i++ {
			out = append(out, ranked[i].p)
		}
	}
	return out
}
