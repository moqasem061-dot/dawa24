package catalog

import (
	"context"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
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

	index, ok := s.catalogueIndex(ctx, session)
	if !ok {
		stats.Unmatched = len(residual)
		return stats
	}

	bare, corroborated := matchFloors(session.Options.MinMatchScore)

	opts := productmatch.DefaultMatchOptions()
	opts.MinStrong = corroborated
	opts.MinReview = min(productmatch.DefaultMinReview, corroborated)
	opts.MaxCandidates = 5

	// Retrieval for the AI tier is a separate, recall-tuned pass rather than a
	// reuse of the scorer's own candidates.
	//
	// By the time a row reaches the model the scorer has already decided it
	// cannot answer, and handing over the top rows of the pool that defeated it
	// asks a question the shortlist has answered wrongly. The other three
	// importers have done it this way for some time; this one had not, which is
	// why its adjudication tier could only ever re-rank.
	recall := productmatch.DefaultRecallOptions()
	recall.Limit = catalogCeilings.RecallLimit

	rows := make([]*productmatch.Row, len(residual))
	for n, i := range residual {
		rows[n] = matchRowFor(prods[i])
	}
	scored := productmatch.MatchAll(index, rows, opts, 0)
	retrieved := productmatch.RecallAll(index, rows, recall, 0)

	var forAI []pendingMatch
	for n, i := range residual {
		row, res := rows[n], scored[n]
		switch {
		case res.Matched() && acceptsUpdate(index, row, res, bare, corroborated):
			matches[i] = ExistingMatch{ProductID: res.ProductID, Reason: MatchSimilar}
			stats.Similar++
			// And it is checked. A similarity match here overwrites the
			// catalogue entry every pharmacy on the platform reads, so it is
			// the one applied match on the platform most worth a second
			// opinion — and until now it was the only one that never got one.
			id := res.ProductID
			forAI = append(forAI, pendingMatch{
				index:      i,
				candidates: withCurrent(retrieved[n], index, id),
				settled:    true,
				guess:      &id,
				score:      res.Score,
			})
		default:
			wide := retrieved[n]
			if len(wide) == 0 {
				stats.Unmatched++
				continue
			}
			forAI = append(forAI, pendingMatch{index: i, candidates: wide, score: res.Score})
		}
	}

	if len(forAI) == 0 {
		return stats
	}
	if !session.Options.UseAI {
		stats.Unmatched += len(forAI)
		return stats
	}

	unresolved := 0
	for _, p := range forAI {
		if p.settled {
			stats.Verified++
			continue
		}
		unresolved++
	}

	// The cache answers first. An administrator re-uploading the same registry
	// extract used to pay for the whole residue again, asking the model
	// questions the vendor import and the smart order had already bought
	// answers to and filed in the very same table.
	forAI = s.applyMatchMemory(ctx, index, prods, matches, forAI, &stats, session)

	if s.adjudicator == nil {
		stats.Unmatched += unresolved - stats.AI
		return stats
	}
	s.adjudicateMatches(ctx, session, prods, matches, forAI, &stats, progress)
	stats.Unmatched += unresolved - stats.AI
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

func summarizeCandidates(index *productmatch.Index,
	candidates []productmatch.MatchCandidate) []MatchAdjudicationCandidate {

	out := make([]MatchAdjudicationCandidate, 0, len(candidates))
	for _, c := range candidates {
		entry := MatchAdjudicationCandidate{
			ProductID:     c.ProductID,
			Name:          c.Name,
			Scientific:    c.Scientific,
			DosageForm:    c.DosageForm,
			Concentration: c.Concentration,
			Manufacturer:  c.Manufacturer,
		}
		// The English name comes from the index rather than from the shortlist,
		// which does not carry it — and it is what settles the transliteration
		// cases this tier exists for.
		if p, ok := index.Lookup(c.ProductID); ok {
			entry.NameEN = p.NameEN
			if entry.Name == "" {
				entry.Name = p.NameAR
			}
		}
		out = append(out, entry)
	}
	return out
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
