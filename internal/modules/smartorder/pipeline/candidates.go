package pipeline

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
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

// Matcher scores unresolved lines against the catalogue.
type Matcher struct {
	repo  smartorder.Repository
	index *productmatch.Index
	opts  productmatch.MatchOptions
}

// NewMatcher constructs the fuzzy stage.
//
// minScore is the buyer's أقل نسبة مطابقة, the same control and the same 50%
// default as the three import tools. It decides what the scorer will offer at
// all — below it a candidate is not shown, not reviewed and not sent to the AI
// tier. It is emphatically NOT Cutoff, which decides what is bought without
// asking, stays at 0.850 and is not the buyer's to lower: the cost of a wrong
// confident match here is the wrong medicine on a pharmacy's shelf.
func NewMatcher(repo smartorder.Repository, minScore float64) *Matcher {
	opts := productmatch.DefaultMatchOptions()
	// TrustSupplierCode stays off. A pharmacy's internal code coinciding with a
	// catalogue code is more often accident than design, and the SKU tier has
	// already had its chance at genuine codes.
	opts.MaxCandidates = 5 // the shortlist the review screen shows the buyer
	if minScore > 0 {
		opts.MinStrong = min(max(minScore, productmatch.DefaultMinReview), 1)
		opts.MinReview = min(productmatch.DefaultMinReview, opts.MinStrong)
	}
	return &Matcher{repo: repo, opts: opts}
}

// Load builds the in-memory catalogue index for this run.
func (m *Matcher) Load(ctx context.Context) error {
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
func (m *Matcher) Size() int {
	if m.index == nil {
		return 0
	}
	return m.index.Size()
}

// Index exposes the loaded catalogue to the AI stage, which needs it for two
// things the shortlist cannot supply: the English name of a candidate, and the
// deterministic strength re-check that validates an answer before it is written.
func (m *Matcher) Index() *productmatch.Index { return m.index }

// Score matches every unresolved line and returns those still needing help.
//
// Lines that clear the cutoff are marked resolved here and are then off-limits
// to AI — FR-018 forbids overwriting a confident deterministic result. What
// comes back is exactly the two populations the buyer sees as غير مطابق and
// مطلوب للمراجعة: nothing matched at all, or matched but below the cutoff.
func (m *Matcher) Score(lines []*smartorder.Line) []Review {
	if m.index == nil || m.index.Size() == 0 {
		// An empty index is not "nothing matched": it is a broken run, and
		// reporting it as unmatched lines would send the buyer hunting for
		// products that are in fact right there.
		return nil
	}

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
func (m *Matcher) Retrieve(reviews []Review) []Review {
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
func (m *Matcher) Verify(lines []*smartorder.Line) []Review {
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
