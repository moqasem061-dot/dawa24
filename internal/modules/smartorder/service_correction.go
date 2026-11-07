package smartorder

import (
	"context"
)

// CorrectMatch applies a buyer's correction, updates candidates and selections, and remembers it.
func (s *Service) CorrectMatch(ctx context.Context, orgID, lineID, productID int64) (*Line, error) {
	line, err := s.repo.GetLine(ctx, orgID, lineID)
	if err != nil {
		return nil, err
	}

	if productID <= 0 {
		line.MatchedProductID = nil
		line.MatchMethod = MethodNone
		line.MatchConfidence = 0.0
		line.CorrectedByUser = true
		line.Outcome = OutcomeUnmatched
		line.OutcomeReason = ""
		if err := s.repo.UpdateLines(ctx, []*Line{line}); err != nil {
			return nil, err
		}
		_ = s.repo.DeleteSelection(ctx, orgID, lineID)
		_ = s.repo.ReplaceCandidates(ctx, lineID, nil)
		s.recomputeRunStats(ctx, orgID, line.RunID)
		return line, nil
	}

	line.MatchedProductID = &productID
	line.MatchMethod = MethodManual
	line.MatchConfidence = 1.0
	line.CorrectedByUser = true
	if names, err := s.repo.ProductNames(ctx, []int64{productID}); err == nil && len(names) > 0 {
		line.MatchedProductName = names[productID]
	}

	run, err := s.repo.GetRunByID(ctx, orgID, line.RunID)
	if err != nil {
		return nil, err
	}
	// The same single-line re-evaluation a quantity edit uses; see refreshLine.
	if err := s.refreshLine(ctx, run, line, 0); err != nil {
		return nil, err
	}

	if err := s.repo.UpdateLines(ctx, []*Line{line}); err != nil {
		return nil, err
	}
	// The correction is remembered for this buyer's organization only. It is not
	// written as a global alias: a global alias would resolve the same text to
	// this product for every other pharmacy (Batch 3, S09). Promoting a
	// correction to global is a staff decision, not built yet.
	if err := s.repo.SaveLearnedMapping(ctx, orgID, line.RawName, productID); err != nil {
		s.log.WarnContext(ctx, "could not remember match correction",
			"line_id", lineID, "error", err)
	}

	s.recomputeRunStats(ctx, orgID, line.RunID)
	return line, nil
}

func (s *Service) recomputeRunStats(ctx context.Context, orgID, runID int64) {
	lines, _, err := s.repo.ListLines(ctx, runID, LineFilter{All: true})
	if err != nil {
		return
	}
	var stats Stats
	stats.TotalRows = len(lines)
	for _, l := range lines {
		switch l.Outcome {
		case OutcomeOrdered:
			stats.MatchedRows++
		case OutcomeUnmatched:
			stats.UnmatchedRows++
		case OutcomeNoSupplier:
			stats.NoSupplierRows++
		case OutcomeCoverageBlocked:
			stats.CoverageBlockedRows++
		case OutcomeInstitutionalBlocked:
			stats.InstitutionalBlockedRows++
		case OutcomeBelowMinQty:
			stats.BelowMinQtyRows++
		case OutcomeQuotaBlocked:
			stats.QuotaBlockedRows++
		default:
			if !l.Matched() {
				stats.UnmatchedRows++
			}
		}
	}
	run, err := s.repo.GetRunByID(ctx, orgID, runID)
	if err == nil && run != nil {
		run.Stats = stats
		_ = s.repo.UpdateRunStats(ctx, run)
	}
}
