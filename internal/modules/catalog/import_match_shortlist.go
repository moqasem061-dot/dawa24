package catalog

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// Choosing which rows are worth an AI shortlist, before paying to build one.
//
// The matching phase has two expensive passes over the residue: scoring, which
// every row needs, and retrieval, which only the rows that reach the model do.
// They used to run one after the other over the whole residue, and the second
// was measured at ninety seconds per twenty thousand rows on a catalogue whose
// names are not distinctive — for shortlists that were, at most twelve hundred
// of them, ever read.
//
// The adjudication ceiling has been there the whole time. It was simply applied
// at the point of ASKING rather than at the point of paying, so a run built
// nineteen thousand shortlists, handed nineteen thousand keys to the decision
// cache, and then dropped all but the first twelve hundred.

// classifyOptions are the thresholds one classification pass runs under.
type classifyOptions struct {
	useAI        bool
	bare         float64
	corroborated float64
}

// askable is one row the model is worth asking about, and what it is being
// asked.
//
// settled is carried rather than re-derived. Whether the similarity tier
// applied a row is a decision acceptsUpdate makes once, on thresholds and a
// catalogue identity check; recomputing it from the score alone in a second
// place is how the two answers drift apart.
type askable struct {
	// at indexes into the residual row slices.
	at int
	// settled says the similarity tier already applied a match, so the question
	// is verification rather than resolution.
	settled bool
	// guess is the product it applied, valid only when settled.
	guess int64
}

// classifyScored applies the similarity tier's verdict to every residual row
// and returns the rows worth asking the model about.
//
// matches and stats are mutated: this is where a similarity match becomes a
// staged update.
func (s *Service) classifyScored(
	index *productmatch.Index,
	rows []*productmatch.Row,
	residual []int,
	scored []productmatch.MatchResult,
	matches map[int]ExistingMatch,
	stats *MatchStats,
	opts classifyOptions,
) []askable {
	// Settled rows are asked about first. A settled row is one the similarity
	// tier has just applied to the shared catalogue, and the model is being
	// asked to CHECK it — the one question here whose answer prevents a wrong
	// overwrite rather than merely adding a match. An unresolved row only
	// stands to gain one.
	var settled, unresolved []askable

	for n, i := range residual {
		if n >= len(scored) {
			stats.Unmatched++
			continue
		}
		res := scored[n]
		if res.Matched() && acceptsUpdate(index, rows[n], res, opts.bare, opts.corroborated) {
			matches[i] = ExistingMatch{ProductID: res.ProductID, Reason: MatchSimilar}
			stats.Similar++
			if opts.useAI {
				settled = append(settled, askable{at: n, settled: true, guess: res.ProductID})
			}
			continue
		}
		if !opts.useAI {
			stats.Unmatched++
			continue
		}
		unresolved = append(unresolved, askable{at: n})
	}

	return capToAdjudicationCeiling(settled, unresolved, stats)
}

// adjudicationCeiling is how many rows one run may ask the model about: every
// batch it is allowed, filled.
func adjudicationCeiling() int {
	return catalogCeilings.MaxRequestsPerRun * catalogCeilings.MaxItemsPerRequest
}

// capToAdjudicationCeiling trims the rows to what the run can afford to ask
// about, settled ones first, and accounts for what it dropped.
//
// A dropped row keeps its deterministic outcome, which is what it would have
// kept anyway: a settled row keeps its applied match, an unresolved one keeps
// "this is a new product". CeilingHit tells the review screen that a low match
// rate on a huge file is a budget rather than a bad catalogue.
//
// Every unresolved row counts as unmatched here, whether it is asked about or
// not; recordMatch subtracts the ones the model goes on to settle.
func capToAdjudicationCeiling(settled, unresolved []askable, stats *MatchStats) []askable {
	stats.Unmatched += len(unresolved)

	ceiling := adjudicationCeiling()
	if ceiling <= 0 || len(settled)+len(unresolved) <= ceiling {
		return append(settled, unresolved...)
	}

	stats.CeilingHit = true
	kept := settled
	if len(kept) > ceiling {
		return kept[:ceiling]
	}
	return append(kept, unresolved[:min(ceiling-len(kept), len(unresolved))]...)
}

// retrieveForAI builds the shortlists for the chosen rows, and only those.
func (s *Service) retrieveForAI(
	ctx context.Context,
	index *productmatch.Index,
	rows []*productmatch.Row,
	residual []int,
	scored []productmatch.MatchResult,
	wanted []askable,
	report func(int),
) ([]pendingMatch, error) {
	if len(wanted) == 0 {
		return nil, nil
	}

	subset := make([]*productmatch.Row, 0, len(wanted))
	for _, w := range wanted {
		subset = append(subset, rows[w.at])
	}

	opts := productmatch.DefaultRecallOptions()
	opts.Limit = catalogCeilings.RecallLimit

	shortlists, err := recallRows(ctx, s.log, index, subset, opts, report)
	if err != nil {
		return nil, err
	}

	out := make([]pendingMatch, 0, len(wanted))
	for k, w := range wanted {
		if k >= len(shortlists) {
			break
		}
		if w.settled {
			// The product the similarity tier chose must be on the shortlist,
			// or the model is asked to confirm an id it was never shown and the
			// answer is refused as a hallucination — so a check could only ever
			// fail.
			id := w.guess
			out = append(out, pendingMatch{
				index:      residual[w.at],
				candidates: withCurrent(shortlists[k], index, id),
				settled:    true,
				guess:      &id,
				score:      scored[w.at].Score,
			})
			continue
		}
		if len(shortlists[k]) == 0 {
			continue
		}
		out = append(out, pendingMatch{
			index:      residual[w.at],
			candidates: shortlists[k],
			score:      scored[w.at].Score,
		})
	}
	return out, nil
}
