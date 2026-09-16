package commerce

import (
	"context"
	"fmt"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AvailabilityLine describes one prospective purchase line within a batch.
type AvailabilityLine struct {
	VariantID   int64
	VendorOrgID int64
	Quantity    int
}

// CheckAvailabilityBatch answers many lines for one buyer/branch/moment.
// It exists so a listing costs a bounded number of queries instead of one
// CheckAvailability per row.
func (s *Service) CheckAvailabilityBatch(
	ctx context.Context, customerOrgID, customerBranchID int64,
	when time.Time, lines []AvailabilityLine,
) (map[int64]AvailabilityResult, error) {
	out := make(map[int64]AvailabilityResult, len(lines))
	if len(lines) == 0 {
		return out, nil
	}

	if s.availability == nil {
		for _, line := range lines {
			out[line.VariantID] = denied(ReasonVariantInvalid, 0,
				i18n.T("ar", "err.avail_check_failed"),
				"Availability cannot be verified right now.")
		}
		return out, nil
	}

	if when.IsZero() {
		when = time.Now()
	}

	// 1. Initial fast checks that require no external queries.
	type pendingLine struct {
		line AvailabilityLine
	}
	var pending []pendingLine
	for _, l := range lines {
		if l.Quantity <= 0 {
			out[l.VariantID] = denied(ReasonQuantityInvalid, 0,
				i18n.T("ar", "err.qty_must_be_positive"),
				"Quantity must be greater than zero.")
			continue
		}
		if l.VendorOrgID <= 0 {
			out[l.VariantID] = denied(ReasonVendorInvalid, 0,
				i18n.T("ar", "err.supplier_not_specified"),
				"No supplier was specified for this item.")
			continue
		}
		if customerOrgID > 0 && customerOrgID == l.VendorOrgID {
			out[l.VariantID] = denied(ReasonOwnOrganization, 0,
				i18n.T("ar", "err.own_organization_supply"),
				"You cannot buy your own organization's items.")
			continue
		}
		pending = append(pending, pendingLine{line: l})
	}

	if len(pending) == 0 {
		return out, nil
	}

	// 2. Batch load vendors and variants for all pending lines.
	vendorIDsMap := make(map[int64]struct{}, len(pending))
	variantIDsMap := make(map[int64]struct{}, len(pending))
	for _, p := range pending {
		vendorIDsMap[p.line.VendorOrgID] = struct{}{}
		if p.line.VariantID > 0 {
			variantIDsMap[p.line.VariantID] = struct{}{}
		}
	}

	vendorIDs := make([]int64, 0, len(vendorIDsMap))
	for vid := range vendorIDsMap {
		vendorIDs = append(vendorIDs, vid)
	}

	variantIDs := make([]int64, 0, len(variantIDsMap))
	for vid := range variantIDsMap {
		variantIDs = append(variantIDs, vid)
	}

	vendors, err := s.availability.VendorsByIDs(ctx, vendorIDs)
	if err != nil {
		return nil, fmt.Errorf("availability batch: load vendors: %w", err)
	}

	variants, err := s.availability.VariantsByIDs(ctx, variantIDs)
	if err != nil {
		return nil, fmt.Errorf("availability batch: load variants: %w", err)
	}

	// 3. Process vendor, variant, and stock checks per line.
	// Lines that pass are eligible for branch/coverage/institutional checks.
	type readyLine struct {
		line    AvailabilityLine
		variant VariantAvailability
	}
	var readyForBranch []readyLine

	for _, p := range pending {
		l := p.line
		vendor, vendorFound := vendors[l.VendorOrgID]
		if !vendorFound || vendor.ID == 0 || !vendor.IsVendor {
			out[l.VariantID] = denied(ReasonVendorInvalid, 0,
				i18n.T("ar", "err.supplier_invalid"),
				"The specified supplier is not valid.")
			continue
		}
		if !vendor.Approved {
			out[l.VariantID] = denied(ReasonVendorUnapproved, 0,
				i18n.T("ar", "err.supplier_invalid"),
				"This supplier is not currently approved.")
			continue
		}

		if l.VariantID <= 0 {
			out[l.VariantID] = denied(ReasonVariantInvalid, 0,
				i18n.TDefault("w4_mod.w4str_126_126"),
				"No product variant was specified.")
			continue
		}
		variant, variantFound := variants[l.VariantID]
		if !variantFound || variant.ID == 0 {
			out[l.VariantID] = denied(ReasonVariantInvalid, 0,
				i18n.TDefault("w4_mod.w4str_127_127"),
				"The requested product variant does not exist.")
			continue
		}
		if !variant.Active {
			out[l.VariantID] = denied(ReasonVariantInactive, 0,
				i18n.TDefault("w4_mod.w4str_128_128"),
				"This product is not currently available.")
			continue
		}
		if variant.OrganizationID != l.VendorOrgID {
			out[l.VariantID] = denied(ReasonWrongVendor, 0,
				i18n.TDefault("w4_mod.w4str_129_129"),
				"This product does not belong to the specified supplier.")
			continue
		}

		if variant.StockQty <= 0 {
			out[l.VariantID] = denied(ReasonOutOfStock, 0,
				i18n.TDefault("w4_mod.w4str_130_130"),
				"This item is out of stock at the supplier.")
			continue
		}
		if l.Quantity > variant.StockQty {
			out[l.VariantID] = denied(ReasonInsufficientStock, variant.StockQty,
				fmt.Sprintf(i18n.TDefault("w4_mod.d_131"), variant.StockQty),
				fmt.Sprintf("Only %d available from this supplier.", variant.StockQty))
			continue
		}
		if variant.MinOrderQty > 0 && l.Quantity < variant.MinOrderQty {
			out[l.VariantID] = denied(ReasonBelowMinimum, variant.StockQty,
				fmt.Sprintf(i18n.TDefault("w4_mod.d_132"), variant.MinOrderQty),
				fmt.Sprintf("Minimum order quantity for this item is %d.", variant.MinOrderQty))
			continue
		}

		readyForBranch = append(readyForBranch, readyLine{line: l, variant: variant})
	}

	if len(readyForBranch) == 0 {
		return out, nil
	}

	// 4. Branch validation.
	if customerBranchID <= 0 {
		for _, r := range readyForBranch {
			out[r.line.VariantID] = denied(ReasonBranchInvalid, r.variant.StockQty,
				i18n.TDefault("w4_mod.w4str_133_133"),
				"Select a receiving branch first.")
		}
		return out, nil
	}

	branch, err := s.availability.CustomerBranch(ctx, customerBranchID)
	if err != nil {
		return nil, fmt.Errorf("availability batch: load branch %d: %w", customerBranchID, err)
	}
	if branch.ID == 0 {
		for _, r := range readyForBranch {
			out[r.line.VariantID] = denied(ReasonBranchInvalid, r.variant.StockQty,
				i18n.TDefault("w4_mod.w4str_134_134"),
				"The selected receiving branch does not exist.")
		}
		return out, nil
	}
	if customerOrgID > 0 && branch.OrganizationID != customerOrgID {
		for _, r := range readyForBranch {
			out[r.line.VariantID] = denied(ReasonBranchNotOwned, r.variant.StockQty,
				i18n.TDefault("w4_mod.w4str_135_135"),
				"The selected branch does not belong to your organization.")
		}
		return out, nil
	}
	if len(branch.InstitutionalWorks) == 0 {
		for _, r := range readyForBranch {
			out[r.line.VariantID] = denied(ReasonBranchNoInstitutionalWorks, r.variant.StockQty,
				i18n.TDefault("commerce.availability.branch_no_institutional_works"),
				"No institutional works are associated with this branch. Please enable institutional works to place orders.")
		}
		return out, nil
	}

	// 5. Institutional connection check in batch.
	branchLines := make([]AvailabilityLine, len(readyForBranch))
	for i, r := range readyForBranch {
		branchLines[i] = r.line
	}
	instConnections, err := s.availability.VendorInstitutionalConnections(ctx, customerBranchID, branchLines)
	if err != nil {
		return nil, fmt.Errorf("availability batch: institutional connections: %w", err)
	}

	// Filter lines that passed institutional connections.
	var readyForCoverage []readyLine
	for _, r := range readyForBranch {
		if !instConnections[r.line.VariantID] {
			out[r.line.VariantID] = denied(ReasonBranchInstitutionalMismatch, r.variant.StockQty,
				i18n.TDefault("commerce.availability.branch_institutional_mismatch"),
				"The receiving branch's institutional work is not connected to the vendor's branch institutional works according to platform settings.")
			continue
		}
		readyForCoverage = append(readyForCoverage, r)
	}

	if len(readyForCoverage) == 0 {
		return out, nil
	}

	// 6. Branch location check.
	hasLocation := (branch.Latitude != nil && branch.Longitude != nil) || (branch.CityID != nil && *branch.CityID > 0)
	if !hasLocation {
		for _, r := range readyForCoverage {
			out[r.line.VariantID] = denied(ReasonBranchNoLocation, r.variant.StockQty,
				i18n.TDefault("w4_mod.w4str_136_136"),
				"This branch has no map location yet; set one to order.")
		}
		return out, nil
	}

	var bLat, bLon float64
	if branch.Latitude != nil {
		bLat = *branch.Latitude
	}
	if branch.Longitude != nil {
		bLon = *branch.Longitude
	}

	// Coverage is branch-specific when an offer names its supplier branch.
	type coverageKey struct {
		vendorOrgID    int64
		vendorBranchID int64
	}
	coverageCache := make(map[coverageKey]bool)
	for _, r := range readyForCoverage {
		key := coverageKey{vendorOrgID: r.line.VendorOrgID, vendorBranchID: r.variant.VendorBranchID}
		if _, ok := coverageCache[key]; !ok {
			covered, err := s.availability.VendorCovers(ctx, key.vendorOrgID, key.vendorBranchID, bLat, bLon, when.Weekday(), branch.CityID)
			if err != nil {
				return nil, fmt.Errorf("availability batch: coverage for vendor %d branch %d: %w", key.vendorOrgID, key.vendorBranchID, err)
			}
			coverageCache[key] = covered
		}
	}

	// 7. Coverage and Quota evaluation.
	//
	// The quota consumption for every capped variant on this page is read in
	// one statement before the loop. Asking inside it cost one round trip per
	// offer, which on a ninety-six card catalogue page is ninety-six sequential
	// queries for an answer the same statement can produce once.
	quotaVariantIDs := make([]int64, 0, len(readyForCoverage))
	for _, r := range readyForCoverage {
		key := coverageKey{vendorOrgID: r.line.VendorOrgID, vendorBranchID: r.variant.VendorBranchID}
		if r.variant.QuotaLimit > 0 && coverageCache[key] {
			quotaVariantIDs = append(quotaVariantIDs, r.line.VariantID)
		}
	}
	quotaUsed, err := s.branchQuotaUsedFor(ctx, quotaVariantIDs, customerBranchID)
	if err != nil {
		return nil, fmt.Errorf("availability batch: quota for branch %d: %w", customerBranchID, err)
	}

	for _, r := range readyForCoverage {
		key := coverageKey{vendorOrgID: r.line.VendorOrgID, vendorBranchID: r.variant.VendorBranchID}
		if !coverageCache[key] {
			out[r.line.VariantID] = denied(ReasonNotCovered, r.variant.StockQty,
				i18n.TDefault("w4_mod.w4str_137_137"),
				"This supplier does not cover your branch's location on this day.")
			continue
		}

		maxQty := r.variant.StockQty
		if r.variant.QuotaLimit > 0 {
			usage := BranchQuotaUsage{
				VariantID: r.line.VariantID,
				BranchID:  customerBranchID,
				Limit:     r.variant.QuotaLimit,
				Used:      quotaUsed[r.line.VariantID],
			}
			remaining := usage.Remaining()
			if remaining <= 0 {
				out[r.line.VariantID] = denied(ReasonQuotaExhausted, 0,
					fmt.Sprintf(i18n.T(i18n.AR, "quota.exhausted"), r.variant.QuotaLimit),
					fmt.Sprintf(i18n.T(i18n.EN, "quota.exhausted"), r.variant.QuotaLimit))
				continue
			}
			if r.line.Quantity > remaining {
				out[r.line.VariantID] = denied(ReasonQuotaExceeded, remaining,
					fmt.Sprintf(i18n.T(i18n.AR, "quota.exceeded"), remaining, r.variant.QuotaLimit),
					fmt.Sprintf(i18n.T(i18n.EN, "quota.exceeded"), remaining, r.variant.QuotaLimit))
				continue
			}
			if remaining < maxQty {
				maxQty = remaining
			}
		}

		out[r.line.VariantID] = AvailabilityResult{
			Allowed:     true,
			MaxQuantity: maxQty,
			Reason:      ReasonOK,
		}
	}

	return out, nil
}
