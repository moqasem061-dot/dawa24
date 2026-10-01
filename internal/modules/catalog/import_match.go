package catalog

import (
	"context"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// Matching an imported row to the product it is meant to update.
//
// The exact tiers — item code, barcode, and the Arabic name folded by
// platform.normalize_arabic — settle a re-upload of the very same file and
// almost nothing else. Measured on the live catalogue they resolved under a
// tenth of a real supplier price list, because a supplier does not write
// i18n.TDefault("w4_mod.0_5_265") the way the catalogue records it: they write
// i18n.TDefault("w4_mod.5_266"), or they put the strength in its own column, or they
// append the pack count. Every one of those is a different string and the same
// medicine.
//
// So the residue goes through the same engine the vendor import and the smart
// order use: an in-memory index of the catalogue, scored on the identifying
// words with the dose and the pharmaceutical form able to veto. One query
// builds it, and the per-row cost after that is CPU.
//
// What is deliberately NOT done here is applying a weak match silently. Every
// similarity match is staged under its own reason so the review screen shows it
// as a judgement rather than as a fact, and the admin commits it knowingly.

// pendingMatch is a row carried to the AI tier with its shortlist already
// retrieved, so adjudication costs no further catalogue work.
type pendingMatch struct {
	index      int
	candidates []productmatch.MatchCandidate
	// settled says the similarity tier already applied a match to this row, so
	// the model is asked to check it rather than to make one.
	settled bool
	guess   *int64
	score   float64
}

// resolveSimilarMatches ties the rows the exact tiers missed to the catalogue
// products they are most likely meant to update.
//
// matches is mutated in place. Rows already matched by an identifier are never
// reconsidered: an exact code beats any similarity score, always.
func (s *Service) resolveSimilarMatches(
	ctx context.Context,
	session *ImportSession,
	prods []*Product,
	matches map[int]ExistingMatch,
	progress ProgressFunc,
) MatchStats {
	stats := MatchStats{Exact: len(matches)}

	residual := make([]int, 0, len(prods))
	for i, p := range prods {
		if p == nil {
			continue
		}
		if _, done := matches[i]; done {
			continue
		}
		residual = append(residual, i)
	}
	if len(residual) == 0 || s.imports == nil {
		stats.Unmatched = len(residual)
		return stats
	}

	// Still the catalogue phase: this is a second read of the same table, and
	// it reports no count either. The scoring below is what the matching phase
	// covers, and that one can count.
	progress.report(ImportPhaseCatalogue, 0, 0)
	index, ok := s.catalogueIndex(ctx, session)
	if !ok {
		stats.Unmatched = len(residual)
		return stats
	}
	// The AI tier is decided once, here, and every later branch reads this
	// rather than the option. It used to be read in four places, and two of
	// them were reached only because a slice happened to be empty when the
	// switch was off — a guard by coincidence, which is no guard at all.
	//
	// The ports are checked alongside the switch because retrieval exists to
	// feed them: the recall pass builds shortlists for the adjudicator and the
	// decision cache, and on a deployment with neither it is several seconds of
	// work per twenty thousand rows for an answer nothing will read. It is
	// deliberately NOT gated on the mapper — that port answers a different
	// question, which columns and taxonomy words mean what, and a deployment
	// can have one without the other.
	useAI := session.Options.UseAI && (s.adjudicator != nil || s.matchMemory != nil)

	progress.report(ImportPhaseMatching, 0, len(prods))

	bare, corroborated := matchFloors(session.Options.MinMatchScore)

	opts := productmatch.DefaultMatchOptions()
	opts.MinStrong = corroborated
	opts.MinReview = min(productmatch.DefaultMinReview, corroborated)
	opts.MaxCandidates = 5

	rows := make([]*productmatch.Row, len(residual))
	for n, i := range residual {
		rows[n] = matchRowFor(prods[i])
	}

	// Both passes are chunked so the phase can report, be cancelled and be
	// timed. See import_match_chunks.go: a step that cannot say where it has
	// reached is indistinguishable from one that has died, and this is the step
	// every "stuck at N%" report has named.
	passes := 1
	if useAI {
		passes = 2
	}
	settled := len(matches)

	scored, err := scoreRows(ctx, s.log, s.matchingSvc, index, rows, opts,
		matchingProgress(progress, settled, len(residual), len(prods), 0, passes))
	if err != nil {
		stats.Unmatched = len(residual)
		return stats
	}

	// Classification happens on the SCORES ALONE, before any retrieval.
	//
	// That ordering is the whole saving. Retrieval used to run over every
	// residual row so the loop below could read retrieved[n] — a second pass as
	// expensive as the scoring one, measured at ninety seconds per twenty
	// thousand rows on a catalogue whose names are not distinctive — and then
	// at most twelve hundred of those shortlists were ever asked about. The
	// rest were built, held in memory, and thrown away.
	//
	// So the rows are classified first, the ones worth asking about are chosen,
	// and only those are retrieved for.
	wanted := s.classifyScored(index, rows, residual, scored, matches, &stats,
		classifyOptions{useAI: useAI, bare: bare, corroborated: corroborated})

	forAI, err := s.retrieveForAI(ctx, index, rows, residual, scored, wanted,
		matchingProgress(progress, settled, len(residual), len(prods), 1, passes))
	if err != nil {
		return stats
	}

	// No report here. The chunked passes above have already walked the count to
	// the end of the residue, and len(matches) is a smaller number — reporting
	// it would rewind the bar, which reads as a failure even on a healthy run.
	if ctx.Err() != nil {
		return stats
	}

	if len(forAI) == 0 {
		return stats
	}
	if !useAI {
		stats.Unmatched += len(forAI)
		return stats
	}

	for _, p := range forAI {
		if p.settled {
			stats.Verified++
		}
	}

	// The cache answers first. An administrator re-uploading the same registry
	// extract used to pay for the whole residue again, asking the model
	// questions the vendor import and the smart order had already bought
	// answers to and filed in the very same table.
	forAI = s.applyMatchMemory(ctx, index, prods, matches, forAI, &stats, session)

	// capToAdjudicationCeiling has already counted every unresolved row as
	// unmatched, asked or not; only the ones the AI (fresh or remembered)
	// resolved come off. Adding the asked rows again here counted them twice.
	if s.adjudicator == nil {
		stats.Unmatched -= stats.AI
		if stats.Unmatched < 0 {
			stats.Unmatched = 0
		}
		return stats
	}
	s.adjudicateMatches(ctx, session, prods, matches, forAI, &stats, progress)
	stats.Unmatched -= stats.AI
	if stats.Unmatched < 0 {
		stats.Unmatched = 0
	}
	return stats
}

// catalogueIndex builds the in-memory catalogue the residue is scored against.
func (s *Service) catalogueIndex(ctx context.Context, session *ImportSession) (*productmatch.Index, bool) {
	catalogue, err := s.imports.ListMatchProducts(ctx)
	if err != nil {
		s.log.WarnContext(ctx, "catalogue projection unavailable; similarity matching skipped",
			"session", session.PublicID, "error", err)
		return nil, false
	}
	if len(catalogue) == 0 {
		return nil, false
	}
	masters := make([]productmatch.MasterProduct, 0, len(catalogue))
	for _, c := range catalogue {
		masters = append(masters, productmatch.MasterProduct{
			ID: c.ID, NameAR: c.NameAR, NameEN: c.NameEN, SKU: c.SKU,
			Barcode: c.Barcode, Scientific: c.Scientific, DosageForm: c.DosageForm,
			Concentration: c.Concentration, Unit: c.Unit,
			Manufacturer: c.Manufacturer, PublicPrice: c.PublicPrice,
		})
	}
	return productmatch.NewIndex(masters), true
}

// matchRowFor projects an imported product onto the row shape the shared
// matcher scores.
func matchRowFor(p *Product) *productmatch.Row {
	return &productmatch.Row{
		Name:          p.Name.Get(i18n.AR),
		NameEN:        p.Name.Get(i18n.EN),
		Scientific:    p.ScientificName,
		SKU:           p.SKU,
		Barcode:       p.Barcode,
		Manufacturer:  p.ManufacturingCompanies,
		DosageForm:    p.DosageForm,
		Concentration: p.Concentration,
		Unit:          p.Unit,
	}
}

// adjudicationText is what the model is shown for the incoming row: everything
// that identifies it, and nothing that does not.
func adjudicationText(p *Product) string {
	parts := make([]string, 0, 5)
	for _, v := range []string{
		p.Name.Get(i18n.AR), p.Name.Get(i18n.EN),
		p.Concentration, p.DosageForm, p.ManufacturingCompanies,
	} {
		if v = strings.TrimSpace(v); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " | ")
}

func inShortlist(candidates []productmatch.MatchCandidate, productID int64) bool {
	for _, c := range candidates {
		if c.ProductID == productID {
			return true
		}
	}
	return false
}

// withCurrent puts a settled row's own product on its shortlist if retrieval
// left it out.
//
// Without it the model is asked to confirm an id it was never shown, and the
// answer is then refused as a hallucination — so a check could only ever fail.
func withCurrent(candidates []productmatch.MatchCandidate, index *productmatch.Index,
	id int64) []productmatch.MatchCandidate {
	if inShortlist(candidates, id) {
		return candidates
	}
	p, ok := index.Lookup(id)
	if !ok {
		return candidates
	}
	return append(candidates, productmatch.MatchCandidate{
		ProductID:     p.ID,
		Name:          p.NameAR,
		Scientific:    p.Scientific,
		DosageForm:    p.DosageForm,
		Concentration: p.Concentration,
		Manufacturer:  p.Manufacturer,
	})
}
