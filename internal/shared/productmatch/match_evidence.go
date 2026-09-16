package productmatch

// What counts as evidence that two names are the same product.
//
// This file exists because of a screen full of live wrong matches, every one of
// which had the same shape: a supplier line and a catalogue entry that shared
// exactly one word, and that word was packaging. "اجارتوب نقط/باكت96" was
// offered against "نو كال 300 باكت" at 16%; "كورش فريش 5 مل قطرة/باكت" against
// "بولي فريش ادفانس"; a dozen unrelated medicines all against the same
// catalogue entry, because that entry happens to carry two very common words
// and therefore meets almost everything half way.
//
// The scorer was not wrong about the numbers. Sixteen per cent is an honest
// reading of "one word out of five agrees", and rarity weighting had already
// pushed it that low. What was missing is that a low score was still OFFERED:
// the row arrived on the review screen with a named product beside it, and a
// suggestion is read as an opinion however small the percentage printed next to
// it. Nine thousand rows of that is not a review queue, it is noise.
//
// So the question asked here is categorical rather than continuous: is there
// any word these two names share that could identify a product at all? If the
// only agreement is on words the catalogue carries by the thousand, the answer
// is no and no candidate is reported — the row is unmatched, which is the truth.

// nameEvidence is how two names agree, not merely how much.
type nameEvidence struct {
	// similarity is the best of the four channels, as before.
	similarity float64
	// distinctShare is the fraction of the row's DISTINCTIVE vocabulary the
	// candidate carries. A row whose brand agreed scores high here even when
	// the rest of the line is packaging; a row that agreed only on packaging
	// scores zero however long the shared text is.
	distinctShare float64
	// distinctHits is how many distinctive words agreed at all.
	distinctHits int
}

// Evidence floors.
//
// They are stated as constants rather than as options because they are not a
// tuning knob: they encode what may be called a match at all, and a caller free
// to lower them would be free to reproduce the failure above.
const (
	// minNameSimilarity is the floor below which a candidate is not scored.
	minNameSimilarity = 0.18
	// minBlindSimilarity is what a candidate must reach when NO distinctive
	// word agrees.
	//
	// It is high on purpose, and it is not unreachable: it is the
	// transliteration case, where the brand is spelled differently on the two
	// sides and therefore matches through trigrams or the consonant skeleton
	// rather than as a word — "ابيكوبريد" against "ابيكوبرايد". Those score in
	// the eighties. Two products sharing only a unit noun score in the teens.
	minBlindSimilarity = 0.72
	// minSingleWordSimilarity is what a candidate must reach when exactly one
	// distinctive word agrees and it accounts for little of the row.
	//
	// One shared brand word out of six is how a line extension looks
	// ("بانادول" inside "بانادول اكسترا نايت"), and also how a coincidence
	// looks. The dose, the form and the modifier check separate the first from
	// the second; this stops the residue being offered as a suggestion.
	minSingleWordSimilarity = 0.55
	// minDistinctShare is the share of the row's distinctive vocabulary below
	// which a single agreeing word is treated as thin evidence.
	minDistinctShare = 0.30
	// minFuzzyChannel is how much raw character agreement the two wordless
	// channels need before they are allowed to speak at all.
	//
	// They exist for one job: the same brand transliterated two ways, where no
	// whole word is shared and the letters still line up — "ابيكوبريد" against
	// "ابيكوبرايد" overlaps four fifths of its trigrams. Left ungated they also
	// answer a question nobody asked, because two multi-word Arabic cosmetics
	// names overlap about half their trigrams merely by being Arabic cosmetics
	// names: "ستار فيل غسول نسائى جيل مرطب 200 مل" and another company's
	// "تيلير غسول نسائي 200 مل" reached 0.45 that way, with the brand differing
	// on both sides and the word channels — correctly — saying 0.23.
	//
	// Above this line character agreement is a real signal; below it, it is the
	// alphabet.
	minFuzzyChannel = 0.55
)

// sufficient reports whether the agreement is of a kind worth offering.
func (e nameEvidence) sufficient(q *query) bool {
	if e.similarity < minNameSimilarity {
		return false
	}
	// A row with no distinctive vocabulary of its own — a line that is nothing
	// but packaging words — cannot be held to a distinctive-word test, so the
	// blind bar applies to it and only the near-identical survive.
	if q.distinctWeight <= 0 || e.distinctHits == 0 {
		return e.similarity >= minBlindSimilarity
	}
	if e.distinctHits == 1 && e.distinctShare < minDistinctShare {
		return e.similarity >= minSingleWordSimilarity
	}
	return true
}

// nameEvidenceOf compares two product names and reports both how alike they are
// and what the likeness rests on.
//
// The two containment channels report their own evidence; the trigram and
// skeleton channels can raise the similarity but never invent a distinctive
// word, because they do not work in words at all. That asymmetry is deliberate:
// it is what makes a high fuzzy score the only way past the blind bar, and a
// high fuzzy score between two unrelated brands is rare where a shared unit
// noun is not.
func (idx *Index) nameEvidenceOf(q *query, p *MasterProduct) nameEvidence {
	var ev nameEvidence
	sides := [2]nameSide{
		{p.coreAR, p.tokKeyAR, p.wAR, p.totAR},
		{p.coreEN, p.tokKeyEN, p.wEN, p.totEN},
	}
	for _, side := range sides {
		ratio, distinct, hits := q.containment(side)
		if ratio > ev.similarity {
			ev.similarity = ratio
		}
		if distinct > ev.distinctShare {
			ev.distinctShare = distinct
		}
		if hits > ev.distinctHits {
			ev.distinctHits = hits
		}
	}
	if ev.similarity >= 0.99 || len(q.tri) == 0 {
		return ev
	}

	// Trigram agreement is weaker evidence than a shared word, so it is
	// discounted before it can outrank one. Both sides are sorted sets built
	// once, so this is a walk rather than a set construction.
	for _, tri := range [][]trigram{p.triAR, p.triEN} {
		if v := jaccardSorted(q.tri, tri); v >= minFuzzyChannel && v*0.92 > ev.similarity {
			ev.similarity = v * 0.92
		}
	}

	// The cross-script channel, discounted further still. It is the only one
	// that can see an Arabic query and a Latin catalogue name as the same
	// product, and it is also the loosest: a skeleton drops every vowel, so two
	// different brands can reduce alike. Scored below trigrams so it only ever
	// decides a comparison the other two could not.
	if v := skeletonSimilarity(q.skeleton, p.skeleton); v >= minFuzzyChannel && v*0.86 > ev.similarity {
		ev.similarity = v * 0.86
	}
	return ev
}

// nameSimilarity is the score alone, for the callers that only need a number.
func (idx *Index) nameSimilarity(q *query, p *MasterProduct) float64 {
	return idx.nameEvidenceOf(q, p).similarity
}
