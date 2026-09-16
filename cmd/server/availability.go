package main

import (
	"context"
	"errors"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// availabilityProbe answers the cross-module questions behind
// commerce.CheckAvailability: is this a real approved vendor, does this variant
// exist and have stock, does the branch belong to the buyer, and does the
// supplier cover it today.
//
// It lives in the composition root because modules must not import each other
// (ADR 0002) — the same reason docsGate is built here. commerce declares the
// AvailabilityProbe interface; this is its only implementation.
type availabilityProbe struct {
	catalog   *catalog.Service
	org       *org.Service
	coverage  *workflow.CoverageService
	inventory *inventory.Service
}

func newAvailabilityProbe(cat *catalog.Service, o *org.Service, cov *workflow.CoverageService, inv *inventory.Service) commerce.AvailabilityProbe {
	return &availabilityProbe{catalog: cat, org: o, coverage: cov, inventory: inv}
}

// Variant reads the stock and ownership facts for one sellable variant.
//
// The read runs AsSystem: a pharmacy is buying from a supplier, so it is by
// definition reading another tenant's catalog row. Only the four fields below
// cross the boundary, and only to answer "may I buy this".
func (p *availabilityProbe) Variant(ctx context.Context, variantID int64) (commerce.VariantAvailability, error) {
	if p.catalog == nil {
		return commerce.VariantAvailability{}, nil
	}
	v, err := p.catalog.GetVariant(database.AsSystem(ctx), variantID)
	if err != nil {
		if isNotFound(err) {
			return commerce.VariantAvailability{}, nil // zero ID = "does not exist"
		}
		return commerce.VariantAvailability{}, err
	}
	if v == nil {
		return commerce.VariantAvailability{}, nil
	}
	// Stock comes from inventory.stocks, NOT from v.StockQty. That field is
	// declared on catalog.ProductVariant but no query populates it — the column
	// does not exist on catalog.product_variants — so it is a permanent zero.
	// Reading it here would refuse every purchase.
	qty := 0
	if p.inventory != nil {
		n, err := p.inventory.AvailableQuantity(ctx, variantID)
		if err != nil {
			return commerce.VariantAvailability{}, err
		}
		qty = n
	}

	return commerce.VariantAvailability{
		ID:             v.ID,
		OrganizationID: v.OrganizationID,
		VendorBranchID: branchIDOf(v.BranchID),
		StockQty:       qty,
		MinOrderQty:    v.MinOrderQty,
		Active:         v.Status == catalog.StatusActive,
		// The supplier's per-branch cap. Only the number crosses; how much of
		// it the buying branch has already taken is commerce's own arithmetic
		// over its own orders, and asking catalog for it would put the
		// consumption in two places.
		QuotaLimit:             v.QuotaLimitOrZero(),
		Price:                  v.Price,
		Discount:               v.Discount,
		EffectivePrice:         v.EffectiveSellingPrice(),
		IsNegotiable:           v.IsNegotiable,
		CostPrice:              v.CostPrice,
		CostDiscountPercentage: v.CostDiscountPercentage,
	}, nil
}

// Vendor reports whether the supplier is a real, approved vendor organization.
// AsSystem for the same reason as Variant: the buyer is not this tenant.
func (p *availabilityProbe) Vendor(ctx context.Context, orgID int64) (commerce.VendorAvailability, error) {
	if p.org == nil {
		return commerce.VendorAvailability{}, nil
	}
	o, err := p.org.GetOrganization(database.AsSystem(ctx), orgID)
	if err != nil {
		if isNotFound(err) {
			return commerce.VendorAvailability{}, nil
		}
		return commerce.VendorAvailability{}, err
	}
	if o == nil {
		return commerce.VendorAvailability{}, nil
	}
	return commerce.VendorAvailability{
		ID:       o.ID,
		IsVendor: string(o.Type) == "vendor" || string(o.Type) == "supplier" || string(o.Type) == "company" || string(o.Type) == "agency",
		Approved: string(o.Status) == "approved",
	}, nil
}

// CustomerBranch reads the buying company's own branch -- a pharmacy's, or a
// supplier's warehouse when a supplier is the one restocking. commerce compares
// the returned OrganizationID against the actor's, so a caller cannot probe
// another company's branches.
func (p *availabilityProbe) CustomerBranch(ctx context.Context, branchID int64) (commerce.BranchAvailability, error) {
	if p.org == nil {
		return commerce.BranchAvailability{}, nil
	}
	b, err := p.org.GetBranch(database.AsSystem(ctx), branchID)
	if err != nil {
		if isNotFound(err) {
			return commerce.BranchAvailability{}, nil
		}
		return commerce.BranchAvailability{}, err
	}
	if b == nil {
		return commerce.BranchAvailability{}, nil
	}
	return commerce.BranchAvailability{
		ID:                 b.ID,
		OrganizationID:     b.OrganizationID,
		CityID:             b.CityID,
		Latitude:           b.Latitude,
		Longitude:          b.Longitude,
		InstitutionalWorks: b.InstitutionalWorks,
	}, nil
}

// VendorCovers defers to the one implementation of the coverage rule. There is
// deliberately no second distance calculation anywhere in this codebase.
func (p *availabilityProbe) VendorCovers(ctx context.Context, vendorOrgID, vendorBranchID int64, lat, lon float64, day time.Weekday, cityID *int64) (bool, error) {
	if p.coverage == nil {
		return false, nil // fail closed
	}
	var targetCityID *int64
	if cityID != nil && *cityID > 0 {
		targetCityID = cityID
	}
	served, _, err := p.coverage.ServesPoint(ctx, vendorOrgID, day, workflow.Coord{
		Lat:    lat,
		Lon:    lon,
		CityID: targetCityID,
	}, vendorBranchID)
	if err != nil {
		return false, err
	}
	return served, nil
}

// VendorInstitutionalConnection answers commerce's Corporate Operations
// question: is the buyer's branch connected to a branch of this supplier?
//
// The rule itself is org.Service.BranchesInstitutionallyConnected. It used to
// be written out here, and smart ordering had a second, different version of
// it — which is how a smart order's review screen could show a line as
// orderable that this very check then refused at the last click. There is one
// implementation now, and both callers reach it through this method or through
// the smart-order gate, which calls the same function.
//
// The only thing left here is the translation commerce needs and org must not
// know about: resolving a variant to the branch its offer sits on. A variant
// naming no branch means the offer can come from any branch of the supplier,
// which is what a nil VendorBranchID says.
func (p *availabilityProbe) VendorInstitutionalConnection(ctx context.Context, vendorOrgID int64, customerBranchID int64, variantID int64) (bool, error) {
	if p.org == nil {
		return false, nil
	}

	var vendorBranchID *int64
	if variantID > 0 && p.catalog != nil {
		v, err := p.catalog.GetVariant(database.AsSystem(ctx), variantID)
		if err != nil {
			return false, err
		}
		if v != nil && v.BranchID != nil && *v.BranchID > 0 {
			branchID := *v.BranchID
			vendorBranchID = &branchID
		}
	}

	return p.org.BranchesInstitutionallyConnected(database.AsSystem(ctx), org.InstitutionalConnection{
		BuyerBranchID:  customerBranchID,
		VendorOrgID:    vendorOrgID,
		VendorBranchID: vendorBranchID,
	})
}

// VariantsByIDs batch-resolves variants and their available quantities.
func (p *availabilityProbe) VariantsByIDs(ctx context.Context, variantIDs []int64) (map[int64]commerce.VariantAvailability, error) {
	out := make(map[int64]commerce.VariantAvailability, len(variantIDs))
	if p.catalog == nil || len(variantIDs) == 0 {
		return out, nil
	}
	vars, err := p.catalog.GetVariantsByIDs(database.AsSystem(ctx), variantIDs)
	if err != nil {
		return nil, err
	}
	stockMap := make(map[int64]int)
	if p.inventory != nil {
		sm, err := p.inventory.AvailableQuantities(ctx, variantIDs)
		if err != nil {
			return nil, err
		}
		stockMap = sm
	}
	for id, v := range vars {
		if v == nil {
			continue
		}
		out[id] = commerce.VariantAvailability{
			ID:             v.ID,
			OrganizationID: v.OrganizationID,
			VendorBranchID: branchIDOf(v.BranchID),
			StockQty:       stockMap[v.ID],
			MinOrderQty:    v.MinOrderQty,
			Active:         v.Status == catalog.StatusActive,
			QuotaLimit:             v.QuotaLimitOrZero(),
			Price:                  v.Price,
			Discount:               v.Discount,
			EffectivePrice:         v.EffectiveSellingPrice(),
			IsNegotiable:           v.IsNegotiable,
			CostPrice:              v.CostPrice,
			CostDiscountPercentage: v.CostDiscountPercentage,
		}
	}
	return out, nil
}

func branchIDOf(branchID *int64) int64 {
	if branchID == nil {
		return 0
	}
	return *branchID
}

// VendorsByIDs batch-resolves vendor organizations.
func (p *availabilityProbe) VendorsByIDs(ctx context.Context, orgIDs []int64) (map[int64]commerce.VendorAvailability, error) {
	out := make(map[int64]commerce.VendorAvailability, len(orgIDs))
	if p.org == nil || len(orgIDs) == 0 {
		return out, nil
	}
	orgs, err := p.org.GetOrganizations(database.AsSystem(ctx), orgIDs)
	if err != nil {
		return nil, err
	}
	for id, o := range orgs {
		if o == nil {
			continue
		}
		out[id] = commerce.VendorAvailability{
			ID:       o.ID,
			IsVendor: string(o.Type) == "vendor" || string(o.Type) == "supplier" || string(o.Type) == "company" || string(o.Type) == "agency",
			Approved: string(o.Status) == "approved",
		}
	}
	return out, nil
}

// VendorInstitutionalConnections batch-evaluates institutional connectivity.
func (p *availabilityProbe) VendorInstitutionalConnections(ctx context.Context, customerBranchID int64, lines []commerce.AvailabilityLine) (map[int64]bool, error) {
	out := make(map[int64]bool, len(lines))
	if p.org == nil || len(lines) == 0 {
		return out, nil
	}

	variantIDs := make([]int64, 0, len(lines))
	for _, l := range lines {
		if l.VariantID > 0 {
			variantIDs = append(variantIDs, l.VariantID)
		}
	}

	varMap := make(map[int64]*catalog.ProductVariant)
	if p.catalog != nil && len(variantIDs) > 0 {
		vm, err := p.catalog.GetVariantsByIDs(database.AsSystem(ctx), variantIDs)
		if err != nil {
			return nil, err
		}
		varMap = vm
	}

	type connKey struct {
		vendorOrgID    int64
		hasBranch      bool
		vendorBranchID int64
	}
	cache := make(map[connKey]bool)

	for _, l := range lines {
		if l.VariantID <= 0 {
			continue
		}
		var vendorBranchID *int64
		key := connKey{vendorOrgID: l.VendorOrgID}
		if v := varMap[l.VariantID]; v != nil && v.BranchID != nil && *v.BranchID > 0 {
			bID := *v.BranchID
			vendorBranchID = &bID
			key.hasBranch = true
			key.vendorBranchID = bID
		}

		if connected, ok := cache[key]; ok {
			out[l.VariantID] = connected
			continue
		}

		connected, err := p.org.BranchesInstitutionallyConnected(database.AsSystem(ctx), org.InstitutionalConnection{
			BuyerBranchID:  customerBranchID,
			VendorOrgID:    l.VendorOrgID,
			VendorBranchID: vendorBranchID,
		})
		if err != nil {
			return nil, err
		}
		cache[key] = connected
		out[l.VariantID] = connected
	}
	return out, nil
}

// isNotFound distinguishes "this row does not exist (or is not visible to this
// tenant)" from a real failure. A missing row is a refusal reason, not an
// outage: commerce turns it into a message the pharmacy can act on, while a
// genuine error surfaces and stops the purchase.
func isNotFound(err error) bool {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e.Kind == apperr.KindNotFound
	}
	return database.IsNotFound(err)
}
