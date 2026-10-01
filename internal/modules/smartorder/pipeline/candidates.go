package pipeline

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// The fuzzy tier.
//
// Everything the exact tiers could not settle is scored against an in-memory
// projection of the catalogue. The index is built **once per run** — roughly
// twenty thousand products and a few megabytes — and every remaining line is
// scored against it in memory. That is the whole trick: one query loads the
// catalogue, and the per-row cost after that is CPU, which parallelises and does
// not queue behind a network.
//
// The scorer is internal/shared/productmatch, the same engine the vendor import
// uses. Its strength veto matters more here than anywhere: a 500 mg row matched
// to a 1 g product is not a ranking inaccuracy, it is the wrong medicine.

// CandidateMatcher scores unresolved lines against the catalogue using Matcher.
type CandidateMatcher struct {
	repo    smartorder.Repository
	service Matcher
	index   *productmatch.Index
	opts    productmatch.MatchOptions
}

// NewCandidateMatcher constructs the candidate matching stage.
func NewCandidateMatcher(repo smartorder.Repository, minScore float64) *CandidateMatcher {
	opts := productmatch.DefaultMatchOptions()
	opts.MaxCandidates = 5
	if minScore > 0 {
		opts.MinStrong = min(max(minScore, productmatch.DefaultMinReview), 1)
		opts.MinReview = min(productmatch.DefaultMinReview, opts.MinStrong)
	}
	return &CandidateMatcher{repo: repo, opts: opts}
}

// NewMatcher constructs the fuzzy stage.
func NewMatcher(repo smartorder.Repository, minScore float64) *CandidateMatcher {
	return NewCandidateMatcher(repo, minScore)
}

// SetService wires the smartorder Matcher port.
func (m *CandidateMatcher) SetService(matcher Matcher) {
	m.service = matcher
}

// Load builds the in-memory catalogue index for this run.
func (m *CandidateMatcher) Load(ctx context.Context) error {
	products, err := m.repo.LoadMatchIndex(ctx)
	if err != nil {
		return err
	}
	masters := make([]productmatch.MasterProduct, 0, len(products))
	for _, p := range products {
		masters = append(masters, productmatch.MasterProduct{
			ID:            p.ID,
			NameAR:        p.NameAR,
			NameEN:        p.NameEN,
			SKU:           p.SKU,
			Barcode:       p.Barcode,
			Scientific:    p.Scientific,
			DosageForm:    p.DosageForm,
			Concentration: p.Concentration,
			Unit:          p.Unit,
			Manufacturer:  p.Manufacturer,
		})
	}
	m.index = productmatch.NewIndex(masters)
	return nil
}

// Size reports how many catalogue products are indexed, for telemetry and for
// the "is the catalogue empty?" check that otherwise looks like "nothing
// matched".
func (m *CandidateMatcher) Size() int {
	if m.index == nil {
		return 0
	}
	return m.index.Size()
}

// Index exposes the loaded catalogue to the AI stage, which needs it for two
// things the shortlist cannot supply: the English name of a candidate, and the
// deterministic strength re-check that validates an answer before it is written.
func (m *CandidateMatcher) Index() *productmatch.Index { return m.index }

// Score matches every unresolved line and returns those still needing help.
func (m *CandidateMatcher) Score(lines []*smartorder.Line) []Review {
	return m.ScoreWithContext(context.Background(), 0, 0, lines)
}

// ScoreWithContext matches unresolved lines using Matcher.Prepare if available,
// falling back to in-memory Index matching.
func (m *CandidateMatcher) ScoreWithContext(ctx context.Context, orgID, runID int64, lines []*smartorder.Line) []Review {
	// The rows are decomposed first — strength, form and pack move into the
	// structured fields the scorer compares separately, and therapeutic prose is
	// dropped (see query.go) — and then scored together, across every core.
	open := make([]*smartorder.Line, 0, len(lines))
	rows := make([]*productmatch.Row, 0, len(lines))
	for _, l := range lines {
		if l.Matched() && l.MatchConfidence >= Cutoff {
			continue
		}
		open = append(open, l)
		rows = append(rows, BuildRow(l))
	}
	if len(open) == 0 {
		return nil
	}

	if m.service != nil {
		reqRows := make([]productmatch.Row, len(rows))
		for i, r := range rows {
			reqRows[i] = *r
		}
		consumerRef := ""
		if runID > 0 {
			consumerRef = fmt.Sprintf("smartorder.runs:%d", runID)
		}
		prepReq := productmatch.PrepareRequest{
			Scope: productmatch.Scope{
				EffectiveOrg: orgID,
				Workflow:     productmatch.WorkflowSmartOrder,
				EntryPoint:   "smartorder_pipeline",
			},
			ConsumerRef: consumerRef,
			SourceRev:   1,
			Rows:        reqRows,
			Required:    productmatch.GranularityPack,
		}
		prepRes, err := m.service.Prepare(ctx, prepReq)
		if err == nil && len(prepRes.Decisions) == len(open) {
			var reviews []Review
			for i, l := range open {
				row := rows[i]
				dec := prepRes.Decisions[i]
				cols := matchflow.Columns(dec)
				l.MatchState = cols.MatchState
				l.MatchBasis = cols.MatchBasis
				l.MatchGranularity = cols.MatchGranularity
				l.MatchRevision = dec.Revision
				l.AIState = cols.AIState
				if decBytes, err := json.Marshal(dec); err == nil {
					l.MatchDecision = decBytes
				}
				if prepRes.RunID > 0 {
					rID := prepRes.RunID
					l.MatchRunID = &rID
				}

				cands := make([]productmatch.MatchCandidate, 0, len(dec.Candidates))
				for _, c := range dec.Candidates {
					mc := productmatch.MatchCandidate{
						ProductID: c.ProductID,
						Score:     1.0,
					}
					if m.index != nil {
						if p, ok := m.index.Lookup(c.ProductID); ok && p != nil {
							mc.Name = p.NameAR
							if mc.Name == "" {
								mc.Name = p.NameEN
							}
							mc.Scientific = p.Scientific
							mc.DosageForm = p.DosageForm
							mc.Concentration = p.Concentration
							mc.Manufacturer = p.Manufacturer
						}
					}
					cands = append(cands, mc)
				}

				ambiguous := dec.State == matchflow.StateAmbiguous
				switch {
				case (dec.State == matchflow.StateAccepted || dec.State == matchflow.StateConfirmed) && cols.MatchedProductID != nil:
					setMatch(l, *cols.MatchedProductID, smartorder.MatchMethod(cols.MatchLevel), 1.0)
				default:
					reviews = append(reviews, Review{
						Line:       l,
						Row:        row,
						Candidates: cands,
						Ambiguous:  ambiguous,
					})
				}
			}
			if prepRes.RunID > 0 {
				_ = m.service.MarkReviewable(ctx, prepRes.RunID)
			}
			return reviews
		}
	}

	if m.index == nil || m.index.Size() == 0 {
		return nil
	}

	results := productmatch.MatchAll(m.index, rows, m.opts, 0)

	var reviews []Review
	for i, l := range open {
		row, res := rows[i], results[i]

		ambiguous := res.Level == productmatch.MatchAmbiguous
		switch {
		case res.Matched() && res.Score >= Cutoff:
			setMatch(l, res.ProductID, methodFor(res.Level), res.Score)

		case res.Matched() && res.Level == productmatch.MatchStrong:
			// Below the cutoff but plausible: record it so the buyer sees the
			// system's best guess, and carry it into the AI stage anyway.
			setMatch(l, res.ProductID, smartorder.MethodFuzzy, res.Score)
			reviews = append(reviews, Review{
				Line: l, Row: row, Candidates: res.Candidates, Ambiguous: ambiguous})

		default:
			reviews = append(reviews, Review{
				Line: l, Row: row, Candidates: res.Candidates, Ambiguous: ambiguous})
		}
	}
	return reviews
}

// Retrieve widens each review's shortlist for the AI stage.
//
// Match's candidates are the by-product of a decision that has already failed:
// they come from the pool that defeated the scorer, ranked by the scoring that
// could not separate them, and when the scorer found nothing at all there are
// none. Handing those to a model asks it to re-rank a list that does not contain
// the answer.
//
// Recall runs a different retrieval — token, trigram and molecule strategies
// unioned, with the disqualifying penalties removed — and replaces the
// shortlist with a wider one. The deterministic candidates are kept at the head
// of it, because when the scorer *did* find something plausible its ranking is
// still the best evidence available about what to look at first.
//
// Called only when the AI stage will actually run, so an import that resolves
// deterministically never pays for retrieval nothing reads.
// It also decides which lines are worth asking about at all, and that is the
// single largest saving available in this pipeline. A line whose best retrieved
// candidate is implausible has no answer for the model to choose: the product is
// not in the catalogue. Sending it anyway is what run 43 did — 8,790 rows of
// cosmetics against a pharmaceutical catalogue, 7,939 lines sent, 156 improved,
// thirty requests, the ceiling hit, and 345 seconds of wall clock against 1.6
// seconds of deterministic work. i18n.TDefault("w4_mod.s_364_364") was already the right answer for
// almost all of them, and it was the answer they got, five minutes later.
//
// Returned rather than filtered in place, so the caller keeps the full review
// set for the screen and sends only the answerable subset.
func (m *CandidateMatcher) Retrieve(reviews []Review) []Review {
	if m.index == nil {
		return nil
	}
	opts := productmatch.DefaultRecallOptions()
	opts.Limit = ceilings.RecallLimit

	rows := make([]*productmatch.Row, len(reviews))
	for i := range reviews {
		rows[i] = reviews[i].Row
	}
	wide := productmatch.RecallAll(m.index, rows, opts, 0)

	askable := make([]Review, 0, len(reviews))
	for i := range reviews {
		reviews[i].Candidates = mergeCandidates(reviews[i].Candidates, wide[i], ceilings.RecallLimit)
		if plausible(reviews[i].Candidates, ceilings.MinPlausible) {
			askable = append(askable, reviews[i])
		}
	}
	return askable
}

// Verify prepares the rows the deterministic engine SETTLED, so the model can
// check them.
//
// This is the population nothing used to look at. The stage saw only what the
// engine could not resolve, which means the rows it resolved WRONGLY — the ones
// where a name similarity of 0.99 hid a different pack size, and where the
// buyer has no signal at all that anything went wrong — were the only rows in
// the file never examined twice. Measured against labelled data the engine is
// right about 99.3% of the time on those, and 0.7% of a twenty-five-thousand-row
// price list is a hundred and seventy wrongly priced medicines.
//
// Only name-based tiers are offered. A barcode is the same physical package and
// the buyer's own confirmed mapping is their own assertion; a model cannot
// improve on either, and asking spends the budget the ambiguous rows need.
//
// Retrieval runs here as it does for the unresolved rows, because a check is
// only useful if the alternatives are on the table: "is this the 20-tablet pack
// or the 200?" cannot be answered from the 20-tablet pack alone.
func (m *CandidateMatcher) Verify(lines []*smartorder.Line) []Review {
	if m.index == nil || m.index.Size() == 0 {
		return nil
	}
	opts := productmatch.DefaultRecallOptions()
	opts.Limit = ceilings.RecallLimit

	settled := make([]*smartorder.Line, 0, len(lines))
	rows := make([]*productmatch.Row, 0, len(lines))
	for _, l := range lines {
		if !l.Matched() || l.MatchConfidence < Cutoff || !verifiable(l) {
			continue
		}
		settled = append(settled, l)
		rows = append(rows, BuildRow(l))
	}
	found := productmatch.RecallAll(m.index, rows, opts, 0)

	out := make([]Review, 0, len(settled))
	for i, l := range settled {
		out = append(out, Review{
			Line:       l,
			Row:        rows[i],
			Candidates: found[i],
			Settled:    true,
		})
	}
	return out
}

// plausible reports whether retrieval found anything worth a second opinion.
//
// The best candidate decides it. A shortlist whose strongest member is a
// coincidence of one shared word is not a shortlist, and offering it to a model
// asks a question whose honest answer is the one already recorded.
func plausible(candidates []productmatch.MatchCandidate, floor float64) bool {
	for _, c := range candidates {
		if c.Score >= floor {
			return true
		}
	}
	return false
}

// mergeCandidates keeps the deterministic shortlist first, then fills from the
// wider retrieval, de-duplicated, up to the limit.
func mergeCandidates(primary, wide []productmatch.MatchCandidate, limit int) []productmatch.MatchCandidate {
	out := make([]productmatch.MatchCandidate, 0, limit)
	seen := make(map[int64]bool, limit)

	for _, sets := range [][]productmatch.MatchCandidate{primary, wide} {
		for _, c := range sets {
			if len(out) >= limit {
				return out
			}
			if c.ProductID <= 0 || seen[c.ProductID] {
				continue
			}
			seen[c.ProductID] = true
			out = append(out, c)
		}
	}
	return out
}

// methodFor maps the matcher's level onto the method the buyer sees.
func methodFor(level productmatch.MatchLevel) smartorder.MatchMethod {
	switch level {
	case productmatch.MatchBarcode:
		return smartorder.MethodBarcode
	case productmatch.MatchCode:
		return smartorder.MethodSKU
	case productmatch.MatchExact:
		return smartorder.MethodExactName
	case productmatch.MatchStrong:
		return smartorder.MethodIdentityKey
	default:
		return smartorder.MethodFuzzy
	}
}
