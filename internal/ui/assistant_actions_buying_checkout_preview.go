package ui

import (
	"context"
	"fmt"
	"strconv"

	"github.com/moqasem061-dot/dawa24/internal/modules/assistant"
	"github.com/moqasem061-dot/dawa24/internal/modules/assistant/actions"
	"github.com/moqasem061-dot/dawa24/internal/modules/assistant/handles"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func (h *UIHandler) checkoutPreview(ctx context.Context, plan *checkoutPlan, branch, method string) actions.Preview {
	goods := computeCheckoutGoodsAmount(plan.Items)
	total := goods
	vendors := make([]int64, 0, len(plan.Input.VendorShippingFees))
	vendorGoods := map[int64]money.Amount{}
	for _, it := range plan.Items {
		if _, seen := vendorGoods[it.VendorOrgID]; !seen {
			vendors = append(vendors, it.VendorOrgID)
		}
		line := computeCheckoutGoodsAmount([]commerce.CheckoutLineItem{it})
		vendorGoods[it.VendorOrgID], _ = vendorGoods[it.VendorOrgID].Add(line)
	}
	p := actions.Preview{Title: "تأكيد طلب شراء"}
	for _, vendorID := range vendors {
		fee := plan.Input.VendorShippingFees[vendorID]
		total, _ = total.Add(fee)
		p.Details = append(p.Details, actions.Detail{
			Label: h.resolveOrgName(ctx, vendorID),
			Value: fmt.Sprintf("%s + توصيل %s", egp(vendorGoods[vendorID]), egp(fee)),
		})
	}
	p.Summary = fmt.Sprintf("%s من %s إلى %s — الإجمالي التقديري %s",
		countLabel(len(plan.Items), "صنف واحد", "أصناف"), countLabel(len(vendors), "مورد واحد", "موردين"), branch, egp(total))
	p.Details = append(p.Details, actions.Detail{Label: "طريقة الدفع", Value: h.paymentMethodName(ctx, method)})
	for i, it := range plan.Items {
		if i == maxPreviewLines {
			p.Details = append(p.Details, actions.Detail{Label: "…", Value: fmt.Sprintf("و %d أصناف أخرى", len(plan.Items)-maxPreviewLines)})
			break
		}
		p.Details = append(p.Details, actions.Detail{Label: it.ProductName.Get(i18n.AR), Value: "× " + strconv.Itoa(it.Quantity)})
	}
	p.Warnings = []string{"سيُرسل الطلب إلى الموردين فور التأكيد."}
	return p
}

func (h *UIHandler) paymentMethodName(ctx context.Context, id string) string {
	if h.billSvc != nil {
		if pm, err := h.billSvc.GetPlatformPaymentMethod(ctx, id); err == nil && pm != nil {
			if name := pm.Name.Get(i18n.AR); name != "" {
				return name
			}
		}
	}
	return id
}

func (h *UIHandler) productName(ctx context.Context, productID int64) string {
	if h.catSvc == nil || productID <= 0 {
		return "صنف"
	}
	prod, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), productID)
	if err != nil || prod == nil {
		return "صنف"
	}
	if name := prod.Name.Get(i18n.AR); name != "" {
		return name
	}
	return prod.Name.Get(i18n.EN)
}

func favoriteCommand(h *UIHandler, name, label string, add bool) assistantCommand {
	desc := "Add a product to the user's favourites. product is a product ref."
	if !add {
		desc = "Remove a product from the user's favourites. product is a product ref."
	}
	return assistantCommand{
		def: actions.Definition{
			Name: name, Label: label, Risk: actions.RiskLow, Description: desc,
			Params: []actions.Param{{Name: "product", Type: actions.ParamRef, RefKind: handles.KindProduct, Required: true}},
		},
		// The favourites routes are gated on the view capability.
		audience: audienceBuyer, caps: []rbac.Capability{rbac.BuyFavoriteView},
		prepare: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error) {
			if h.idSvc == nil {
				return actions.Preview{}, actions.Refuse("المفضلة غير متاحة حالياً.")
			}
			return actions.Preview{Title: label, Summary: h.productName(ctx, args.ID("product"))}, nil
		},
		execute: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Outcome, error) {
			if h.idSvc == nil {
				return actions.Outcome{}, actions.Refuse("المفضلة غير متاحة حالياً.")
			}
			product := h.productName(ctx, args.ID("product"))
			if add {
				if err := h.idSvc.AddFavorite(ctx, actor.UserID, args.ID("product")); err != nil {
					return actions.Outcome{}, err
				}
				return actions.Outcome{Message: "أُضيف " + product + " إلى المفضلة.", URL: "/favorites"}, nil
			}
			if err := h.idSvc.RemoveFavorite(ctx, actor.UserID, args.ID("product")); err != nil {
				return actions.Outcome{}, err
			}
			return actions.Outcome{Message: "حُذف " + product + " من المفضلة.", URL: "/favorites"}, nil
		},
	}
}

// FindOffers answers Capsule's find_offers: the catalogue page's query and
// availability verdicts for one of the caller's buying branches. Listings the
// catalogue would hide are not returned at all.
func (a *AssistantActions) FindOffers(ctx context.Context, actor authctx.Actor, q assistant.OfferQuery) (*assistant.OfferResult, error) {
	h := a.h
	keys := rbac.RequiredKeys(actor.DashboardScope(), rbac.BuyCatalogView)
	if !actor.IsBuyer() || !orgApproved(actor) || len(keys) == 0 || !actor.CanAny(keys...) {
		return nil, actions.ErrNotAllowed
	}
	if h.catSvc == nil {
		return nil, actions.Refuse("الكتالوج غير متاح حالياً.")
	}
	ctx = commandContext(ctx, actor)
	bctx, bactor, branch, err := h.buyingContext(ctx, actor, q.BranchID)
	if err != nil {
		return nil, err
	}
	branchID := h.buyingBranchID(bctx, &bactor)
	offers, total, verdicts, err := h.buyerOffersPage(bctx, buyerOrgID(bctx), branchID, catalog.BuyerOfferQuery{
		Query: q.Search, OnlyDiscounted: q.OnlyDiscounted, OnlyInStock: false, Limit: q.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := &assistant.OfferResult{Branch: branch, Total: total}
	for _, off := range offers {
		verdict, checked := verdicts[off.VariantID]
		if checked && verdict.IsHidden() {
			continue
		}
		item := assistant.BuyableOffer{
			VariantID: off.VariantID, ProductID: off.ProductID,
			Product: off.ProductName.Get(i18n.AR), Scientific: off.ScientificName,
			Supplier: off.VendorOrgName, Price: egp(off.Price), Discount: off.DiscountPercent,
			MinQuantity: off.MinOrderQty, Orderable: checked && verdict.IsOrderable(),
		}
		if off.PublicPrice.IsPositive() && off.PublicPrice != off.Price {
			item.PublicPrice = egp(off.PublicPrice)
		}
		if off.ExpiryDate != nil {
			item.Expiry = off.ExpiryDate.Format("2006-01-02")
		}
		if checked && !verdict.Allowed {
			item.MaxQuantity = verdict.MaxQuantity
			item.Reason = verdict.DisplayReasonAr()
		}
		if !checked {
			item.Reason = i18n.T("ar", "customer.cart.availability_check_failed")
		}
		out.Offers = append(out.Offers, item)
	}
	return out, nil
}
