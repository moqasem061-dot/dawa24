package ui

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Capsule's view of العروض والخصومات: the same offer rule and the same cart
// line the offers board and its add-to-cart button use, for pharmacies and for
// suppliers that buy.

// promotionBuyer applies the offers screen's route chain and resolves the
// buying branch, returning the context, the offer query and the branch name.
func (a *AssistantActions) promotionBuyer(ctx context.Context, actor authctx.Actor, branchID int64) (context.Context, offerBuyer, string, error) {
	keys := rbac.RequiredKeys(actor.DashboardScope(), rbac.BuyOfferView)
	if !actor.IsBuyer() || !orgApproved(actor) || len(keys) == 0 || !actor.CanAny(keys...) {
		return ctx, offerBuyer{}, "", actions.ErrNotAllowed
	}
	if a.h.promoSvc == nil {
		return ctx, offerBuyer{}, "", actions.Refuse("العروض غير متاحة حالياً.")
	}
	ctx = commandContext(ctx, actor)
	bctx, bactor, branch, err := a.h.buyingContext(ctx, actor, branchID)
	if err != nil {
		return ctx, offerBuyer{}, "", err
	}
	return bctx, a.h.buyerOfferQuery(bctx, bactor, *bactor.BranchID), branch, nil
}

// FindPromotions lists the promotions the buying branch can buy today.
func (a *AssistantActions) FindPromotions(ctx context.Context, actor authctx.Actor, q assistant.PromotionQuery) (*assistant.PromotionResult, error) {
	bctx, buyer, branch, err := a.promotionBuyer(ctx, actor, q.BranchID)
	if err != nil {
		return nil, err
	}
	out := &assistant.PromotionResult{Branch: branch, Notice: buyer.Notice}
	if buyer.Notice != "" {
		return out, nil
	}
	query := buyer.Query
	query.Search, query.Discounts, query.Limit = q.Search, q.OnlyDiscounted, q.Limit
	query.Sort = promo.SortOffersDiscountDesc
	offers, total, err := a.h.promoSvc.ListBuyerOffers(bctx, query)
	if err != nil {
		return nil, err
	}
	out.Total = total
	for _, o := range offers {
		out.Promotions = append(out.Promotions, promotionOf(o))
	}
	return out, nil
}

// PromotionDetail opens one promotion with its bundle and the verdict for the
// buying branch. A promotion the buyer may not see at all is not found.
func (a *AssistantActions) PromotionDetail(ctx context.Context, actor authctx.Actor, offerID, branchID int64) (*assistant.PromotionDetail, error) {
	bctx, buyer, _, err := a.promotionBuyer(ctx, actor, branchID)
	if err != nil {
		return nil, err
	}
	q := buyer.Query
	q.OfferID = offerID
	verdict, err := a.h.promoSvc.OfferVerdict(bctx, q)
	if err != nil {
		return nil, err
	}
	if verdict.Reason(false) != promo.OfferOK {
		return nil, actions.Refuse("%s", offerRefusal(verdict.Reason(false)))
	}
	sp, err := a.h.promoSvc.GetSpecialOffer(bctx, offerID)
	if err != nil || sp == nil {
		return nil, actions.Refuse("العرض المطلوب غير موجود.")
	}

	d := &assistant.PromotionDetail{
		Promotion: assistant.Promotion{
			OfferID: sp.ID, Title: sp.Title.Get(i18n.AR), Supplier: sp.OrganizationName,
			Products: len(sp.Products),
		},
		Description: sp.Description.Get(i18n.AR),
	}
	if sp.DiscountPercentage > 0 {
		d.Discount = fmt.Sprintf("%.0f%%", sp.DiscountPercentage)
	}
	if price := bundlePrice(sp); price.IsPositive() {
		d.BundlePrice = egp(price)
	}
	if sp.EndDate != nil {
		d.Expires = sp.EndDate.Format("2006-01-02")
	}
	for _, p := range sp.Products {
		item := assistant.PromotionProduct{Product: p.VariantName, Quantity: p.Quantity}
		if p.CustomPrice.IsPositive() {
			item.Price = egp(p.CustomPrice)
		}
		d.Items = append(d.Items, item)
	}
	switch {
	case buyer.Notice != "":
		d.Reason = buyer.Notice
	default:
		d.Reason = offerRefusal(verdict.Reason(true))
	}
	d.Purchasable = d.Reason == ""
	return d, nil
}

func promotionOf(o *promo.BuyerOffer) assistant.Promotion {
	p := assistant.Promotion{
		OfferID: o.ID, Title: o.Title.Get(i18n.AR), Supplier: o.SupplierName(i18n.AR),
		Products: o.ProductCount, Sponsored: o.Sponsored,
	}
	if desc := o.Description.Get(i18n.AR); desc != "" {
		p.Description = desc
	}
	if pct := o.DiscountPercentage(); pct > 0 {
		p.Discount = fmt.Sprintf("%.0f%%", pct)
	} else if o.DiscountValue.IsPositive() {
		p.Discount = egp(o.DiscountValue)
	}
	if o.TotalPrice.IsPositive() {
		p.BundlePrice = egp(o.TotalPrice)
	}
	if o.MinOrderAmount.IsPositive() {
		p.MinOrder = egp(o.MinOrderAmount)
	}
	if o.ExpiresAt != nil {
		p.Expires = o.ExpiresAt.Format("2006-01-02")
	}
	return p
}

// offerAddCommand puts a promotional bundle in the cart: the offers board's
// add button, through the same cart line and the same offer rule.
func (h *UIHandler) offerAddCommand() assistantCommand {
	plan := func(ctx context.Context, actor authctx.Actor, args actions.Args) (context.Context, *offerLine, error) {
		ctx = commandContext(ctx, actor)
		bctx, bactor, branch, err := h.buyingContext(ctx, actor, 0)
		if err != nil {
			return ctx, nil, err
		}
		offerID := args.ID("offer")
		item, title, err := h.offerCartItem(bctx, offerID, int(args.Int("quantity")))
		if err != nil {
			return bctx, nil, err
		}
		if ok, reason := h.offerPurchasable(bctx, bactor, offerID, *bactor.BranchID); !ok {
			return bctx, nil, refuseMessage(reason)
		}
		return bctx, &offerLine{item: item, title: title, branch: branch}, nil
	}
	return assistantCommand{
		def: actions.Definition{
			Name: "offer_add", Label: "إضافة عرض إلى السلة", Risk: actions.RiskLow,
			Description: "Add a promotional bundle (عرض) to the user's cart. offer is a ref from list_promotions; quantity is the number of bundles.",
			Params: []actions.Param{
				{Name: "offer", Type: actions.ParamRef, RefKind: handles.KindOffer, Required: true},
				{Name: "quantity", Type: actions.ParamInt, Required: true, Min: 1, Max: maxCartQuantity},
			},
		},
		audience: audienceBuyer, caps: []rbac.Capability{rbac.BuyCartUse},
		prepare: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error) {
			_, line, err := plan(ctx, actor, args)
			if err != nil {
				return actions.Preview{}, err
			}
			total, err := line.item.UnitPrice.MulInt(int64(line.item.Quantity))
			if err != nil {
				return actions.Preview{}, refuseMessage("الكمية كبيرة جداً.")
			}
			return actions.Preview{
				Title:   "إضافة عرض إلى السلة",
				Summary: fmt.Sprintf("%d × %s", line.item.Quantity, line.title),
				Details: []actions.Detail{
					{Label: "سعر الحزمة", Value: egp(line.item.UnitPrice)},
					{Label: "الإجمالي", Value: egp(total)},
					{Label: "فرع الاستلام", Value: line.branch},
				},
			}, nil
		},
		execute: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Outcome, error) {
			bctx, line, err := plan(ctx, actor, args)
			if err != nil {
				return actions.Outcome{}, err
			}
			if h.commSvc == nil {
				return actions.Outcome{}, actions.Refuse("السلة غير متاحة حالياً.")
			}
			if _, err := h.commSvc.AddToCart(bctx, actor.UserID, buyerOrgID(bctx), line.item); err != nil {
				return actions.Outcome{}, refuseMessage(h.safeMessage(err, "ar"))
			}
			return actions.Outcome{Message: fmt.Sprintf("أُضيف %d × %s إلى السلة.", line.item.Quantity, line.title), URL: "/cart"}, nil
		},
	}
}

// offerLine is a planned bundle cart line.
type offerLine struct {
	item   *commerce.CartItem
	title  string
	branch string
}
