package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/assistant/actions"
	"github.com/moqasem061-dot/dawa24/internal/modules/assistant/datasets"
	"github.com/moqasem061-dot/dawa24/internal/modules/assistant/handles"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// Buying commands: the cart, the checkout, cancelling an order and the
// favourites list, for pharmacies and for suppliers that buy.

const maxCartQuantity = 100000

func (h *UIHandler) buyingCommands() []assistantCommand {
	return []assistantCommand{
		h.offerAddCommand(),
		{
			def: actions.Definition{
				Name: "cart_add", Label: "إضافة إلى السلة", Risk: actions.RiskLow,
				Description: "Add a supplier listing to the user's cart. offer is a ref from find_offers.",
				Params: []actions.Param{
					{Name: "offer", Type: actions.ParamRef, RefKind: handles.KindVariant, Required: true},
					{Name: "quantity", Type: actions.ParamInt, Required: true, Min: 1, Max: maxCartQuantity},
				},
			},
			audience: audienceBuyer, caps: []rbac.Capability{rbac.BuyCartUse},
			prepare: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error) {
				line, err := h.planCartAdd(ctx, actor, args.ID("offer"), int(args.Int("quantity")))
				if err != nil {
					return actions.Preview{}, err
				}
				return line.preview("إضافة إلى السلة"), nil
			},
			execute: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Outcome, error) {
				line, err := h.planCartAdd(ctx, actor, args.ID("offer"), int(args.Int("quantity")))
				if err != nil {
					return actions.Outcome{}, err
				}
				if _, err := h.commSvc.AddToCart(line.ctx, actor.UserID, buyerOrgID(line.ctx), line.item); err != nil {
					return actions.Outcome{}, refuseMessage(h.safeMessage(err, "ar"))
				}
				return actions.Outcome{Message: fmt.Sprintf("أُضيف %d × %s إلى السلة.", line.item.Quantity, line.product), URL: "/cart"}, nil
			},
		},
		{
			def: actions.Definition{
				Name: "cart_set_quantity", Label: "تعديل كمية في السلة", Risk: actions.RiskLow,
				Description: "Change the quantity of a cart line; 0 removes it. line is a ref from the cart_items dataset.",
				Params: []actions.Param{
					{Name: "line", Type: actions.ParamRef, RefKind: datasets.KindCartLine, Required: true},
					{Name: "quantity", Type: actions.ParamInt, Required: true, Min: 0, Max: maxCartQuantity},
				},
			},
			audience: audienceBuyer, caps: []rbac.Capability{rbac.BuyCartUse},
			prepare: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error) {
				plan, err := h.planCartQuantity(ctx, actor, args.ID("line"), int(args.Int("quantity")))
				if err != nil {
					return actions.Preview{}, err
				}
				return plan.preview(), nil
			},
			execute: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Outcome, error) {
				plan, err := h.planCartQuantity(ctx, actor, args.ID("line"), int(args.Int("quantity")))
				if err != nil {
					return actions.Outcome{}, err
				}
				return plan.apply()
			},
		},
		{
			def: actions.Definition{
				Name: "cart_remove", Label: "حذف من السلة", Risk: actions.RiskLow,
				Description: "Remove a line from the cart. line is a ref from the cart_items dataset.",
				Params: []actions.Param{
					{Name: "line", Type: actions.ParamRef, RefKind: datasets.KindCartLine, Required: true},
				},
			},
			audience: audienceBuyer, caps: []rbac.Capability{rbac.BuyCartUse},
			prepare: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error) {
				plan, err := h.planCartQuantity(ctx, actor, args.ID("line"), 0)
				if err != nil {
					return actions.Preview{}, err
				}
				return plan.preview(), nil
			},
			execute: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Outcome, error) {
				plan, err := h.planCartQuantity(ctx, actor, args.ID("line"), 0)
				if err != nil {
					return actions.Outcome{}, err
				}
				return plan.apply()
			},
		},
		{
			def: actions.Definition{
				Name: "place_order", Label: "إرسال طلب الشراء", Risk: actions.RiskHigh,
				Description: "Place the current cart as an order. branch is a branch ref (omit for the user's buying branch); " +
					"payment_method is an active checkout method id, default cod. Wallet payment is not available through the assistant.",
				Params: []actions.Param{
					{Name: "branch", Type: actions.ParamRef, RefKind: handles.KindBranch},
					{Name: "payment_method", Type: actions.ParamString, MaxLen: 40},
					{Name: "notes", Type: actions.ParamString, MaxLen: 500},
				},
			},
			audience: audienceBuyer, caps: []rbac.Capability{rbac.BuyOrderCreate},
			prepare: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error) {
				plan, err := h.planAssistantCheckout(ctx, actor, args)
				if err != nil {
					return actions.Preview{}, err
				}
				return plan.preview, nil
			},
			execute: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Outcome, error) {
				plan, err := h.planAssistantCheckout(ctx, actor, args)
				if err != nil {
					return actions.Outcome{}, err
				}
				order, failure := h.placeCheckout(plan.ctx, plan.actor, "ar", plan.checkout)
				if failure != nil {
					return actions.Outcome{}, checkoutRefusal(failure)
				}
				return actions.Outcome{
					Message: fmt.Sprintf("تم إرسال طلب الشراء رقم %s.", order.OrderNumber),
					URL:     "/orders/" + strconv.FormatInt(order.ID, 10),
				}, nil
			},
		},
		{
			def: actions.Definition{
				Name: "order_cancel", Label: "إلغاء طلب شراء", Risk: actions.RiskHigh,
				Description: "Cancel one of the user's purchase orders before any shipment leaves. order is a ref from purchase_orders.",
				Params: []actions.Param{
					{Name: "order", Type: actions.ParamRef, RefKind: handles.KindOrder, Required: true},
					{Name: "reason", Type: actions.ParamString, MaxLen: 300},
				},
			},
			audience: audienceBuyer, caps: []rbac.Capability{rbac.BuyOrderUpdate},
			prepare: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error) {
				order, failure := h.cancellableBuyerOrder(ctx, actor, "ar", args.ID("order"))
				if failure != nil {
					return actions.Preview{}, refuseMessage(failure.Message)
				}
				p := actions.Preview{
					Title:   "إلغاء طلب الشراء " + order.OrderNumber,
					Summary: fmt.Sprintf("الإجمالي %s — الحالة الحالية: %s", egp(order.TotalAmount), string(order.Status)),
					Details: []actions.Detail{{Label: "الشحنات", Value: strconv.Itoa(len(order.Shipments))}},
				}
				if reason := args.Str("reason"); reason != "" {
					p.Details = append(p.Details, actions.Detail{Label: "السبب", Value: reason})
				}
				p.Warnings = []string{"سيُبلَّغ الموردون بالإلغاء، ولا يمكن التراجع عنه."}
				return p, nil
			},
			execute: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Outcome, error) {
				id := args.ID("order")
				if failure := h.cancelBuyerOrder(ctx, actor, "ar", id, args.Str("reason"), ""); failure != nil {
					return actions.Outcome{}, refuseMessage(failure.Message)
				}
				return actions.Outcome{Message: "أُلغي الطلب وأُبلغ الموردون.", URL: "/orders/" + strconv.FormatInt(id, 10)}, nil
			},
		},
		favoriteCommand(h, "favorite_add", "إضافة إلى المفضلة", true),
		favoriteCommand(h, "favorite_remove", "حذف من المفضلة", false),
	}
}

// cartAddPlan is a validated cart line.
type cartAddPlan struct {
	ctx      context.Context
	item     *commerce.CartItem
	product  string
	supplier string
	branch   string
}

func (p *cartAddPlan) preview(title string) actions.Preview {
	total, _ := p.item.UnitPrice.MulInt(int64(p.item.Quantity))
	return actions.Preview{
		Title:   title,
		Summary: fmt.Sprintf("%d × %s من %s", p.item.Quantity, p.product, p.supplier),
		Details: []actions.Detail{
			{Label: "سعر الوحدة", Value: egp(p.item.UnitPrice)},
			{Label: "الإجمالي", Value: egp(total)},
			{Label: "فرع الاستلام", Value: p.branch},
		},
	}
}

// planCartAdd is AddToCartSubmit without the HTTP: resolve the listing, apply
// the availability rule at the buyer's branch, price from the catalogue.
func (h *UIHandler) planCartAdd(ctx context.Context, actor authctx.Actor, variantID int64, qty int) (*cartAddPlan, error) {
	if h.commSvc == nil || h.catSvc == nil {
		return nil, actions.Refuse("خدمة السلة غير متاحة حالياً.")
	}
	bctx, bactor, branch, err := h.buyingContext(ctx, actor, 0)
	if err != nil {
		return nil, err
	}
	variantID, productID, vendorOrgID := h.resolveCartLine(bctx, variantID, 0, 0)
	if variantID <= 0 || vendorOrgID <= 0 {
		return nil, actions.Refuse("هذا العرض لم يعد متاحاً.")
	}
	res, err := h.cartLineAvailability(bctx, bactor, h.buyingBranchID(bctx, &bactor), variantID, vendorOrgID, qty)
	if err != nil {
		h.log.ErrorContext(ctx, "assistant cart_add availability", "error", err, "variant", variantID)
		return nil, actions.Refuse("%s", i18n.T("ar", "customer.cart.availability_check_failed"))
	}
	if !res.Allowed {
		return nil, refuseMessage(res.Message("ar"))
	}
	item := &commerce.CartItem{ProductID: productID, ProductVariantID: variantID, OrganizationID: vendorOrgID, Quantity: qty}
	if err := h.priceCartItem(bctx, item); err != nil {
		return nil, err
	}
	return &cartAddPlan{
		ctx: bctx, item: item, branch: branch,
		product:  h.productName(bctx, productID),
		supplier: h.resolveOrgName(bctx, vendorOrgID),
	}, nil
}

// cartQuantityPlan is a validated change to an existing cart line.
type cartQuantityPlan struct {
	h      *UIHandler
	ctx    context.Context
	actor  authctx.Actor
	line   *commerce.CartItem
	name   string
	target int
}

func (p *cartQuantityPlan) preview() actions.Preview {
	if p.target == 0 {
		return actions.Preview{
			Title:   "حذف من السلة",
			Summary: fmt.Sprintf("%s (الكمية الحالية %d)", p.name, p.line.Quantity),
		}
	}
	total, _ := p.line.UnitPrice.MulInt(int64(p.target))
	return actions.Preview{
		Title:   "تعديل كمية في السلة",
		Summary: fmt.Sprintf("%s: من %d إلى %d", p.name, p.line.Quantity, p.target),
		Details: []actions.Detail{{Label: "الإجمالي الجديد", Value: egp(total)}},
	}
}

// apply is UpdateCartQuantitySubmit's two paths: an offer line by its own id,
// a variant line by variant (with quantity 0 meaning remove).
func (p *cartQuantityPlan) apply() (actions.Outcome, error) {
	var err error
	if p.line.ProductVariantID <= 0 {
		_, err = p.h.commSvc.SetCartLineQuantity(p.ctx, p.actor.UserID, p.line.ID, p.target)
	} else {
		_, err = p.h.commSvc.SetCartQuantity(p.ctx, p.actor.UserID, p.line.ProductVariantID, p.target)
	}
	if err != nil {
		return actions.Outcome{}, refuseMessage(p.h.safeMessage(err, "ar"))
	}
	if p.target == 0 {
		return actions.Outcome{Message: "حُذف " + p.name + " من السلة.", URL: "/cart"}, nil
	}
	return actions.Outcome{Message: fmt.Sprintf("أصبحت كمية %s %d.", p.name, p.target), URL: "/cart"}, nil
}

func (h *UIHandler) planCartQuantity(ctx context.Context, actor authctx.Actor, lineID int64, qty int) (*cartQuantityPlan, error) {
	if h.commSvc == nil {
		return nil, actions.Refuse("خدمة السلة غير متاحة حالياً.")
	}
	bctx, bactor, _, err := h.buyingContext(ctx, actor, 0)
	if err != nil {
		return nil, err
	}
	cart, err := h.commSvc.GetCart(bctx, bactor.UserID, buyerOrgID(bctx))
	if err != nil {
		return nil, err
	}
	var line *commerce.CartItem
	if cart != nil {
		for _, it := range cart.Items {
			if it != nil && it.ID == lineID {
				line = it
			}
		}
	}
	if line == nil {
		return nil, actions.Refuse("هذا البند لم يعد في السلة.")
	}
	name := line.ProductName.Get(i18n.AR)
	if name == "" {
		name = h.productName(bctx, line.ProductID)
	}
	// Raising a quantity is a purchase decision and gets the same check as
	// adding the line; lowering or removing does not.
	if qty > 0 && line.ProductVariantID > 0 {
		res, err := h.cartLineAvailability(bctx, bactor, h.buyingBranchID(bctx, &bactor), line.ProductVariantID, line.OrganizationID, qty)
		if err != nil {
			return nil, actions.Refuse("%s", i18n.T("ar", "customer.cart.availability_check_failed"))
		}
		if !res.Allowed {
			return nil, refuseMessage(res.Message("ar"))
		}
	}
	return &cartQuantityPlan{h: h, ctx: bctx, actor: bactor, line: line, name: name, target: qty}, nil
}

// assistantCheckout is a checkout plan with the card that describes it.
type assistantCheckout struct {
	ctx      context.Context
	actor    authctx.Actor
	checkout *checkoutPlan
	preview  actions.Preview
}

func (h *UIHandler) planAssistantCheckout(ctx context.Context, actor authctx.Actor, args actions.Args) (*assistantCheckout, error) {
	method := strings.TrimSpace(args.Str("payment_method"))
	if method == "" {
		method = commerce.DefaultPaymentMethod
	}
	if commerce.IsWalletPayment(method) {
		return nil, actions.Refuse("الدفع من المحفظة غير متاح عبر المساعد. أكمل الطلب من صفحة الدفع إذا أردت الدفع من المحفظة.")
	}
	bctx, bactor, branch, err := h.buyingContext(ctx, actor, args.ID("branch"))
	if err != nil {
		return nil, err
	}
	plan, failure := h.planCheckout(bctx, bactor, "ar", checkoutRequest{PaymentMethod: method, Notes: args.Str("notes")})
	if failure != nil {
		if failure.Back == "/cart" && failure.Message == "" {
			return nil, actions.Refuse("السلة فارغة.")
		}
		return nil, checkoutRefusal(failure)
	}
	return &assistantCheckout{
		ctx: bctx, actor: bactor, checkout: plan,
		preview: h.checkoutPreview(bctx, plan, branch, method),
	}, nil
}

func checkoutRefusal(f *checkoutFailure) error {
	if f.Message != "" {
		return refuseMessage(f.Message)
	}
	if f.Err != nil {
		return f.Err
	}
	return actions.Refuse("تعذّر تجهيز الطلب.")
}

// maxPreviewLines bounds how many products the card lists by name.
const maxPreviewLines = 15
