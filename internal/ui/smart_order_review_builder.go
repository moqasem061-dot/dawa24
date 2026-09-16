package ui

import (
	"context"
	"net/http"
	"sort"
	"strconv"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SmartOrderReviewPage renders step 5: grouping by supplier and placing orders.
//
// It reads only smartorder data. The ordinary shopping cart is never touched:
// an abandoned import must not leave items in a cart the buyer believes is
// empty (FR-042).
func (h *UIHandler) SmartOrderReviewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}

	limit := 25
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && (l == 10 || l == 25 || l == 50 || l == 100 || l == -1) {
		limit = l
	}

	orderable, _, err := h.smartOrderSvc.Results(ctx, run, smartorder.LineFilter{
		Outcome: string(smartorder.OutcomeOrdered), All: true,
	})
	if err != nil {
		http.Error(w, i18n.T(lang, "errors.data_load_failed"), http.StatusInternalServerError)
		return
	}

	data := pages.SmartOrderReviewData{
		Run:        run,
		Error:      r.URL.Query().Get("error"),
		BranchName: h.branchName(ctx, run),
		// Shown once, on the render that follows a refused finalisation.
		Stale:   h.smartOrderStale.take(run.PublicID),
		Page:    page,
		PerPage: limit,
	}

	type vendorLineItem struct {
		vendor string
		line   pages.SmartOrderReviewLine
	}

	byVendor := map[string]*pages.SmartOrderReviewGroup{}
	var vendorOrder []string

	for _, l := range orderable {
		sel, err := h.smartOrderSvc.Selection(ctx, run.OrganizationID, l.ID)
		if err != nil {
			continue
		}
		candidates, err := h.smartOrderSvc.Candidates(ctx, run.OrganizationID, l.ID)
		if err != nil {
			continue
		}
		chosen, alternatives := splitCandidates(candidates, sel.CandidateID)
		if chosen == nil {
			continue
		}
		vendor := h.vendorName(ctx, chosen.VendorOrgID)

		group, exists := byVendor[vendor]
		if !exists {
			group = &pages.SmartOrderReviewGroup{VendorName: vendor}
			byVendor[vendor] = group
			vendorOrder = append(vendorOrder, vendor)
		}

		lineNet := sel.LineNet
		if calculatedNet, err := smartorder.LineNet(chosen.NetUnitPrice, l.EffectiveQty); err == nil {
			lineNet = calculatedNet
		}

		revLine := pages.SmartOrderReviewLine{
			Line:           l,
			VendorName:     vendor,
			AvailableStock: chosen.StockQty,
			UnitPrice:      chosen.NetUnitPrice,
			DiscountPct:    float64(chosen.DiscountBps) / 100,
			LineNet:        lineNet,
			DecidedBy:      sel.DecidedBy,
			SkippedName:    h.skippedVendorName(ctx, candidates, sel.SkippedCandidateID),
			SkippedExcess:  derefFloat(sel.SkippedExcessPct),
			Alternatives:   alternatives,
		}
		group.TotalCount++
		if sum, err := group.Subtotal.Add(lineNet); err == nil {
			group.Subtotal = sum
		}
		group.Lines = append(group.Lines, revLine)
	}

	sort.Strings(vendorOrder)

	var allLineItems []vendorLineItem
	for _, v := range vendorOrder {
		g := byVendor[v]
		data.AllGroups = append(data.AllGroups, *g)
		for _, l := range g.Lines {
			allLineItems = append(allLineItems, vendorLineItem{vendor: v, line: l})
		}
	}

	data.TotalLines = len(allLineItems)

	// Recompute order estimated total from all groups to guarantee consistency
	var grandTotal money.Amount
	for _, g := range data.AllGroups {
		if sum, err := grandTotal.Add(g.Subtotal); err == nil {
			grandTotal = sum
		}
	}
	if grandTotal.IsPositive() || len(data.AllGroups) == 0 {
		data.Run.EstimatedTotal = grandTotal
	}

	if limit == -1 {
		data.Groups = data.AllGroups
	} else {
		start := (page - 1) * limit
		if start < 0 {
			start = 0
		}
		if start > data.TotalLines {
			start = data.TotalLines
		}
		end := start + limit
		if end > data.TotalLines {
			end = data.TotalLines
		}

		pageItems := allLineItems[start:end]
		pagedByVendor := map[string]*pages.SmartOrderReviewGroup{}
		var pagedVendorOrder []string

		for _, item := range pageItems {
			pg, exists := pagedByVendor[item.vendor]
			if !exists {
				fullGroup := byVendor[item.vendor]
				pg = &pages.SmartOrderReviewGroup{
					VendorName: item.vendor,
					TotalCount: fullGroup.TotalCount,
					Subtotal:   fullGroup.Subtotal,
				}
				pagedByVendor[item.vendor] = pg
				pagedVendorOrder = append(pagedVendorOrder, item.vendor)
			}
			pg.Lines = append(pg.Lines, item.line)
		}

		for _, v := range pagedVendorOrder {
			data.Groups = append(data.Groups, *pagedByVendor[v])
		}
	}

	// Everything the buyer is not getting, shown separately rather than omitted
	// (FR-045).
	for _, outcome := range []smartorder.Outcome{
		smartorder.OutcomeUnmatched, smartorder.OutcomeNoSupplier,
		smartorder.OutcomeCoverageBlocked, smartorder.OutcomeInstitutionalBlocked,
		smartorder.OutcomeOutOfStock, smartorder.OutcomeBelowMinQty,
		smartorder.OutcomeZeroQty, smartorder.OutcomeQuotaBlocked,
	} {
		excluded, _, err := h.smartOrderSvc.Results(ctx, run, smartorder.LineFilter{
			Outcome: string(outcome), Limit: 200,
		})
		if err == nil {
			data.Excluded = append(data.Excluded, excluded...)
		}
	}

	h.renderPage(ctx, w, "render smart order review page", pages.SmartOrderReviewPage(lang, dir, data))
}

// splitCandidates separates the chosen offer from the alternatives the buyer
// could switch to.
func splitCandidates(candidates []smartorder.Candidate, chosenID int64) (*smartorder.Candidate, int) {
	var chosen *smartorder.Candidate
	alternatives := 0
	for i := range candidates {
		if candidates[i].ID == chosenID {
			chosen = &candidates[i]
			continue
		}
		if candidates[i].Eligible {
			alternatives++
		}
	}
	return chosen, alternatives
}

// skippedVendorName names the supplier the tolerance band passed over, so the
// line can say who was skipped rather than just that someone was.
func (h *UIHandler) skippedVendorName(ctx context.Context, candidates []smartorder.Candidate, skippedID *int64) string {
	if skippedID == nil {
		return ""
	}
	for _, c := range candidates {
		if c.ID == *skippedID {
			return h.vendorName(ctx, c.VendorOrgID)
		}
	}
	return ""
}

func (h *UIHandler) branchName(ctx context.Context, run *smartorder.Run) string {
	if h.orgSvc == nil {
		return ""
	}
	b, err := h.orgSvc.GetBranch(ctx, run.BranchID)
	if err != nil || b == nil {
		return ""
	}
	return b.Name.Get("ar")
}

func derefFloat(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
