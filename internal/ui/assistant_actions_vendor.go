package ui

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/datasets"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Supplier commands: fulfilling orders, answering negotiations and quotation
// requests, and keeping listings and stock right.

var shipmentTargets = []string{string(commerce.StatusConfirmed), string(commerce.StatusShipped), string(commerce.StatusDelivered), string(commerce.StatusCancelled)}

var shipmentStatusLabel = map[string]string{
	"confirmed": "تأكيد الشحنة", "shipped": "شحن", "delivered": "تسليم", "cancelled": "إلغاء الشحنة",
}

func (h *UIHandler) vendorCommands() []assistantCommand {
	return []assistantCommand{
		{
			def: actions.Definition{
				Name: "shipment_update_status", Label: "تحديث حالة شحنة", Risk: actions.RiskHigh,
				Description: "Move one of this supplier's shipments to confirmed, shipped, delivered or cancelled. shipment is a ref from sales_orders.",
				Params: []actions.Param{
					{Name: "shipment", Type: actions.ParamRef, RefKind: handles.KindShipment, Required: true},
					{Name: "status", Type: actions.ParamEnum, Values: shipmentTargets, Required: true},
					{Name: "notes", Type: actions.ParamString, MaxLen: 300},
					{Name: "carrier", Type: actions.ParamString, MaxLen: 80},
					{Name: "tracking", Type: actions.ParamString, MaxLen: 80},
				},
			},
			audience: audienceVendor, keys: []string{"vendor.order.update"},
			prepare: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error) {
				sh, err := h.ownShipmentForTransition(ctx, actor, args.ID("shipment"), commerce.OrderStatus(args.Str("status")))
				if err != nil {
					return actions.Preview{}, err
				}
				p := actions.Preview{
					Title:   shipmentStatusLabel[args.Str("status")] + " " + sh.ShipmentNumber,
					Summary: fmt.Sprintf("من %s إلى %s — قيمة الشحنة %s", sh.Status, args.Str("status"), egp(sh.TotalAmount)),
				}
				for _, d := range []struct{ label, key string }{{"ملاحظات", "notes"}, {"شركة الشحن", "carrier"}, {"رقم التتبع", "tracking"}} {
					if v := args.Str(d.key); v != "" {
						p.Details = append(p.Details, actions.Detail{Label: d.label, Value: v})
					}
				}
				p.Warnings = []string{"سيُبلَّغ العميل بالحالة الجديدة."}
				return p, nil
			},
			execute: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Outcome, error) {
				id := args.ID("shipment")
				to := commerce.OrderStatus(args.Str("status"))
				if _, err := h.ownShipmentForTransition(ctx, actor, id, to); err != nil {
					return actions.Outcome{}, err
				}
				if err := h.transitionVendorShipment(ctx, actor, id, to, args.Str("notes"), args.Str("carrier"), args.Str("tracking")); err != nil {
					return actions.Outcome{}, refuseMessage(h.safeMessage(err, "ar"))
				}
				return actions.Outcome{Message: "تم تحديث حالة الشحنة وإبلاغ العميل.", URL: "/vendor/orders/" + strconv.FormatInt(id, 10)}, nil
			},
		},
		negotiationCommand(h, true),
		negotiationCommand(h, false),
		{
			def: actions.Definition{
				Name: "purchase_request_respond", Label: "الرد على طلب عرض أسعار", Risk: actions.RiskHigh,
				Description: "Set the status of a quotation request sent to this supplier, with a note to the pharmacy. request is a ref from incoming_purchase_requests.",
				Params: []actions.Param{
					{Name: "request", Type: actions.ParamRef, RefKind: handles.KindRequest, Required: true},
					{Name: "status", Type: actions.ParamEnum, Required: true, Values: []string{
						string(commerce.PurchaseRequestApproved), string(commerce.PurchaseRequestProcessing),
						string(commerce.PurchaseRequestCompleted), string(commerce.PurchaseRequestCancelled)}},
					{Name: "notes", Type: actions.ParamString, MaxLen: 1000},
				},
			},
			audience: audienceVendor, keys: []string{"vendor.purchase_request.respond"},
			prepare: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error) {
				if h.commSvc == nil {
					return actions.Preview{}, actions.Refuse("الخدمة غير متاحة حالياً.")
				}
				req, msg := h.incomingPurchaseRequest(ctx, actor, "ar", args.ID("request"))
				if msg != "" {
					return actions.Preview{}, refuseMessage(msg)
				}
				p := actions.Preview{
					Title:   "الرد على طلب عرض الأسعار " + req.RequestNumber,
					Summary: fmt.Sprintf("من %s إلى %s — %d أصناف، إجمالي تقديري %s", req.Status, args.Str("status"), req.TotalItems, egp(req.EstimatedTotal)),
				}
				if notes := args.Str("notes"); notes != "" {
					p.Details = []actions.Detail{{Label: "ملاحظتك للعميل", Value: notes}}
				}
				return p, nil
			},
			execute: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Outcome, error) {
				if h.commSvc == nil {
					return actions.Outcome{}, actions.Refuse("الخدمة غير متاحة حالياً.")
				}
				id := args.ID("request")
				if _, msg := h.respondVendorPurchaseRequest(ctx, actor, "ar", id,
					commerce.PurchaseRequestStatus(args.Str("status")), args.Str("notes")); msg != "" {
					return actions.Outcome{}, refuseMessage(msg)
				}
				return actions.Outcome{Message: "تم الرد على طلب عرض الأسعار وإبلاغ العميل.", URL: "/vendor/purchase-requests/" + strconv.FormatInt(id, 10)}, nil
			},
		},
		{
			def: actions.Definition{
				Name: "listing_update", Label: "تعديل صنف", Risk: actions.RiskHigh,
				Description: "Change one of this supplier's listings: price (EGP), discount (percent 0-100), min_order_qty, quota_limit (0 removes), " +
					"expiry_date (YYYY-MM-DD), status active|inactive. Only the fields given change. listing is a ref from catalog_listings.",
				Params: []actions.Param{
					{Name: "listing", Type: actions.ParamRef, RefKind: handles.KindVariant, Required: true},
					{Name: "price", Type: actions.ParamNumber, Min: 0, Max: 10000000},
					{Name: "discount", Type: actions.ParamNumber, Min: 0, Max: 100},
					{Name: "min_order_qty", Type: actions.ParamInt, Min: 1, Max: 100000},
					{Name: "quota_limit", Type: actions.ParamInt, Min: 0, Max: float64(catalog.MaxQuotaLimit)},
					{Name: "expiry_date", Type: actions.ParamString, MaxLen: 10},
					{Name: "status", Type: actions.ParamEnum, Values: []string{string(catalog.StatusActive), string(catalog.StatusInactive)}},
				},
			},
			audience: audienceVendor, keys: []string{"vendor.product.update"},
			prepare: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error) {
				return h.previewListingUpdate(ctx, actor, args)
			},
			execute: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Outcome, error) {
				form, err := listingForm(args)
				if err != nil {
					return actions.Outcome{}, err
				}
				if h.catSvc == nil {
					return actions.Outcome{}, actions.Refuse("خدمة الكتالوج غير متاحة حالياً.")
				}
				if _, msg, _ := h.saveVariantEdit(ctx, actor, "ar", args.ID("listing"), form, ""); msg != "" {
					return actions.Outcome{}, refuseMessage(msg)
				}
				return actions.Outcome{Message: "تم تعديل الصنف.", URL: "/vendor/products"}, nil
			},
		},
		{
			def: actions.Definition{
				Name: "stock_adjust", Label: "تسوية مخزون", Risk: actions.RiskHigh,
				Description: "Add to or remove from one stock line (a listing in a warehouse). change is +/- units. stock is a ref from stock_levels.",
				Params: []actions.Param{
					{Name: "stock", Type: actions.ParamRef, RefKind: datasets.KindStock, Required: true},
					{Name: "change", Type: actions.ParamInt, Required: true, Min: -1000000, Max: 1000000},
					{Name: "reason", Type: actions.ParamString, MaxLen: 300},
				},
			},
			audience: audienceVendor, keys: []string{"vendor.inventory.adjust"},
			prepare: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error) {
				st, err := h.adjustableStock(ctx, args.ID("stock"), int(args.Int("change")))
				if err != nil {
					return actions.Preview{}, err
				}
				change := int(args.Int("change"))
				p := actions.Preview{
					Title:   "تسوية مخزون " + h.productName(ctx, st.ProductID),
					Summary: fmt.Sprintf("المخزن %s: من %d إلى %d", h.warehouseName(ctx, st.WarehouseID), st.Quantity, st.Quantity+change),
				}
				if reason := args.Str("reason"); reason != "" {
					p.Details = []actions.Detail{{Label: "السبب", Value: reason}}
				}
				return p, nil
			},
			execute: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Outcome, error) {
				change := int(args.Int("change"))
				if _, err := h.adjustableStock(ctx, args.ID("stock"), change); err != nil {
					return actions.Outcome{}, err
				}
				st, err := h.invSvc.AdjustStock(ctx, inventory.AdjustStockInput{
					StockID: args.ID("stock"), Delta: change, Type: inventory.MovementAdjustment,
					Details: args.Str("reason"), UserID: &actor.UserID,
				})
				if err != nil {
					return actions.Outcome{}, refuseMessage(h.safeMessage(err, "ar"))
				}
				return actions.Outcome{Message: fmt.Sprintf("أصبح الرصيد %d.", st.Quantity), URL: "/vendor/inventory"}, nil
			},
		},
	}
}

func (h *UIHandler) ownShipmentForTransition(ctx context.Context, actor authctx.Actor, id int64, to commerce.OrderStatus) (*commerce.OrderShipment, error) {
	if h.commSvc == nil {
		return nil, actions.Refuse("الخدمة غير متاحة حالياً.")
	}
	sh, err := h.commSvc.GetVendorShipment(ctx, id, actor.OrganizationID)
	if err != nil || sh == nil || sh.OrganizationID != actor.OrganizationID {
		return nil, actions.Refuse("%s", i18n.T("ar", "orders.order_not_found"))
	}
	if !commerce.IsValidStatusTransition(sh.Status, to) {
		return nil, actions.Refuse("لا يمكن نقل الشحنة من %s إلى %s.", sh.Status, to)
	}
	return sh, nil
}

func negotiationCommand(h *UIHandler, accept bool) assistantCommand {
	name, label, desc := "negotiation_accept", "قبول سعر التفاوض", "Accept the buyer's proposed prices on an order this supplier ships part of."
	if !accept {
		name, label, desc = "negotiation_reject", "رفض سعر التفاوض", "Reject the buyer's proposed prices, which cancels the negotiated order."
	}
	params := []actions.Param{{Name: "shipment", Type: actions.ParamRef, RefKind: handles.KindShipment, Required: true}}
	if !accept {
		params = append(params, actions.Param{Name: "reason", Type: actions.ParamString, MaxLen: 300})
	}
	orderFor := func(ctx context.Context, actor authctx.Actor, shipmentID int64) (*commerce.Order, error) {
		if h.commSvc == nil {
			return nil, actions.Refuse("الخدمة غير متاحة حالياً.")
		}
		sh, err := h.commSvc.GetVendorShipment(ctx, shipmentID, actor.OrganizationID)
		if err != nil || sh == nil {
			return nil, actions.Refuse("%s", i18n.T("ar", "vendor.orders.order_not_found"))
		}
		order, msg := h.supplierOrder(ctx, actor, "ar", sh.OrderID)
		if msg != "" {
			return nil, refuseMessage(msg)
		}
		if !order.IsNegotiation || order.NegotiationStatus != "pending" {
			return nil, actions.Refuse("لا يوجد تفاوض معلّق على هذا الطلب.")
		}
		return order, nil
	}
	return assistantCommand{
		def: actions.Definition{Name: name, Label: label, Risk: actions.RiskHigh,
			Description: desc + " shipment is a ref from sales_orders.", Params: params},
		audience: audienceVendor, keys: []string{"vendor.order.negotiate"},
		prepare: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error) {
			order, err := orderFor(ctx, actor, args.ID("shipment"))
			if err != nil {
				return actions.Preview{}, err
			}
			p := actions.Preview{Title: label + " — " + order.OrderNumber, Summary: "إجمالي الطلب " + egp(order.TotalAmount)}
			if !accept {
				p.Warnings = []string{"رفض التفاوض يلغي الطلب."}
				if reason := args.Str("reason"); reason != "" {
					p.Details = []actions.Detail{{Label: "السبب", Value: reason}}
				}
			}
			return p, nil
		},
		execute: func(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Outcome, error) {
			order, err := orderFor(ctx, actor, args.ID("shipment"))
			if err != nil {
				return actions.Outcome{}, err
			}
			reason := args.Str("reason")
			if !accept && reason == "" {
				reason = i18n.T("ar", "vendor.orders.negotiation_default_reject_reason")
			}
			if msg, _ := h.decideVendorNegotiation(ctx, actor, "ar", order.ID, accept, reason); msg != "" {
				return actions.Outcome{}, refuseMessage(msg)
			}
			return actions.Outcome{Message: label + " تم وأُبلغ العميل.", URL: "/vendor/orders"}, nil
		},
	}
}

// listingForm maps listing_update arguments to the edit dialog's form keys, so
// the dialog's own validation applies.
func listingForm(args actions.Args) (url.Values, error) {
	form := url.Values{}
	if args.Has("price") {
		form.Set("price", args.Decimal("price"))
	}
	if args.Has("discount") {
		form.Set("discount", args.Decimal("discount"))
	}
	if args.Has("min_order_qty") {
		form.Set("min_order_qty", strconv.FormatInt(args.Int("min_order_qty"), 10))
	}
	if args.Has("quota_limit") {
		form.Set("quota_limit", strconv.FormatInt(args.Int("quota_limit"), 10))
	}
	if args.Has("expiry_date") {
		form.Set("expiry_date", args.Str("expiry_date"))
	}
	if args.Has("status") {
		form.Set("status", args.Str("status"))
	}
	if len(form) == 0 {
		return nil, actions.Refuse("حدّد حقلاً واحداً على الأقل لتعديله.")
	}
	return form, nil
}

func (h *UIHandler) previewListingUpdate(ctx context.Context, actor authctx.Actor, args actions.Args) (actions.Preview, error) {
	form, err := listingForm(args)
	if err != nil {
		return actions.Preview{}, err
	}
	if h.catSvc == nil {
		return actions.Preview{}, actions.Refuse("خدمة الكتالوج غير متاحة حالياً.")
	}
	current, msg := h.ownVariant(ctx, actor, "ar", args.ID("listing"))
	if msg != "" {
		return actions.Preview{}, refuseMessage(msg)
	}
	next := *current
	if err := applyVariantValues(form, &next, "ar"); err != nil {
		return actions.Preview{}, refuseMessage(err.Error())
	}
	p := actions.Preview{Title: "تعديل صنف " + h.productName(ctx, current.ProductID)}
	change := func(label, before, after string) {
		if before != after {
			p.Details = append(p.Details, actions.Detail{Label: label, Value: before + " ← " + after})
		}
	}
	change("السعر", egp(current.Price), egp(next.Price))
	change("الخصم %", current.Discount.String(), next.Discount.String())
	change("أقل كمية", strconv.Itoa(current.MinOrderQty), strconv.Itoa(next.MinOrderQty))
	change("حد الكوتة", quotaText(current.QuotaLimit), quotaText(next.QuotaLimit))
	change("الصلاحية", dateText(current.ExpiryDate), dateText(next.ExpiryDate))
	change("الحالة", string(current.Status), string(next.Status))
	if len(p.Details) == 0 {
		return actions.Preview{}, actions.Refuse("القيم المطلوبة مطابقة للقيم الحالية.")
	}
	labels := make([]string, len(p.Details))
	for i, d := range p.Details {
		labels[i] = d.Label
	}
	p.Summary = "يتغير: " + strings.Join(labels, "، ")
	return p, nil
}

func quotaText(q *int) string {
	if q == nil {
		return "بدون"
	}
	return strconv.Itoa(*q)
}

func dateText[T interface{ Format(string) string }](t *T) string {
	if t == nil {
		return "—"
	}
	return (*t).Format("2006-01-02")
}

func (h *UIHandler) adjustableStock(ctx context.Context, stockID int64, change int) (*inventory.Stock, error) {
	if h.invSvc == nil {
		return nil, actions.Refuse("خدمة المخزون غير متاحة حالياً.")
	}
	if change == 0 {
		return nil, actions.Refuse("حدّد مقدار التغيير.")
	}
	st, err := h.invSvc.GetStockByID(ctx, stockID)
	if err != nil || st == nil {
		return nil, actions.Refuse("بند المخزون غير موجود.")
	}
	if st.Quantity+change < 0 {
		return nil, actions.Refuse("الرصيد الحالي %d لا يكفي لخصم %d.", st.Quantity, -change)
	}
	return st, nil
}

func (h *UIHandler) warehouseName(ctx context.Context, id int64) string {
	if h.invSvc == nil {
		return "—"
	}
	w, err := h.invSvc.GetWarehouse(ctx, id)
	if err != nil || w == nil {
		return "—"
	}
	return w.Name
}
