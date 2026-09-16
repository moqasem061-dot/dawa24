package productmatch

import (
	"sort"
	"strings"
	"sync"
)

// Retrieval for the AI enhancement stage.
//
// Match() is tuned for precision. It has to be: whatever it settles is applied
// without asking, so a candidate that is merely plausible must be refused. That
// same tuning is what makes its shortlist useless to a second opinion — by the
// time a row reaches AI, the scorer has *already* decided it cannot answer, and
// handing the model the top five products from the pool that defeated it asks a
// question the shortlist has already answered wrongly.
//
// On the live catalogue that failure is not hypothetical. "اتاكاند 32مجم بلس
// 14قرص" is in the catalogue as "اتاكاند بلس 32/25 ملجم 14 قرص"; the scorer
// splits the strength differently, drops below its floor and produces a
// shortlist the buyer never sees. Recall() exists to put that product back on
// the table.
//
// Three retrieval strategies are unioned, because each one fails differently:
//
//   - The **token pool** finds rows that share a distinctive word. It fails when
//     the pharmacy transliterated the brand differently ("ابليفاى" against the
//     catalogue's "ابيليفاي") — no whole word is shared, so no posting list is
//     read and the pool comes back empty.
//   - The **trigram pool** covers exactly that case: the two spellings share
//     "ليف" and "يفا" even though they share no word.
//   - The **molecule pool** covers the case where the pharmacy wrote the generic
//     name and the catalogue carries only brands.
//
// Nothing here decides anything. Recall ranks and truncates; the decision is the
// model's, bounded by the fact that it may only choose from what this returned.

// RecallOptions tune retrieval breadth.
type RecallOptions struct {
	// Limit is how many candidates one row may contribute. Above roughly a
	// dozen the extra rows are noise the model has to read and pay for; below
	// half that, the correct product starts falling off the end.
	Limit int
	// PoolLimit caps how many products are scored per strategy.
	PoolLimit int
	// MinScore drops candidates too weak to be worth a token. Deliberately far
	// below Match's floor: this is retrieval, not judgement.
	MinScore float64
}

// DefaultRecallOptions are the measured defaults.
func DefaultRecallOptions() RecallOptions {
	return RecallOptions{Limit: 12, PoolLimit: 600, MinScore: 0.08}
}

// recallIndex holds the two posting lists only Recall needs, built on first use.
//
// They are lazy because most runs never reach the AI stage: an import that
// resolves deterministically should not pay to build indexes nothing reads.
// Building them costs one pass over the catalogue and a few megabytes, which is
// worth paying once for the runs that do.
//
// The scientific list is kept separate from Index.tokens rather than folded into
// it, because that one is built from product *names* only. Folding molecules in
// would change what every existing name-similarity score means; keeping them
// apart lets retrieval consult them without disturbing the scorer.
type recallIndex struct {
	once sync.Once
	tri  map[trigram][]*MasterProduct
	sci  map[string][]*MasterProduct
	// skel maps a consonant-skeleton trigram to the products carrying it. It is
	// what makes a cross-script query retrievable at all: an Arabic row shares
	// no token and no within-script trigram with a Latin catalogue name, so
	// without this the correct product is not in the pool to be scored.
	skel map[trigram][]*MasterProduct
}

func (idx *Index) recall() *recallIndex {
	idx.tri.once.Do(func() {
		tri := make(map[trigram][]*MasterProduct, idx.total*8)
		sci := make(map[string][]*MasterProduct, idx.total)
		skel := make(map[trigram][]*MasterProduct, idx.total*6)
		for _, p := range idx.products {
			seen := make(map[trigram]bool, len(p.triAR)+len(p.triEN))
			for _, set := range [][]trigram{p.triAR, p.triEN} {
				for _, t := range set {
					if seen[t] {
						continue
					}
					seen[t] = true
					tri[t] = append(tri[t], p)
				}
			}
			for _, tok := range coreTokens(p.Scientific) {
				sci[tok] = append(sci[tok], p)
			}
			for _, t := range sortedStringTrigrams(p.skeleton) {
				skel[t] = append(skel[t], p)
			}
		}
		idx.tri.tri, idx.tri.sci, idx.tri.skel = tri, sci, skel
	})
	return &idx.tri
}

// Recall returns the catalogue products most likely to be this row, widest net
// first. It never applies a match and never refuses to answer: an empty result
// means the catalogue genuinely holds nothing related, which is information the
// caller needs rather than an error.
func (idx *Index) Recall(row *Row, opts RecallOptions) []MatchCandidate {
	if idx == nil || idx.total == 0 || row == nil {
		return nil
	}
	if opts.Limit <= 0 {
		opts.Limit = 12
	}
	if opts.PoolLimit <= 0 {
		opts.PoolLimit = 600
	}

	q := idx.newQuery(row)
	pool := make(map[int64]*MasterProduct, opts.PoolLimit*2)

	// Strategy 1 — shared distinctive words.
	for _, p := range idx.candidatePool(q.tokens, opts.PoolLimit) {
		pool[p.ID] = p
	}
	// The exact folded name, which the token pool can miss when every word in
	// it is common.
	if q.nameKey != "" {
		for _, p := range idx.byName[q.nameKey] {
			pool[p.ID] = p
		}
	}

	// Strategy 2 — shared character trigrams, for the transliteration cases
	// where no whole word survives on both sides.
	idx.addTrigramPool(pool, q.tri, opts.PoolLimit)

	// Strategy 3 — the molecule, for the rows where the pharmacy wrote the
	// generic name and the catalogue carries only brands.
	if row.Scientific != "" {
		sci := idx.recall().sci
		for _, tok := range coreTokens(row.Scientific) {
			for _, p := range sci[tok] {
				pool[p.ID] = p
			}
			if len(pool) >= opts.PoolLimit {
				break
			}
		}
	}

	// Strategy 4 — the consonant skeleton, for the rows written in the other
	// alphabet. It runs last and only when the pool is thin, because it is the
	// loosest net here: it drops every vowel, so it retrieves generously and
	// relies on the scorer to sort out what it brought back.
	if len(pool) < opts.PoolLimit/2 {
		if sk := skeletonOf(coreTokens(row.Name + " " + row.NameEN)); sk != "" {
			skel := idx.recall().skel
			for _, t := range sortedStringTrigrams(sk) {
				for _, p := range skel[t] {
					pool[p.ID] = p
				}
				if len(pool) >= opts.PoolLimit {
					break
				}
			}
		}
	}

	if len(pool) == 0 {
		return nil
	}

	scored := make([]scoredProduct, 0, len(pool))
	for _, p := range pool {
		sp := idx.rateForRecall(q, p)
		if sp.score >= opts.MinScore {
			scored = append(scored, sp)
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].product.ID < scored[j].product.ID
	})
	return describe(scored, opts.Limit)
}

// addTrigramPool reads the rarest trigrams first and stops once there is plenty
// to score.
//
// Ordering by rarity matters more here than in the token pool: a trigram like
// "الا" appears in a third of the catalogue and reading its posting list first
// would fill the pool with products chosen by an article rather than a brand.
func (idx *Index) addTrigramPool(pool map[int64]*MasterProduct, tri []trigram, limit int) {
	if len(tri) == 0 {
		return
	}
	postings := idx.recall().tri

	ordered := make([]trigram, 0, len(tri))
	for _, t := range tri {
		if len(postings[t]) > 0 {
			ordered = append(ordered, t)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return len(postings[ordered[i]]) < len(postings[ordered[j]])
	})

	crowded := idx.total / 8
	if crowded < 128 {
		crowded = 128
	}
	for _, t := range ordered {
		list := postings[t]
		if len(list) > crowded && len(pool) > 0 {
			break // every remaining trigram is at least this common
		}
		if takeInto(pool, list, limit) {
			return
		}
	}
}

// rateForRecall scores a candidate for retrieval rather than for application.
//
// The difference from rate() is deliberate and is the whole point of this file:
// a disagreeing strength scores *down* but is not disqualifying. The row that
// says 32 mg and the catalogue entry that says 32/25 mg disagree numerically and
// are the same product; refusing to retrieve it is how the scorer lost it in the
// first place. Deciding that question is the model's job, and the applier
// re-checks the answer before anything is written.
func (idx *Index) rateForRecall(q *query, p *MasterProduct) scoredProduct {
	name := idx.nameSimilarity(q, p)
	score := name

	// Per unit and over every dose each side states, the same comparison the
	// scorer makes. Comparing first-against-first demoted correct candidates
	// for measuring different things — a tube size against a concentration —
	// and retrieval is the one place that costs an answer outright: a candidate
	// pushed below the recall floor is never offered to the model at all.
	if agree, comparable := compareStrengths(q.strengths, p.strengths); comparable {
		if agree {
			score += 0.12
		} else {
			score -= 0.10
		}
	}
	if q.formKey != "" && p.formKey != "" {
		if q.formKey == p.formKey {
			score += 0.06
		} else {
			score -= 0.05
		}
	}
	if q.makerKey != "" && p.makerKey != "" &&
		(q.makerKey == p.makerKey || strings.Contains(p.makerKey, q.makerKey)) {
		score += 0.06
	}
	if q.sciKey != "" && p.sciKey != "" && q.sciKey == p.sciKey {
		score += 0.08
	}
	if q.packSize > 0 && p.packSize > 0 && q.packSize == p.packSize {
		score += 0.04
	}

	return scoredProduct{
		product: p,
		score:   clamp(score),
		name:    name,
	}
}
