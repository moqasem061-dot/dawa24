package main

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
	"github.com/moqasem061-dot/dawa24/internal/modules/workflow"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/timeutil"
)

type apiQuoteResolver struct {
	catalog  *catalog.Service
	org      *org.Service
	promo    *promo.Service
	coverage *workflow.CoverageService
	// paymentMethod and addresses back checkBuyerChoices.
	paymentMethod func(context.Context, string) (*billing.PlatformPaymentMethod, error)
	addresses     func(context.Context, int64) ([]*identity.UserAddress, error)
}

func (q *apiQuoteResolver) quote(ctx context.Context, requested commerce.CheckoutInput) (commerce.CheckoutInput, error) {
	if requested.OfferID < 0 {
		return commerce.CheckoutInput{}, quoteUnavailable()
	}
	if requested.CustomerOrgID <= 0 || requested.BranchID == nil || *requested.BranchID <= 0 {
		return commerce.CheckoutInput{}, apperr.Validation("checkout.branch_required", "يرجى اختيار فرع الاستلام. Please select a receiving branch.", nil)
	}
	sys := database.AsSystem(ctx)
	branch, err := q.org.GetBranch(sys, *requested.BranchID)
	if err != nil {
		return commerce.CheckoutInput{}, err
	}
	if branch == nil || branch.OrganizationID != requested.CustomerOrgID || branch.Status == "inactive" || branch.Status == "suspended" {
		return commerce.CheckoutInput{}, apperr.Forbidden("checkout.branch_invalid", "فرع الاستلام غير متاح. Receiving branch is unavailable.")
	}
	if err := q.checkBuyerChoices(ctx, &requested); err != nil {
		return commerce.CheckoutInput{}, err
	}
	requested.PaymentStatus = commerce.PaymentUnpaid
	requested.ShippingFee = money.Zero
	requested.TaxAmount = money.Zero
	requested.MinOrderAmount = money.Zero
	requested.VendorShippingFees = make(map[int64]money.Amount)
	requested.VendorBranchIDs = make(map[int64]*int64)
	requested.VendorBranchID = nil
	offerTotals := map[int64]money.Amount{}
	offerMinimums := map[int64]money.Amount{}
	offerQuantities := map[int64]int{}
	offerLineMinimums := map[int64]int{}
	for i := range requested.Items {
		submitted := requested.Items[i]
		resolved, err := q.quoteLine(ctx, requested, submitted)
		if err != nil {
			return commerce.CheckoutInput{}, err
		}
		variant, selected, line := resolved.variant, resolved.offer, resolved.line
		if selected != nil {
			offerQuantities[selected.Product.ID] += submitted.Quantity
			offerLineMinimums[selected.Product.ID] = selected.Product.CustomQty
			if selected.Product.MaxQtyPerOrder != nil && offerQuantities[selected.Product.ID] > *selected.Product.MaxQtyPerOrder {
				return commerce.CheckoutInput{}, quoteUnavailable()
			}
			lineTotal, err := line.UnitPrice.MulInt(int64(submitted.Quantity))
			if err != nil {
				return commerce.CheckoutInput{}, err
			}
			offerTotals[selected.Offer.ID], err = offerTotals[selected.Offer.ID].Add(lineTotal)
			if err != nil {
				return commerce.CheckoutInput{}, err
			}
			offerMinimums[selected.Offer.ID] = selected.Offer.MinOrderAmount
		}
		requested.Items[i] = line
		if _, ok := requested.VendorShippingFees[variant.OrganizationID]; !ok {
			delivery, fallbackBranch, err := q.deliveryQuote(sys, variant.OrganizationID, branch)
			if err != nil {
				return commerce.CheckoutInput{}, err
			}
			if delivery.Fee.IsNegative() {
				return commerce.CheckoutInput{}, quoteUnavailable()
			}
			requested.VendorShippingFees[variant.OrganizationID] = delivery.Fee
			requested.ShippingFee, err = requested.ShippingFee.Add(delivery.Fee)
			if err != nil {
				return commerce.CheckoutInput{}, err
			}
			requested.VendorBranchIDs[variant.OrganizationID] = variant.BranchID
			if variant.BranchID == nil {
				requested.VendorBranchIDs[variant.OrganizationID] = fallbackBranch
			}
		} else if variant.BranchID != nil && requested.VendorBranchIDs[variant.OrganizationID] != nil && *variant.BranchID != *requested.VendorBranchIDs[variant.OrganizationID] {
			return commerce.CheckoutInput{}, quoteUnavailable()
		}
	}
	for lineID, minimum := range offerLineMinimums {
		if offerQuantities[lineID] < minimum {
			return commerce.CheckoutInput{}, quoteUnavailable()
		}
	}
	for offerID, minimum := range offerMinimums {
		if offerTotals[offerID].Minor() < minimum.Minor() {
			return commerce.CheckoutInput{}, apperr.Validation("checkout.min_order_not_met", "لم يصل الطلب إلى الحد الأدنى للعرض. Order total is below the offer minimum.", nil)
		}
	}
	return requested, nil
}

// deliveryQuote keeps the existing pricing rule while propagating branch lookup errors.
func (q *apiQuoteResolver) deliveryQuote(ctx context.Context, vendorID int64, destination *org.Branch) (org.DeliveryQuote, *int64, error) {
	bands, err := q.org.GetDeliveryBands(ctx, vendorID)
	if err != nil {
		return org.DeliveryQuote{}, nil, err
	}
	branches, err := q.org.ListBranches(ctx, vendorID)
	if err != nil {
		return org.DeliveryQuote{}, nil, err
	}
	var origin *org.Branch
	var fulfilling *org.Branch
	for _, branch := range branches {
		if branch != nil && fulfilling == nil {
			fulfilling = branch
		}
		if branch != nil && branch.IsMain {
			fulfilling = branch
		}
		if !hasCoordinates(branch) {
			continue
		}
		if origin == nil {
			origin = branch
		}
		if branch.IsMain {
			origin = branch
			break
		}
	}
	distance := org.UnknownDistance
	if hasCoordinates(origin) && hasCoordinates(destination) {
		distance = org.HaversineMeters(*origin.Latitude, *origin.Longitude, *destination.Latitude, *destination.Longitude)
	}
	var fulfillingID *int64
	if fulfilling != nil {
		fulfillingID = &fulfilling.ID
	}
	return org.QuoteDelivery(bands, distance), fulfillingID, nil
}

func hasCoordinates(branch *org.Branch) bool {
	return branch != nil && branch.Latitude != nil && branch.Longitude != nil && (*branch.Latitude != 0 || *branch.Longitude != 0)
}

func quoteUnavailable() error {
	return apperr.Conflict("checkout.unverifiable_on_website", "تعذر التحقق من المنتج أو العرض. يرجى الطلب من الموقع. This product or offer cannot be verified; please order on the website.")
}

func (q *apiQuoteResolver) offerForLine(ctx context.Context, request commerce.CheckoutInput, submitted commerce.CheckoutLineItem, variant *catalog.ProductVariant) (*promo.OfferProductWithOffer, error) {
	rows, err := q.promo.ListOffersForProduct(database.AsSystem(ctx), variant.ProductID)
	if err != nil {
		return nil, err
	}
	var match *promo.OfferProductWithOffer
	for _, row := range rows {
		if row == nil || row.Offer == nil || row.Product == nil || row.Offer.OrganizationID != variant.OrganizationID {
			continue
		}
		if row.Product.VariantID != nil && *row.Product.VariantID != variant.ID {
			continue
		}
		if request.OfferID > 0 && row.Offer.ID != request.OfferID {
			continue
		}
		if submitted.OfferProductID != nil && *submitted.OfferProductID != row.Product.ID && *submitted.OfferProductID != row.Offer.ID {
			continue
		}
		if match != nil {
			return nil, quoteUnavailable()
		}
		match = row
	}
	if match == nil {
		return nil, quoteUnavailable()
	}
	buyer, err := q.buyerOfferQuery(ctx, request)
	if err != nil {
		return nil, err
	}
	buyer.OfferID = match.Offer.ID
	verdict, err := q.promo.OfferVerdict(database.AsSystem(ctx), buyer)
	if err != nil {
		return nil, err
	}
	if verdict.Reason(true) != promo.OfferOK {
		return nil, quoteUnavailable()
	}
	return match, nil
}

func (q *apiQuoteResolver) buyerOfferQuery(ctx context.Context, request commerce.CheckoutInput) (promo.BuyerOfferQuery, error) {
	branch, err := q.org.GetBranch(database.AsSystem(ctx), *request.BranchID)
	if err != nil {
		return promo.BuyerOfferQuery{}, err
	}
	if branch == nil || branch.OrganizationID != request.CustomerOrgID {
		return promo.BuyerOfferQuery{}, quoteUnavailable()
	}
	coord := workflow.Coord{CityID: branch.CityID}
	if branch.Latitude != nil {
		coord.Lat = *branch.Latitude
	}
	if branch.Longitude != nil {
		coord.Lon = *branch.Longitude
	}
	if (branch.Latitude == nil || branch.Longitude == nil) && (branch.CityID == nil || *branch.CityID <= 0) {
		return promo.BuyerOfferQuery{}, quoteUnavailable()
	}
	works, err := q.org.ConnectedWorkIDsForBranch(database.AsSystem(ctx), branch.ID)
	if err != nil {
		return promo.BuyerOfferQuery{}, err
	}
	day := timeutil.Now().Weekday()
	coverage, err := q.coverage.VendorBranchesServing(ctx, day, coord)
	if err != nil {
		return promo.BuyerOfferQuery{}, err
	}
	b := promo.BuyerBranch{ID: branch.ID, Lat: coord.Lat, Lon: coord.Lon, HasCoords: branch.Latitude != nil && branch.Longitude != nil && (coord.Lat != 0 || coord.Lon != 0), Weekday: day, AllowedWorkIDs: works}
	if branch.CityID != nil {
		b.CityID = *branch.CityID
	}
	out := promo.BuyerOfferQuery{BuyerOrgID: request.CustomerOrgID, Buying: true, Branch: b}
	seen := map[int64]bool{}
	wide := map[int64]bool{}
	for _, row := range coverage {
		if row.OrganizationID <= 0 {
			continue
		}
		if !seen[row.OrganizationID] {
			out.Coverage.OrgIDs = append(out.Coverage.OrgIDs, row.OrganizationID)
			seen[row.OrganizationID] = true
		}
		if row.BranchID > 0 {
			out.Coverage.BranchIDs = append(out.Coverage.BranchIDs, row.BranchID)
		} else if !wide[row.OrganizationID] {
			out.Coverage.OrgWideIDs = append(out.Coverage.OrgWideIDs, row.OrganizationID)
			wide[row.OrganizationID] = true
		}
	}
	return out, nil
}
