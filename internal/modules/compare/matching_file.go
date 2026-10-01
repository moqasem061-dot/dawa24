package compare

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// MatchFileRowsToCatalog resolves a batch of extracted rows against the catalog using the matching engine.
// It populates MatchedProductID, MatchConfidence, and MatchMethod on each row, and persists them via BulkUpdateFileRowMatches
// if row IDs are already assigned.
func (s *Service) MatchFileRowsToCatalog(ctx context.Context, fileID int64, rows []*CompareFileRow) error {
	if len(rows) == 0 {
		return nil
	}

	if s.matcher != nil {
		var effOrg int64
		for _, r := range rows {
			if r.OrganizationID != nil && *r.OrganizationID > 0 {
				effOrg = *r.OrganizationID
				break
			}
		}

		matchRows := make([]productmatch.Row, len(rows))
		for i, r := range rows {
			matchRows[i] = productmatch.Row{
				Number: r.RowNumber,
				Name:   strings.TrimSpace(r.RawName),
				SKU:    strings.TrimSpace(r.SKU),
			}
		}

		res, err := s.matcher.Prepare(ctx, productmatch.PrepareRequest{
			Scope: productmatch.Scope{
				EffectiveOrg: effOrg,
				Workflow:     productmatch.WorkflowCompare,
				EntryPoint:   "compare_match_file_rows",
			},
			ConsumerRef: fmt.Sprintf("compare:file:%d", fileID),
			SourceRev:   1,
			Rows:        matchRows,
			Required:    productmatch.GranularityProduct,
		})
		if err != nil {
			return fmt.Errorf("compare matching prepare: %w", err)
		}

		for i, dec := range res.Decisions {
			if i >= len(rows) {
				break
			}
			cols := matchflow.Columns(dec)
			rows[i].MatchState = cols.MatchState
			rows[i].MatchBasis = cols.MatchBasis
			rows[i].MatchGranularity = cols.MatchGranularity
			rows[i].MatchRevision = dec.Revision
			rows[i].AIState = cols.AIState
			rows[i].MatchedProductID = cols.MatchedProductID

			if cols.MatchedProductID != nil {
				confidence := 100.0
				method, _ := compareMethodFor(productmatch.MatchLevel(cols.MatchLevel), confidence)
				rows[i].MatchConfidence = confidence
				rows[i].MatchMethod = method
			}
		}

		var matches []RowMatch
		for _, r := range rows {
			if r.ID > 0 && r.MatchedProductID != nil {
				matches = append(matches, RowMatch{
					RowID:      r.ID,
					ProductID:  r.MatchedProductID,
					Confidence: r.MatchConfidence,
					Method:     r.MatchMethod,
				})
			}
		}

		if len(matches) > 0 {
			return s.repo.BulkUpdateFileRowMatches(ctx, fileID, matches)
		}
		return nil
	}

	// Fallback when matcher is not wired (e.g. standalone tests)
	candidates, err := s.repo.ListCatalogMatchProducts(ctx, 1000)
	if err != nil || len(candidates) == 0 {
		return err
	}

	masters := make([]productmatch.MasterProduct, 0, len(candidates))
	for _, c := range candidates {
		if c == nil || c.ID <= 0 {
			continue
		}
		masters = append(masters, productmatch.MasterProduct{
			ID:            c.ID,
			NameAR:        c.NameAr,
			NameEN:        c.NameEn,
			SKU:           c.SKU,
			Barcode:       c.Barcode,
			Scientific:    c.ScientificName,
			Manufacturer:  c.ManufacturingCompanies,
			DosageForm:    c.DosageForm,
			Concentration: c.Concentration,
			Unit:          c.Unit,
		})
	}
	if len(masters) == 0 {
		return nil
	}

	idx := productmatch.NewIndex(masters)
	matchRows := make([]*productmatch.Row, len(rows))
	for i, r := range rows {
		cleanSKU := strings.TrimSpace(r.SKU)
		matchRows[i] = &productmatch.Row{
			Name: strings.TrimSpace(r.RawName),
			SKU:  cleanSKU,
		}
	}

	opts := productmatch.DefaultMatchOptions()
	results := productmatch.MatchAll(idx, matchRows, opts, 0)

	for i, res := range results {
		if res.Matched() && res.Level.Settled() {
			confidence := math.Round(res.Score * 100.0)
			method, _ := compareMethodFor(res.Level, confidence)
			prodID := res.ProductID
			rows[i].MatchedProductID = &prodID
			rows[i].MatchConfidence = confidence
			rows[i].MatchMethod = method
		}
	}

	if s.enhancer != nil {
		s.refineWithAI(ctx, idx, rows, matchRows, results)
	}

	var matches []RowMatch
	for _, r := range rows {
		if r.ID > 0 && r.MatchedProductID != nil {
			matches = append(matches, RowMatch{
				RowID:      r.ID,
				ProductID:  r.MatchedProductID,
				Confidence: r.MatchConfidence,
				Method:     r.MatchMethod,
			})
		}
	}

	if len(matches) > 0 {
		return s.repo.BulkUpdateFileRowMatches(ctx, fileID, matches)
	}
	return nil
}
