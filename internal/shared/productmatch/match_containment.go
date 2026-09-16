package productmatch

// Comparing a row's words against a catalogue name's, in both directions.
//
// The scorer used to ask one question: how much of the ROW's distinctive
// vocabulary does this catalogue entry carry? That is the right question for a
// terse supplier line against a fully spelled catalogue entry — "بانادول" is
// wholly contained in "بانادول 500مجم 24 قرص", and the three words it did not
// repeat are packaging.
//
// It is the wrong question on its own, and a cosmetics price list is where that
// shows. "ستار فيل غسول نسائى جيل مرطب 200 مل" against another company's
// "تيلير غسول نسائي 200 مل" answers it well: most of the row's words are there.
// The words that are there are the category and the bottle size; the word that
// is not is the brand, on both sides. Asking the second question —  how much of
// the CATALOGUE ENTRY did the row account for? — separates them, because the
// entry's own distinctive word went unclaimed.
//
// Both directions are weighted by rarity, so "غسول" cannot carry either of
// them, and the two are combined multiplicatively rather than averaged: a match
// is only as good as its weaker direction, which is the whole point.

// nameSide is one spelling of a catalogue product — its tokens, their folds,
// their weights — all derived at index time. See MasterProduct.
type nameSide struct {
	tokens  []string
	keys    []string
	weights []float64
	total   float64
}

// coverageFloor is how much of the forward score survives when the catalogue
// entry's own vocabulary went entirely unclaimed.
//
// Not zero, because the legitimate case exists and is common: a pharmacy writes
// the brand and the catalogue writes the brand plus a sub-line the pharmacy did
// not repeat. That should be a weaker match, not an impossible one — the
// modifier check and the dose are what decide whether the extra words matter.
//
// Not one either, which is what the old token-count penalty effectively was: it
// subtracted at most a quarter and counted words rather than weighing them, so
// three unmatched brand words cost the same as three unmatched units.
const coverageFloor = 0.70

// containment scores one candidate spelling against the query, in both
// directions, and reports what the agreement rested on.
//
// One pass over the candidate's tokens does all of it. The forward direction
// claims query slots through the epoch stamp, exactly as before; the reverse
// direction accumulates the candidate's own weight for every token that found a
// home, whether or not the slot was already claimed — a candidate word is
// covered by the row if the row contains that word, however many times.
func (q *query) containment(side nameSide) (ratio, distinct float64, hits int) {
	if q.totalWeight <= 0 || len(side.tokens) == 0 {
		return 0, 0, 0
	}
	q.epoch++

	var matched, distinctMatched, covered float64
	for j, t := range side.tokens {
		slot, ok := q.pos[t]
		exact := ok
		if !ok && len(q.keyPos) > 0 && j < len(side.keys) && side.keys[j] != "" {
			slot, ok = q.keyPos[side.keys[j]]
		}
		if !ok {
			continue
		}
		// The candidate's own word is accounted for either way, which is what
		// the reverse direction measures. A duplicate spelling in the candidate
		// carries weight 0 and so adds nothing twice.
		if j < len(side.weights) {
			covered += side.weights[j]
		}
		if q.stamp[slot] == q.epoch {
			continue
		}
		q.stamp[slot] = q.epoch
		// A word that had to be folded to meet its counterpart is weaker
		// evidence than one that matched letter for letter. See variants.go.
		w := q.weights[slot]
		if !exact {
			w *= variantWeight
		}
		matched += w
		if q.distinct[slot] {
			distinctMatched += w
			hits++
		}
	}

	if matched <= 0 {
		return 0, 0, 0
	}
	if q.distinctWeight > 0 {
		distinct = clamp(distinctMatched / q.distinctWeight)
	}

	ratio = matched / q.totalWeight
	if side.total > 0 {
		coverage := clamp(covered / side.total)
		ratio *= coverageFloor + (1-coverageFloor)*coverage
	}
	return clamp(ratio), distinct, hits
}
