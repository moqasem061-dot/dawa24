package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// validateSpecialOfferForCheckout refuses an offer bundle that must not be
// sold: withdrawn by the vendor, rejected/pending with the platform admins,
// or outside its date window. Empty means the offer may be checked out.
func validateSpecialOfferForCheckout(spo *promo.SpecialOffer) string {
	if spo == nil {
		return "العرض المطلوب غير موجود."
	}
	if spo.Status == "inactive" || spo.Status == "draft" {
		return "هذا العرض موقوف حالياً من المورد ولا يمكن إتمام الطلب عليه."
	}
	if spo.Status == "expired" {
		return "انتهت صلاحية هذا العرض ولا يمكن إتمام الطلب عليه."
	}
	if spo.AdminStatus == "pending" {
		return "هذا العرض قيد مراجعة إدارة المنصة ولم يُعتمد بعد."
	}
	if spo.AdminStatus == "rejected" {
		return "تم رفض هذا العرض من إدارة المنصة ولا يمكن إتمام الطلب عليه."
	}
	now := time.Now()
	if spo.StartDate != nil && now.Before(*spo.StartDate) {
		return "هذا العرض لم يبدأ بعد ولا يمكن إتمام الطلب عليه."
	}
	if spo.EndDate != nil && now.After(*spo.EndDate) {
		return "انتهت صلاحية هذا العرض ولا يمكن إتمام الطلب عليه."
	}
	return ""
}

// checkoutValidationMessage maps checkout validation codes to specific Arabic
// messages. It reports false for non-validation errors, which keep the
// generic renderError path.
func checkoutValidationMessage(lang string, err error) (string, bool) {
	_ = lang
	ae, ok := apperr.As(err)
	if !ok || ae == nil || ae.Kind != apperr.KindValidation {
		return "", false
	}
	switch {
	case ae.Code == "checkout.min_order_not_met":
		total := ""
		min := ""
		if ae.Fields != nil {
			total = ae.Fields["order_total"]
			min = ae.Fields["min_order_total"]
		}
		if total != "" && min != "" {
			return fmt.Sprintf("إجمالي الطلب (%s ج.م) أقل من الحد الأدنى المطلوب للعرض (%s ج.م). أضف أصنافاً أو ارفع الكمية ثم أعد المحاولة.", total, min), true
		}
		return "إجمالي الطلب أقل من الحد الأدنى المطلوب لهذا العرض. أضف أصنافاً أو ارفع الكمية ثم أعد المحاولة.", true
	case ae.Code == "item.vendor_required":
		return "تعذر تحديد المورد لأحد سطور السلة. احذف السطر وأعد إضافته من صفحة العرض، ثم أعد المحاولة.", true
	case ae.Code == "checkout.empty_cart":
		return "سلة المشتريات فارغة. أضف أصنافاً أولاً.", true
	case ae.Code == "item.quantity_invalid":
		return "كمية غير صالحة في أحد سطور السلة. راجع الكميات ثم أعد المحاولة.", true
	case strings.HasPrefix(ae.Code, "checkout.line_unavailable."):
		// Availability refusals already carry the specific Arabic reason.
		if ae.Msg != "" {
			return ae.Msg, true
		}
		return "أحد الأصناف غير متاح حالياً (نفد المخزون أو خارج التغطية). راجع السلة ثم أعد المحاولة.", true
	default:
		// Every other validation refusal already carries a message the domain
		// wrote for a person to read.
		if ae.Msg != "" {
			return ae.Msg, true
		}
		return "", false
	}
}

// filterCheckoutBranches returns active branches for the checkout page, filtering out
// foreign or inactive branches, and respecting owner vs staff assignment.
func filterCheckoutBranches(bList []*org.Branch, actor authctx.Actor) []*org.Branch {
	var branches []*org.Branch
	for _, b := range bList {
		if b == nil || b.OrganizationID != actor.OrganizationID || b.Status == "inactive" || b.Status == "suspended" {
			continue
		}
		if !actor.IsOwner && actor.BranchID != nil && *actor.BranchID > 0 && b.ID != *actor.BranchID {
			continue
		}
		branches = append(branches, b)
	}
	if len(branches) == 0 && !actor.IsOwner && actor.BranchID != nil && *actor.BranchID > 0 {
		for _, b := range bList {
			if b != nil && b.OrganizationID == actor.OrganizationID && b.Status != "inactive" && b.Status != "suspended" {
				branches = append(branches, b)
			}
		}
	}
	return branches
}

// resolveCheckoutBranch determines and validates the receiving branch for checkout.
// The shell selection is authoritative; a form value is only a legacy fallback
// for callers that did not pass through the buying-branch middleware.
func (h *UIHandler) resolveCheckoutBranch(ctx context.Context, actor authctx.Actor, formBranchID string) *int64 {
	var branchID *int64
	if buying, ok := authctx.BuyingBranchFrom(ctx); ok {
		if buying.Active != nil && *buying.Active > 0 {
			branchID = buying.Active
		} else {
			return nil
		}
	} else if bID, err := strconv.ParseInt(formBranchID, 10, 64); err == nil && bID > 0 {
		branchID = &bID
	} else if actor.BranchID != nil && *actor.BranchID > 0 {
		branchID = actor.BranchID
	}

	if branchID != nil && h.orgSvc != nil {
		b, err := h.orgSvc.GetBranch(ctx, *branchID)
		if err != nil || b == nil || b.OrganizationID != actor.OrganizationID || b.Status == "inactive" || b.Status == "suspended" {
			branchID = nil
		}
	}

	if branchID == nil && actor.OrganizationID > 0 {
		targetID := h.buyingBranchID(ctx, &actor)
		if targetID > 0 {
			branchID = &targetID
		}
	}

	return branchID
}

// vendorFulfillingBranch picks the branch a vendor ships from: their main
// branch, or their first if none is marked main. nil means the vendor has no
// branches and the order-level branch stands.
func (h *UIHandler) vendorFulfillingBranch(ctx context.Context, vendorOrgID int64) *int64 {
	if h.orgSvc == nil || vendorOrgID <= 0 {
		return nil
	}
	branches, err := h.orgSvc.ListBranches(ctx, vendorOrgID)
	if err != nil || len(branches) == 0 {
		return nil
	}
	for _, b := range branches {
		if b.IsMain {
			id := b.ID
			return &id
		}
	}
	id := branches[0].ID
	return &id
}

// prepareCheckoutItems resolves prices, discounts, vendor organizations, and offer metadata for cart items.
func (h *UIHandler) prepareCheckoutItems(ctx context.Context, cart *commerce.Cart) ([]commerce.CheckoutLineItem, int64) {
	var items []commerce.CheckoutLineItem
	var offerID int64
	for _, it := range cart.Items {
		pID := it.ProductID
		vID := it.ProductVariantID
		vOrgID := it.OrganizationID
		var listPrice, discAmount, variantDiscount money.Amount
		var costDiscPct float64

		if h.catSvc != nil && pID > 0 {
			if prod, variants, err := h.catSvc.GetProduct(ctx, pID); err == nil && prod != nil {
				if prod.Price.IsPositive() {
					listPrice = prod.Price
				}
				for _, v := range variants {
					if v != nil && v.ID == vID {
						if v.OrganizationID > 0 && vOrgID <= 0 {
							vOrgID = v.OrganizationID
						}
						if v.Price.IsPositive() {
							listPrice = v.Price
						}
						if v.CostDiscountPercentage > 0 {
							costDiscPct = v.CostDiscountPercentage
						}
						if v.Discount.IsPositive() {
							variantDiscount = v.Discount
						}
						break
					}
				}
				if vOrgID <= 0 && prod.OrganizationID > 0 {
					vOrgID = prod.OrganizationID
				}
			}
		}
		if vOrgID <= 0 && it.OfferID != nil && *it.OfferID > 0 && h.promoSvc != nil {
			if spo, serr := h.promoSvc.GetSpecialOffer(ctx, *it.OfferID); serr == nil && spo != nil && spo.OrganizationID > 0 {
				vOrgID = spo.OrganizationID
				if spo.DiscountPercentage > 0 {
					costDiscPct = spo.DiscountPercentage
					variantDiscount = money.FromMinor(int64(spo.DiscountPercentage * 100))
				}
			} else if offer, oerr := h.promoSvc.GetOffer(ctx, *it.OfferID); oerr == nil && offer != nil && offer.OrganizationID > 0 {
				vOrgID = offer.OrganizationID
				if offer.DiscountValue.IsPositive() {
					costDiscPct = float64(offer.DiscountValue.Minor()) / 100.0
					variantDiscount = offer.DiscountValue
				}
			}
		}
		uPrice := it.UnitPrice
		if listPrice.IsZero() {
			listPrice = uPrice
		}
		netUnitPrice := listPrice
		if variantDiscount.IsPositive() && variantDiscount.Minor() > 0 && variantDiscount.Minor() < 10000 {
			netUnitPrice = listPrice.ApplyPercent(10000 - variantDiscount.Minor())
		}
		if listPrice.Minor() > netUnitPrice.Minor() {
			discAmount = money.FromMinor((listPrice.Minor() - netUnitPrice.Minor()) * int64(it.Quantity))
		}
		pName := it.ProductName
		if len(pName) == 0 {
			pName = i18n.Text{"ar": i18n.TDefault("w4_ui.s_67_67"), "en": "Certified Medicine"}
		}
		var pIDPtr, vIDPtr *int64
		if pID > 0 {
			pIDPtr = &pID
		}
		if vID > 0 {
			vIDPtr = &vID
		}
		items = append(items, commerce.CheckoutLineItem{
			VendorOrgID:            vOrgID,
			ProductID:              pIDPtr,
			ProductVariantID:       vIDPtr,
			ProductName:            pName,
			OfferProductID:         it.OfferID,
			Quantity:               it.Quantity,
			UnitPrice:              netUnitPrice,
			ListPrice:              listPrice,
			OriginalPrice:          listPrice,
			OriginalDiscount:       variantDiscount,
			DiscountAmount:         discAmount,
			CostDiscountPercentage: costDiscPct,
		})
		if it.OfferID != nil {
			if offerID == 0 {
				offerID = *it.OfferID
			} else if offerID != *it.OfferID {
				offerID = 0
			}
		}
	}
	return items, offerID
}
