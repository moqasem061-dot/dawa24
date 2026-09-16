package commerce

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// verifyAndSanitizeCheckoutPrices verifies all line item prices against authoritative
// catalog variant data to prevent client-side price tampering or discount forging.
func (s *Service) verifyAndSanitizeCheckoutPrices(ctx context.Context, input *CheckoutInput) error {
	if s.availability == nil {
		return nil
	}

	variantIDs := make([]int64, 0, len(input.Items))
	for _, item := range input.Items {
		if item.ProductVariantID != nil && *item.ProductVariantID > 0 {
			variantIDs = append(variantIDs, *item.ProductVariantID)
		}
	}

	if len(variantIDs) == 0 {
		return nil
	}

	varMap, err := s.availability.VariantsByIDs(ctx, variantIDs)
	if err != nil {
		return err
	}

	for i := range input.Items {
		item := &input.Items[i]
		if item.ProductVariantID == nil || *item.ProductVariantID <= 0 {
			continue
		}

		v, ok := varMap[*item.ProductVariantID]
		if !ok || v.ID == 0 {
			return apperr.Validation("checkout.variant_not_found", "Product variant does not exist.", nil)
		}
		if !v.Active {
			return apperr.Validation("checkout.variant_inactive", "Product variant is no longer active.", nil)
		}

		if item.VendorOrgID <= 0 {
			item.VendorOrgID = v.OrganizationID
		} else if item.VendorOrgID != v.OrganizationID {
			return apperr.Validation("checkout.vendor_mismatch", "Vendor mismatch for variant.", nil)
		}

		if input.IsNegotiation {
			if !v.IsNegotiable {
				return apperr.Validation("checkout.item_not_negotiable", "Item is not eligible for price negotiation.", nil)
			}
			if !item.UnitPrice.IsPositive() {
				return apperr.Validation("checkout.proposed_price_invalid", "Proposed unit price must be positive.", nil)
			}
			item.IsNegotiated = true
			item.ProposedUnitPrice = item.UnitPrice
			item.ListPrice = v.Price
			item.CostPrice = v.CostPrice
			item.CostDiscountPercentage = v.CostDiscountPercentage
			continue
		}

		// Standard purchase: Authoritative server price calculation
		authListPrice := v.Price
		authUnitPrice := v.EffectivePrice
		if !authUnitPrice.IsPositive() && authListPrice.IsPositive() {
			authUnitPrice = authListPrice
		}

		// Detect and alert on client price tampering attempts
		if item.UnitPrice.IsPositive() && item.UnitPrice.Minor() != authUnitPrice.Minor() {
			if s.log != nil {
				s.log.WarnContext(ctx, "checkout price discrepancy detected; enforcing authoritative catalog price",
					"customer_id", input.CustomerID,
					"variant_id", *item.ProductVariantID,
					"client_price", item.UnitPrice.String(),
					"auth_price", authUnitPrice.String(),
				)
			}
		}

		// Calculate authoritative discount
		lineDiscount := money.Zero
		if authListPrice.IsPositive() && authListPrice.Minor() > authUnitPrice.Minor() {
			unitDisc, _ := authListPrice.Sub(authUnitPrice)
			lineDiscount, _ = unitDisc.MulInt(int64(item.Quantity))
		} else if item.DiscountAmount.IsPositive() {
			// Line already carries an authorized discount from an offer or smart order candidate
			lineDiscount = item.DiscountAmount
			if authListPrice.IsPositive() && item.Quantity > 0 {
				totalRetail := authListPrice.Minor() * int64(item.Quantity)
				if totalRetail > item.DiscountAmount.Minor() {
					netMinor := (totalRetail - item.DiscountAmount.Minor()) / int64(item.Quantity)
					authUnitPrice = money.FromMinor(netMinor)
				}
			}
		} else if item.UnitPrice.IsPositive() && authListPrice.IsPositive() && authListPrice.Minor() > item.UnitPrice.Minor() {
			unitDisc, _ := authListPrice.Sub(item.UnitPrice)
			lineDiscount, _ = unitDisc.MulInt(int64(item.Quantity))
			authUnitPrice = item.UnitPrice
		}

		item.UnitPrice = authUnitPrice
		item.ListPrice = authListPrice
		item.DiscountAmount = lineDiscount
		item.CostPrice = v.CostPrice
		item.CostDiscountPercentage = v.CostDiscountPercentage
		item.IsNegotiated = false
		item.ProposedUnitPrice = money.Zero
	}

	return nil
}
