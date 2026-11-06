package ui

import (
	"context"
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// computeCheckoutGoodsAmount calculates the exact sum of line items net of discounts.
func computeCheckoutGoodsAmount(items []commerce.CheckoutLineItem) money.Amount {
	var total money.Amount
	for _, item := range items {
		lineSubtotal, err := item.UnitPrice.MulInt(int64(item.Quantity))
		if err != nil {
			continue
		}
		lineTotal := lineSubtotal
		if item.ListPrice.IsPositive() && item.ListPrice.Minor() > item.UnitPrice.Minor() {
			lineTotal = lineSubtotal
		} else if item.DiscountAmount.IsPositive() && item.DiscountAmount.Minor() < lineSubtotal.Minor() {
			if sub, err := lineSubtotal.Sub(item.DiscountAmount); err == nil { // nolint:errswallow: money arithmetic fails only on overflow; the undiscounted total stands
				lineTotal = sub
			}
		}
		total, _ = total.Add(lineTotal)
	}
	return total
}

// verifyWalletFunds verifies tenant wallet funds availability at checkout time without debiting.
func (h *UIHandler) verifyWalletFunds(
	ctx context.Context,
	actor authctx.Actor,
	goodsAmount money.Amount,
) (int64, error) {
	if !actor.IsBuyer() || actor.OrganizationID <= 0 {
		return 0, apperr.Validation("wallet.company_required", i18n.T("ar", "wallet.company_required"), nil)
	}
	if h.billSvc == nil {
		return 0, apperr.Validation("wallet.service_unavailable", "خدمة المحفظة غير متاحة حالياً.", nil)
	}

	// Defense-in-depth: verify wallet payment method is active and enabled for checkout
	if pm, err := h.billSvc.GetPlatformPaymentMethod(ctx, "wallet"); err != nil || pm == nil || !pm.IsActive || !pm.IsCheckoutEnabled {
		return 0, apperr.Validation("wallet.checkout_disabled", "طريقة الدفع عبر المحفظة غير مفعلة حالياً عند طلب الشراء.", nil)
	}

	if !goodsAmount.IsPositive() {
		return 0, apperr.Validation("wallet.invalid_amount", "المبلغ المطلوب سداده غير صالح.", nil)
	}

	walletUserID, _ := resolveTenantUserIDs(ctx, h, actor)
	wallet, err := h.billSvc.GetWallet(ctx, walletUserID, "EGP")
	if err != nil || wallet == nil {
		return 0, apperr.Validation("wallet.not_found", "تعذر الوصول للمحفظة المؤسسية الخاصة بحسابك.", nil)
	}

	if wallet.AvailableBalance.Minor() < goodsAmount.Minor() {
		return 0, apperr.Validation("wallet.insufficient_funds",
			fmt.Sprintf("رصيد المحفظة المؤسسية المتاح للاستخدام (%s ج.م) غير كافٍ لسداد قيمة الأصناف المطلوبة (%s ج.م).", wallet.AvailableBalance.String(), goodsAmount.String()), nil)
	}

	return walletUserID, nil
}

// buyerWalletUserID returns the user whose wallet pays for a wallet order placed by
// the buyer's organisation.
func (h *UIHandler) buyerWalletUserID(ctx context.Context, order *commerce.Order) int64 {
	var buyerOrgID int64
	if order.OrganizationID != nil {
		buyerOrgID = *order.OrganizationID
	}
	actor := authctx.Actor{
		UserID:         order.CustomerID,
		OrganizationID: buyerOrgID,
		Scope:          "pharmacy",
	}
	walletUserID, _ := resolveTenantUserIDs(ctx, h, actor)
	return walletUserID
}

// prepareWalletShipmentCharge checks, before the commerce transaction starts, that
// the buyer's wallet covers one shipment and returns the wallet's user. The check
// is advisory and gives the message the buyer has always seen. The debit itself
// runs under the wallet lock in the same transaction as the confirmation, and it
// is the authority (Batch 2, S02).
func (h *UIHandler) prepareWalletShipmentCharge(ctx context.Context, order *commerce.Order, shipment *commerce.OrderShipment) (int64, error) {
	goods, err := shipment.WalletGoodsAmount()
	if err != nil {
		return 0, err
	}
	return h.prepareWalletCharge(ctx, order, goods)
}

// prepareWalletNegotiationCharge is prepareWalletShipmentCharge for an accepted
// negotiation: it covers the accepting supplier's shipments only.
func (h *UIHandler) prepareWalletNegotiationCharge(ctx context.Context, order *commerce.Order, vendorOrgID int64) (int64, error) {
	goods := money.Zero
	for _, s := range order.Shipments {
		if s == nil || s.OrganizationID != vendorOrgID {
			continue
		}
		g, err := s.WalletGoodsAmount()
		if err != nil {
			return 0, err
		}
		if goods, err = goods.Add(g); err != nil {
			return 0, err
		}
	}
	return h.prepareWalletCharge(ctx, order, goods)
}

func (h *UIHandler) prepareWalletCharge(ctx context.Context, order *commerce.Order, goodsAmount money.Amount) (int64, error) {
	if h.billSvc == nil {
		return 0, fmt.Errorf("خدمة المحفظة غير متاحة حالياً")
	}
	if !goodsAmount.IsPositive() {
		return 0, nil
	}
	walletUserID := h.buyerWalletUserID(ctx, order)
	wallet, err := h.billSvc.GetWallet(ctx, walletUserID, "EGP")
	if err != nil || wallet == nil {
		return 0, fmt.Errorf("تعذر الوصول للمحفظة المؤسسية الخاصة بالصيدلية")
	}
	if wallet.AvailableBalance.Minor() < goodsAmount.Minor() {
		return 0, fmt.Errorf("رصيد محفظة الصيدلية المتاح (%s ج.م) غير كافٍ لسداد قيمة الأصناف المطلوبة (%s ج.م)",
			wallet.AvailableBalance.String(), goodsAmount.String())
	}
	return walletUserID, nil
}
