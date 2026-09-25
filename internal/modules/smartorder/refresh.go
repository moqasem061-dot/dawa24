package smartorder

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/timeutil"
)

// Keeping a saved smart order true to the world as it is now.
//
// A run decides which suppliers can serve each line once, when it is
// processed, and stores the answer. Everything that answer depends on keeps
// moving afterwards: the buyer edits quantities (stock, minimum order and quota
// are all per quantity), a supplier changes its coverage or runs out, a price
// changes, the delivery day turns over at midnight. Read back hours later the
// stored answer showed a supplier the catalogue had already stopped showing,
// and the buyer only found out when placing the order was refused.
//
// Refresh re-asks the one purchase rule (commerce.CheckAvailability, through
// the gate) for the whole run in a fixed number of statements: one offers read,
// one availability batch, one candidate replace, one selection upsert, one line
// update. The buyer's own supplier choices survive whenever the supplier is
// still orderable; lines the buyer removed stay removed.

// refreshInterval is how long a run's verdicts are trusted on screen before
// opening it again re-checks them. Placing an order always re-checks.
const refreshInterval = 2 * time.Minute

// refreshTracker remembers when each run was last refreshed in this process.
// A restart simply means the next open refreshes, which is the safe direction.
type refreshTracker struct {
	mu   sync.Mutex
	last map[int64]time.Time
}

func (t *refreshTracker) due(runID int64, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.last == nil {
		t.last = make(map[int64]time.Time)
	}
	at, ok := t.last[runID]
	// A new Cairo day changes the delivery schedule that applies.
	return !ok || now.Sub(at) >= refreshInterval ||
		timeutil.InCairo(at).YearDay() != timeutil.InCairo(now).YearDay()
}

func (t *refreshTracker) mark(runID int64, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.last == nil {
		t.last = make(map[int64]time.Time)
	}
	for id, at := range t.last {
		if now.Sub(at) > time.Hour {
			delete(t.last, id)
		}
	}
	t.last[runID] = now
}

// refreshable reports whether a run's results can still change.
func refreshable(run *Run) bool {
	return run != nil && (run.Status == StatusCompleted || run.Status == StatusStale)
}

// RefreshIfDue refreshes a run whose verdicts are older than refreshInterval
// or were taken on another Cairo day. It is what a results or review page
// calls on open; a failure is logged and the stored results are shown.
//
// It returns the run as stored after the refresh (counts and total included),
// or the run it was given when nothing was due or the refresh failed.
func (s *Service) RefreshIfDue(ctx context.Context, run *Run) *Run {
	if !refreshable(run) || s.gate == nil || !s.refreshes.due(run.ID, time.Now()) {
		return run
	}
	if err := s.Refresh(ctx, run); err != nil {
		s.log.WarnContext(ctx, "smart order refresh failed; showing stored results",
			"run_id", run.ID, "error", err)
		return run
	}
	if fresh, err := s.repo.GetRunByID(ctx, run.OrganizationID, run.ID); err == nil && fresh != nil {
		return fresh
	}
	return run
}

// Refresh re-evaluates every line of a run against the purchase rule now.
func (s *Service) Refresh(ctx context.Context, run *Run) error {
	if !refreshable(run) {
		return nil
	}
	if s.gate == nil {
		return fmt.Errorf("smart order refresh: no availability gate")
	}
	lines, _, err := s.repo.ListLines(ctx, run.ID, LineFilter{All: true})
	if err != nil {
		return err
	}
	cfg, err := s.repo.GetConfig(ctx, run.ID)
	if err != nil || cfg == nil {
		cfg = &Config{Criteria: DefaultCriteria}
	}

	// The buyer's own choices, by variant, so they can be restored onto the
	// rebuilt candidates. Replacing candidates cascades to their selections.
	kept := s.userChoices(ctx, run)

	var productIDs []int64
	seenProduct := make(map[int64]bool)
	for _, l := range lines {
		if l.Matched() && l.Outcome != OutcomeRemoved && !seenProduct[*l.MatchedProductID] {
			seenProduct[*l.MatchedProductID] = true
			productIDs = append(productIDs, *l.MatchedProductID)
		}
	}
	offers, err := s.repo.LoadOffers(ctx, run.OrganizationID, productIDs)
	if err != nil {
		return err
	}
	byProduct := make(map[int64][]Offer, len(productIDs))
	for _, o := range offers {
		byProduct[o.ProductID] = append(byProduct[o.ProductID], o)
	}

	verdicts, err := s.checkOffers(ctx, run, lines, byProduct)
	if err != nil {
		return err
	}

	byLine := make(map[int64][]Candidate, len(lines))
	changed := make([]*Line, 0, len(lines))
	for _, l := range lines {
		if l.Outcome == OutcomeRemoved || !l.Matched() {
			continue
		}
		if l.EffectiveQty <= 0 {
			l.Outcome, l.OutcomeReason = OutcomeZeroQty, ""
		} else {
			cands := candidatesFor(run.OrganizationID, l.ID, byProduct[*l.MatchedProductID], verdicts)
			byLine[l.ID] = cands
			outcome, reason := OutcomeFor(true, l.EffectiveQty, cands)
			l.Outcome, l.OutcomeReason = outcome, string(reason)
		}
		changed = append(changed, l)
	}

	stored, err := s.repo.ReplaceRunCandidates(ctx, run.ID, byLine)
	if err != nil {
		return err
	}
	selections := make([]*Selection, 0, len(changed))
	for _, l := range changed {
		if l.Outcome != OutcomeOrdered {
			continue
		}
		if sel := s.selectFor(cfg, l, stored[l.ID], kept[l.ID]); sel != nil {
			selections = append(selections, sel)
		} else {
			l.Outcome = OutcomeNoSupplier
		}
	}
	if err := s.repo.UpsertSelections(ctx, selections); err != nil {
		return err
	}
	if err := s.repo.UpdateLines(ctx, changed); err != nil {
		return err
	}
	s.recomputeRunStats(ctx, run.OrganizationID, run.ID)
	if _, err := s.Recalculate(ctx, run, cfg); err != nil {
		return err
	}
	s.refreshes.mark(run.ID, time.Now())
	return nil
}

// userChoices returns the variant the buyer picked by hand, per line.
func (s *Service) userChoices(ctx context.Context, run *Run) map[int64]int64 {
	out := map[int64]int64{}
	sels, err := s.repo.ListSelectionsByRun(ctx, run.OrganizationID, run.ID)
	if err != nil {
		return out
	}
	for lineID, sel := range sels {
		if sel == nil || !sel.UserOverridden {
			continue
		}
		if c, err := s.repo.GetCandidate(ctx, run.OrganizationID, sel.CandidateID); err == nil && c != nil {
			out[lineID] = c.VariantID
		}
	}
	return out
}

// checkOffers asks the gate about every offer of every live line, once.
//
// The gate answers per variant. A variant on two lines is asked about at the
// larger of their quantities, which is the stricter question.
func (s *Service) checkOffers(ctx context.Context, run *Run, lines []*Line,
	byProduct map[int64][]Offer) (map[int64]GateVerdict, error) {

	qty := make(map[int64]int)
	vendor := make(map[int64]int64)
	for _, l := range lines {
		if l.Outcome == OutcomeRemoved || !l.Matched() || l.EffectiveQty <= 0 {
			continue
		}
		q := int(l.EffectiveQty)
		if q <= 0 {
			q = 1
		}
		for _, o := range byProduct[*l.MatchedProductID] {
			vendor[o.VariantID] = o.VendorOrgID
			qty[o.VariantID] = max(qty[o.VariantID], q)
		}
	}
	if len(qty) == 0 {
		return map[int64]GateVerdict{}, nil
	}
	gateLines := make([]GateLine, 0, len(qty))
	for v, q := range qty {
		gateLines = append(gateLines, GateLine{VariantID: v, VendorOrgID: vendor[v], Quantity: q})
	}
	return s.gate.Check(ctx, run.OrganizationID, run.BranchID, timeutil.Now(), gateLines)
}

// selectFor keeps the buyer's own supplier when it is still orderable, and
// otherwise chooses by the run's criteria.
func (s *Service) selectFor(cfg *Config, l *Line, written []Candidate, keptVariant int64) *Selection {
	var sel *Selection
	var chosen *Candidate
	if keptVariant > 0 {
		for i := range written {
			if written[i].VariantID == keptVariant && written[i].Eligible {
				chosen = &written[i]
				sel = &Selection{LineID: l.ID, OrganizationID: written[i].OrganizationID,
					CandidateID: written[i].ID, DecidedBy: DecidedUser, UserOverridden: true}
				break
			}
		}
	}
	if sel == nil {
		auto, ok := Select(cfg, l.ID, written)
		if !ok {
			return nil
		}
		sel = auto
		for i := range written {
			if written[i].ID == sel.CandidateID {
				chosen = &written[i]
				break
			}
		}
	}
	if chosen != nil {
		if net, err := LineNet(chosen.NetUnitPrice, l.EffectiveQty); err == nil {
			sel.LineNet = net
		}
	}
	return sel
}

// candidatesFor turns a line's offers into candidates using only the gate's
// verdicts. An offer without a verdict is not orderable: every orderability
// question — coverage above all — is the gate's to answer.
func candidatesFor(orgID, lineID int64, offers []Offer, verdicts map[int64]GateVerdict) []Candidate {
	out := make([]Candidate, 0, len(offers))
	for _, o := range offers {
		p := money.FromMinor(o.PriceMinor)
		net, _ := p.Sub(p.ApplyPercent(o.DiscountBps))
		eligible, reason := false, ReasonInactive
		if v, ok := verdicts[o.VariantID]; ok {
			eligible, reason = v.Allowed, ""
			if !v.Allowed {
				reason = EvaluateReason(v.Reason)
			}
		}
		out = append(out, Candidate{
			LineID: lineID, OrganizationID: orgID,
			VendorOrgID: o.VendorOrgID, VariantID: o.VariantID, BranchID: o.BranchID,
			Price: p, DiscountBps: o.DiscountBps, NetUnitPrice: net, Unit: o.Unit,
			MinOrderQty: o.MinOrderQty, StockQty: o.StockQty, IsFollowed: o.IsFollowed,
			Eligible: eligible, IneligibleReason: reason,
		})
	}
	return out
}

// refreshLine re-evaluates one line: its offers, the gate's verdict at its
// quantity, its outcome and its supplier. keptVariant, when set, is the
// buyer's own choice to keep if it is still orderable. It writes the line.
//
// Match correction and a quantity edit both land here, so a line can never be
// re-evaluated by a rule the rest of the run does not use.
func (s *Service) refreshLine(ctx context.Context, run *Run, line *Line, keptVariant int64) error {
	orgID := run.OrganizationID
	var cands []Candidate
	switch {
	case line.Outcome == OutcomeRemoved:
		return nil
	case !line.Matched():
		line.Outcome, line.OutcomeReason = OutcomeUnmatched, ""
	case line.EffectiveQty <= 0:
		line.Outcome, line.OutcomeReason = OutcomeZeroQty, ""
	default:
		offers, err := s.repo.LoadOffers(ctx, orgID, []int64{*line.MatchedProductID})
		if err != nil {
			return err
		}
		verdicts := map[int64]GateVerdict{}
		if s.gate == nil {
			s.log.ErrorContext(ctx, "smart order service has no availability gate; line has no orderable supplier",
				"line_id", line.ID)
		} else if len(offers) > 0 {
			byProduct := map[int64][]Offer{*line.MatchedProductID: offers}
			if vd, err := s.checkOffers(ctx, run, []*Line{line}, byProduct); err == nil {
				verdicts = vd
			} else {
				s.log.WarnContext(ctx, "availability gate check failed; line left unorderable",
					"line_id", line.ID, "error", err)
			}
		}
		cands = candidatesFor(orgID, line.ID, offers, verdicts)
		outcome, reason := OutcomeFor(true, line.EffectiveQty, cands)
		line.Outcome, line.OutcomeReason = outcome, string(reason)
	}

	if err := s.repo.ReplaceCandidates(ctx, line.ID, cands); err != nil {
		return err
	}
	var sel *Selection
	if line.Outcome == OutcomeOrdered {
		written, err := s.repo.ListCandidates(ctx, orgID, line.ID)
		if err != nil {
			return err
		}
		cfg, err := s.repo.GetConfig(ctx, run.ID)
		if err != nil || cfg == nil {
			cfg = &Config{Criteria: DefaultCriteria}
		}
		if sel = s.selectFor(cfg, line, written, keptVariant); sel == nil {
			line.Outcome = OutcomeNoSupplier
		}
	}
	if sel != nil {
		if err := s.repo.UpsertSelections(ctx, []*Selection{sel}); err != nil {
			return err
		}
	} else if err := s.repo.DeleteSelection(ctx, orgID, line.ID); err != nil {
		return err
	}
	return s.repo.UpdateLines(ctx, []*Line{line})
}
