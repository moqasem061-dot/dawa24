package smartorder

import (
	"context"
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// SetQuantity applies a buyer's quantity edit.
//
// Stock, the supplier's minimum order and the branch's quota are all judged at
// a quantity, so an edit re-asks the purchase rule for the line rather than
// only comparing against the chosen supplier's stock. A line raised from zero
// gets a supplier again; one dropped to zero loses it. The buyer's own supplier
// choice is kept while that supplier can still serve the new quantity.
func (s *Service) SetQuantity(ctx context.Context, orgID, lineID int64, qty float64) error {
	if qty < 0 {
		return apperr.Validation("smartorder.negative_quantity",
			"quantity cannot be negative", nil)
	}
	line, err := s.repo.GetLine(ctx, orgID, lineID)
	if err != nil {
		return err
	}
	run, err := s.repo.GetRunByID(ctx, orgID, line.RunID)
	if err != nil {
		return err
	}
	var kept int64
	if sel, err := s.repo.GetSelection(ctx, orgID, lineID); err == nil && sel != nil {
		if cand, err := s.repo.GetCandidate(ctx, orgID, sel.CandidateID); err == nil && cand != nil {
			if cand.StockQty > 0 && qty > float64(cand.StockQty) {
				return apperr.Validation("smartorder.insufficient_stock",
					fmt.Sprintf("الكمية المطلوبة (%g) تتجاوز المخزون المتاح لدى المورد (%d)", qty, cand.StockQty), nil)
			}
			if sel.UserOverridden {
				kept = cand.VariantID
			}
		}
	}
	if err := s.repo.UpdateLineQuantity(ctx, orgID, lineID, qty); err != nil {
		return err
	}
	line.EffectiveQty = qty
	if err := s.refreshLine(ctx, run, line, kept); err != nil {
		return err
	}
	s.recomputeRunStats(ctx, orgID, run.ID)
	return nil
}

// SetDefaultQuantity applies a default quantity to all (or selected) lines in a
// run, then re-evaluates the run: every line it touched has a new quantity to
// be judged at.
func (s *Service) SetDefaultQuantity(ctx context.Context, orgID, runID int64, lineIDs []int64, qty float64) error {
	if qty <= 0 {
		return apperr.Validation("smartorder.invalid_quantity",
			"quantity must be greater than zero", nil)
	}
	if err := s.repo.SetDefaultQuantity(ctx, orgID, runID, lineIDs, qty); err != nil {
		return err
	}
	run, err := s.repo.GetRunByID(ctx, orgID, runID)
	if err != nil {
		return err
	}
	return s.Refresh(ctx, run)
}
