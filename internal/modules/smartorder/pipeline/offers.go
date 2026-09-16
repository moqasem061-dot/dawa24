package pipeline

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Supplier resolution.
//
// One query loads every vendor variant of every matched product in the file.
// Coverage and Corporate Operations are then applied per candidate in memory,
// because both are decisions about a (vendor, buyer-branch, moment) triple that
// no single SQL predicate expresses cleanly — and because evaluating them in Go
// keeps the rules in the services that already own them.

// CoverageGate answers whether a vendor can deliver to a point right now.
//
// An interface rather than a direct dependency on the workflow module: modules
// must not import each other, and it also makes the "window closed" case
// trivially testable without a database.
type CoverageGate interface {
	Serves(ctx context.Context, vendorOrgID int64, day time.Weekday, lat, lng float64) (bool, int, error)
}

// InstitutionalGate answers whether this buyer's branch may buy this offer.
//
// It must be the SAME rule commerce.CheckAvailability applies, because that is
// what runs at checkout: anything this gate lets through and checkout refuses
// is a line the buyer is told about only at the last click, after reviewing an
// order built on it. The composition roots wire it to
// org.Service.BranchesInstitutionallyConnected for exactly that reason.
type InstitutionalGate interface {
	Visible(ctx context.Context, c smartorder.InstitutionalCheck) (bool, error)
}

// BranchLocation is where the order is going.
type BranchLocation struct {
	BranchID int64
	Lat      float64
	Lng      float64
	HasCoord bool
}

// Supplier resolves and selects vendors for matched lines.
type Supplier struct {
	repo          smartorder.Repository
	coverage      CoverageGate
	institutional InstitutionalGate
	gate          smartorder.AvailabilityGate
	cfg           *smartorder.Config
	branch        BranchLocation
	now           func() time.Time
}

// NewSupplier constructs the supplier stage.
func NewSupplier(repo smartorder.Repository, cov CoverageGate, inst InstitutionalGate,
	cfg *smartorder.Config, branch BranchLocation) *Supplier {
	return &Supplier{
		repo: repo, coverage: cov, institutional: inst,
		cfg: cfg, branch: branch, now: time.Now,
	}
}

// SetAvailabilityGate wires the unified purchase availability gate.
func (s *Supplier) SetAvailabilityGate(gate smartorder.AvailabilityGate) {
	s.gate = gate
}

// Resolve loads offers for every matched line, evaluates eligibility, selects a
// supplier, and returns the running order total.
//
// Three passes, and the split is the performance design. Building candidates is
// pure CPU once the offers are loaded, so it happens for the whole file first;
// then one statement writes them all and hands back their ids; then selection
// runs in memory over what came back. The previous shape interleaved a delete,
// an insert and a read into the per-line loop, which was tolerable only while
// the pipeline was reading the first two hundred rows of every file and
// silently discarding the rest.
func (s *Supplier) Resolve(ctx context.Context, lines []*smartorder.Line) (money.Amount, error) {
	productIDs := matchedProductIDs(lines)
	if len(productIDs) == 0 {
		// Nothing matched, so nothing can be ordered. The lines still need their
		// outcome set, or the results screen reports a blank status for every
		// row rather than "not found in the catalogue".
		for _, l := range lines {
			if !l.Matched() {
				l.Outcome = smartorder.OutcomeUnmatched
			}
		}
		return money.Amount{}, nil
	}

	// One query for the whole file.
	offers, err := s.repo.LoadOffers(ctx, s.cfg.OrganizationID, productIDs)
	if err != nil {
		return money.Amount{}, err
	}
	byProduct := make(map[int64][]smartorder.Offer, len(productIDs))
	for _, o := range offers {
		byProduct[o.ProductID] = append(byProduct[o.ProductID], o)
	}

	// Coverage and institutional verdicts are cached per vendor: a file with ten
	// thousand lines typically touches a few dozen vendors, and asking the same
	// question once per line would undo the batching everywhere else.
	//
	// The institutional key is (vendor, vendor branch) rather than the product,
	// because the rule is a fact about those two branches and nothing else. It
	// used to be keyed by product id, which asked the question once per product
	// and cached a per-vendor answer under it — so the FIRST vendor's verdict
	// was applied to every other vendor of the same product.
	covCache := make(map[int64]coverageVerdict)
	instCache := make(map[instKey]bool)
	gateCache := make(map[int64]smartorder.GateVerdict)

	if s.gate != nil {
		var gateLines []smartorder.GateLine
		seen := make(map[int64]bool)
		for _, l := range lines {
			if !l.Matched() || l.EffectiveQty <= 0 {
				continue
			}
			qty := int(l.EffectiveQty)
			if qty <= 0 {
				qty = 1
			}
			for _, o := range byProduct[*l.MatchedProductID] {
				if !seen[o.VariantID] {
					seen[o.VariantID] = true
					gateLines = append(gateLines, smartorder.GateLine{
						VariantID:   o.VariantID,
						VendorOrgID: o.VendorOrgID,
						Quantity:    qty,
					})
				}
			}
		}
		if len(gateLines) > 0 {
			verdicts, err := s.gate.Check(ctx, s.cfg.OrganizationID, s.branch.BranchID, s.now(), gateLines)
			if err != nil {
				return money.Amount{}, err
			}
			gateCache = verdicts
		}
	}

	// Pass one — evaluate every line in memory.
	byLine := make(map[int64][]smartorder.Candidate, len(lines))
	orderable := make([]*smartorder.Line, 0, len(lines))
	runID := int64(0)
	for _, l := range lines {
		if runID == 0 {
			runID = l.RunID
		}
		if !l.Matched() {
			l.Outcome = smartorder.OutcomeUnmatched
			continue
		}
		if l.EffectiveQty <= 0 {
			l.Outcome = smartorder.OutcomeZeroQty
			continue
		}

		candidates, err := s.buildCandidatesWithGate(ctx, l, byProduct[*l.MatchedProductID], covCache, instCache, gateCache)
		if err != nil {
			return money.Amount{}, err
		}
		byLine[l.ID] = candidates

		outcome, reason := smartorder.OutcomeFor(true, l.EffectiveQty, candidates)
		l.Outcome = outcome
		l.OutcomeReason = string(reason)
		if outcome == smartorder.OutcomeOrdered {
			orderable = append(orderable, l)
		}
	}

	// Pass two — one write for the file. The ids come back with it, because the
	// selection has to reference rows that exist and reading them back per line
	// was a third round trip per row.
	stored, err := s.repo.ReplaceRunCandidates(ctx, runID, byLine)
	if err != nil {
		return money.Amount{}, err
	}

	// Pass three — choose a supplier per line, in memory.
	total := money.Amount{}
	selections := make([]*smartorder.Selection, 0, len(orderable))
	for _, l := range orderable {
		written := stored[l.ID]
		sel, ok := smartorder.Select(s.cfg, l.ID, written)
		if !ok {
			l.Outcome = smartorder.OutcomeNoSupplier
			continue
		}
		chosen := findCandidate(written, sel.CandidateID)
		if chosen != nil {
			net, err := smartorder.LineNet(chosen.NetUnitPrice, l.EffectiveQty)
			if err != nil {
				return money.Amount{}, err
			}
			sel.LineNet = net
			if total, err = total.Add(net); err != nil {
				return money.Amount{}, err
			}
		}
		selections = append(selections, sel)
	}

	if err := s.repo.UpsertSelections(ctx, selections); err != nil {
		return money.Amount{}, err
	}
	return total, nil
}

type coverageVerdict struct {
	covered  bool
	distance int
}

// instKey identifies one Corporate Operations question. Two offers from the
// same supplier branch always get the same answer; two from different branches
// of the same supplier may not, because the works are held per branch.
type instKey struct {
	vendorOrgID    int64
	vendorBranchID int64
}

// buildCandidates turns raw offers into evaluated candidates for one line.
func (s *Supplier) buildCandidates(ctx context.Context, l *smartorder.Line, offers []smartorder.Offer,
	covCache map[int64]coverageVerdict, instCache map[instKey]bool) ([]smartorder.Candidate, error) {
	return s.buildCandidatesWithGate(ctx, l, offers, covCache, instCache, nil)
}

func (s *Supplier) buildCandidatesWithGate(ctx context.Context, l *smartorder.Line, offers []smartorder.Offer,
	covCache map[int64]coverageVerdict, instCache map[instKey]bool, gateCache map[int64]smartorder.GateVerdict) ([]smartorder.Candidate, error) {

	out := make([]smartorder.Candidate, 0, len(offers))

	if s.gate != nil && (gateCache == nil || len(gateCache) == 0) && len(offers) > 0 {
		qty := int(l.EffectiveQty)
		if qty <= 0 {
			qty = 1
		}
		gateLines := make([]smartorder.GateLine, len(offers))
		for i, o := range offers {
			gateLines[i] = smartorder.GateLine{
				VariantID:   o.VariantID,
				VendorOrgID: o.VendorOrgID,
				Quantity:    qty,
			}
		}
		vd, err := s.gate.Check(ctx, s.cfg.OrganizationID, s.branch.BranchID, s.now(), gateLines)
		if err != nil {
			return nil, err
		}
		gateCache = vd
	}

	for _, o := range offers {
		c := smartorder.Candidate{
			LineID:         l.ID,
			OrganizationID: s.cfg.OrganizationID,
			VendorOrgID:    o.VendorOrgID,
			VariantID:      o.VariantID,
			BranchID:       o.BranchID,
			Price:          money.FromMinor(o.PriceMinor),
			DiscountBps:    o.DiscountBps,
			Unit:           o.Unit,
			MinOrderQty:    o.MinOrderQty,
			StockQty:       o.StockQty,
			IsFollowed:     o.IsFollowed,
		}
		// Net after discount, in minor units. The discount is basis points, so
		// the arithmetic stays exact — no float touches a price.
		discount := c.Price.ApplyPercent(o.DiscountBps)
		net, err := c.Price.Sub(discount)
		if err != nil {
			return nil, err
		}
		c.NetUnitPrice = net

		// The purchase rule lives in commerce.CheckAvailability and reaches
		// smart ordering only through the gate. There is deliberately no second
		// implementation here to fall back on: this file used to carry one, and
		// with no coverage gate wired it answered "covered" for every supplier,
		// so a run built orders against branches nobody delivers to and checkout
		// refused them one click later.
		//
		// A missing gate is a wiring fault, not a mode. It fails closed and says
		// so, because guessing is what produced the defect this replaced.
		verdict, ok := gateCache[o.VariantID]
		if !ok {
			c.Eligible = false
			c.IneligibleReason = smartorder.ReasonInactive
		} else {
			c.Eligible = verdict.Allowed
			if !verdict.Allowed {
				c.IneligibleReason = smartorder.EvaluateReason(verdict.Reason)
			}
		}
		out = append(out, c)
	}
	return out, nil
}

func matchedProductIDs(lines []*smartorder.Line) []int64 {
	seen := make(map[int64]bool)
	var out []int64
	for _, l := range lines {
		if !l.Matched() {
			continue
		}
		id := *l.MatchedProductID
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func findCandidate(candidates []smartorder.Candidate, id int64) *smartorder.Candidate {
	for i := range candidates {
		if candidates[i].ID == id {
			return &candidates[i]
		}
	}
	return nil
}
