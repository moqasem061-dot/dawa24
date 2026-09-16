package smartorder

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
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

	// Load offers for this newly matched product and evaluate candidates
	offers, err := s.repo.LoadOffers(ctx, orgID, []int64{productID})
	if err == nil && len(offers) > 0 {
		candidates := make([]Candidate, 0, len(offers))

		verdicts := make(map[int64]GateVerdict)
		if s.gate != nil {
			run, _ := s.repo.GetRunByID(ctx, orgID, line.RunID)
			branchID := int64(0)
			if run != nil {
				branchID = run.BranchID
			}
			qty := int(line.EffectiveQty)
			if qty <= 0 {
				qty = 1
			}
			gateLines := make([]GateLine, len(offers))
			for i, o := range offers {
				gateLines[i] = GateLine{
					VariantID:   o.VariantID,
					VendorOrgID: o.VendorOrgID,
					Quantity:    qty,
				}
			}
			if vd, gErr := s.gate.Check(ctx, orgID, branchID, time.Now(), gateLines); gErr == nil {
				verdicts = vd
			} else {
				s.log.WarnContext(ctx, "availability gate check in CorrectMatch returned error; falling back to offer attributes",
					"line_id", lineID, "product_id", productID, "error", gErr)
			}
		}

		for _, o := range offers {
			p := money.FromMinor(o.PriceMinor)
			discount := p.ApplyPercent(o.DiscountBps)
			net, _ := p.Sub(discount)

			var eligible bool
			var reason IneligibleReason
			if s.gate != nil && len(verdicts) > 0 {
				if v, ok := verdicts[o.VariantID]; ok {
					eligible = v.Allowed
					if !v.Allowed {
						reason = EvaluateReason(v.Reason)
					}
				} else {
					if !o.ProductActive || !o.VendorActive {
						eligible = false
						reason = ReasonInactive
					} else if o.StockQty <= 0 {
						eligible = false
						reason = ReasonStock
					} else if o.MinOrderQty > 0 && line.EffectiveQty > 0 && line.EffectiveQty < float64(o.MinOrderQty) {
						eligible = false
						reason = ReasonMinQty
					} else {
						eligible = true
					}
				}
			} else {
				if !o.ProductActive || !o.VendorActive {
					eligible = false
					reason = ReasonInactive
				} else if o.StockQty <= 0 {
					eligible = false
					reason = ReasonStock
				} else if o.MinOrderQty > 0 && line.EffectiveQty > 0 && line.EffectiveQty < float64(o.MinOrderQty) {
					eligible = false
					reason = ReasonMinQty
				} else {
					eligible = true
				}
			}
			candidates = append(candidates, Candidate{
				LineID:           lineID,
				OrganizationID:   orgID,
				VendorOrgID:      o.VendorOrgID,
				VariantID:        o.VariantID,
				BranchID:         o.BranchID,
				Price:            p,
				DiscountBps:      o.DiscountBps,
				NetUnitPrice:     net,
				Unit:             o.Unit,
				MinOrderQty:      o.MinOrderQty,
				StockQty:         o.StockQty,
				IsFollowed:       o.IsFollowed,
				Eligible:         eligible,
				IneligibleReason: reason,
			})
		}
		_ = s.repo.ReplaceCandidates(ctx, lineID, candidates)
		outcome, reason := OutcomeFor(true, line.EffectiveQty, candidates)
		line.Outcome = outcome
		line.OutcomeReason = string(reason)

		// Pick supplier if candidates written and config available
		cfg, _ := s.repo.GetConfig(ctx, line.RunID)
		if cfg == nil {
			cfg = &Config{Criteria: DefaultCriteria}
		}
		writtenCandidates, _ := s.repo.ListCandidates(ctx, orgID, lineID)
		if sel, ok := Select(cfg, line.ID, writtenCandidates); ok {
			for _, c := range writtenCandidates {
				if c.ID == sel.CandidateID {
					sel.LineNet, _ = LineNet(c.NetUnitPrice, line.EffectiveQty)
					break
				}
			}
			_ = s.repo.UpsertSelections(ctx, []*Selection{sel})
		}
	} else {
		line.Outcome = OutcomeNoSupplier
		line.OutcomeReason = string(ReasonInactive)
		_ = s.repo.DeleteSelection(ctx, orgID, lineID)
		_ = s.repo.ReplaceCandidates(ctx, lineID, nil)
	}

	if err := s.repo.UpdateLines(ctx, []*Line{line}); err != nil {
		return nil, err
	}
	if err := s.repo.SaveLearnedMapping(ctx, orgID, line.RawName, productID); err != nil {
		s.log.WarnContext(ctx, "could not remember match correction",
			"line_id", lineID, "error", err)
	}

	source := "manual"
	if line.MatchMethod == MethodAI {
		source = "ai_confirmed"
	}
	if err := s.repo.SaveAlias(ctx, productID, line.NormName, source, 1.0); err != nil {
		s.log.WarnContext(ctx, "could not record product alias",
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
