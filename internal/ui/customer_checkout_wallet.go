package ui

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
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
			if sub, err := lineSubtotal.Sub(item.DiscountAmount); err == nil {
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

// processWalletPayment verifies tenant wallet funds and executes an atomic debit.
func (h *UIHandler) processWalletPayment(
	ctx context.Context,
	actor authctx.Actor,
	goodsAmount money.Amount,
) (int64, error) {
	walletUserID, err := h.verifyWalletFunds(ctx, actor, goodsAmount)
	if err != nil {
		return 0, err
	}

	// Atomic debit with pessimistic FOR UPDATE lock in billing repository
	desc := "سداد قيمة مشتريات أدوية من المحفظة المؤسسية"
	_, err = h.billSvc.Withdraw(ctx, walletUserID, "EGP", goodsAmount, "order", nil, desc)
	if err != nil {
		return 0, err
	}

	return walletUserID, nil
}

// executeOrderWalletPayment performs the actual wallet debit when the supplier confirms the order/shipment.
func (h *UIHandler) executeOrderWalletPayment(
	ctx context.Context,
	order *commerce.Order,
	shipment *commerce.OrderShipment,
) error {
	if order == nil || order.PaymentMethod != "wallet" || order.PaymentStatus == commerce.PaymentPaid {
		return nil
	}
	if h.billSvc == nil {
		return fmt.Errorf("خدمة المحفظة غير متاحة حالياً")
	}

	// Calculate goods amount for this confirmation
	var goodsAmount money.Amount
	if shipment != nil {
		goodsAmount = shipment.TotalAmount
		if shipment.ShippingFee.IsPositive() && shipment.ShippingFee.Minor() < shipment.TotalAmount.Minor() {
			goodsAmount, _ = shipment.TotalAmount.Sub(shipment.ShippingFee)
		} else if shipment.Subtotal.IsPositive() {
			goodsAmount = shipment.Subtotal
		}
	} else {
		goodsAmount = order.TotalAmount
		if order.ShippingFee.IsPositive() && order.ShippingFee.Minor() < order.TotalAmount.Minor() {
			goodsAmount, _ = order.TotalAmount.Sub(order.ShippingFee)
		} else if order.Subtotal.IsPositive() {
			if order.DiscountAmount.IsPositive() && order.DiscountAmount.Minor() < order.Subtotal.Minor() {
				goodsAmount, _ = order.Subtotal.Sub(order.DiscountAmount)
			} else {
				goodsAmount = order.Subtotal
			}
		}
	}

	if !goodsAmount.IsPositive() {
		return nil
	}

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

	wallet, err := h.billSvc.GetWallet(ctx, walletUserID, "EGP")
	if err != nil || wallet == nil {
		return fmt.Errorf("تعذر الوصول للمحفظة المؤسسية الخاصة بالصيدلية")
	}

	if wallet.AvailableBalance.Minor() < goodsAmount.Minor() {
		return fmt.Errorf("رصيد محفظة الصيدلية المتاح (%s ج.م) غير كافٍ لسداد قيمة الأصناف المطلوبة (%s ج.م)",
			wallet.AvailableBalance.String(), goodsAmount.String())
	}

	orderNum := order.OrderNumber
	if orderNum == "" {
		orderNum = fmt.Sprintf("ORD-%d", order.ID)
	}
	desc := fmt.Sprintf("سداد قيمة مشتريات أدوية للطلب رقم %s من المحفظة المؤسسية", orderNum)
	if _, err := h.billSvc.Withdraw(ctx, walletUserID, "EGP", goodsAmount, "order", &order.ID, desc); err != nil {
		return fmt.Errorf("فشل خصم المبلغ من محفظة الصيدلية: %w", err)
	}

	if h.commSvc != nil {
		_ = h.commSvc.UpdateOrderPaymentStatus(ctx, order.ID, commerce.PaymentPaid)
	}
	order.PaymentStatus = commerce.PaymentPaid
	if shipment != nil {
		shipment.PaymentStatus = commerce.PaymentPaid
	}

	return nil
}

// refundOrderWalletPayment returns debited funds to the buyer's wallet if an already-paid order is cancelled.
func (h *UIHandler) refundOrderWalletPayment(
	ctx context.Context,
	order *commerce.Order,
	shipment *commerce.OrderShipment,
	reason string,
) error {
	if order == nil || order.PaymentMethod != "wallet" || order.PaymentStatus != commerce.PaymentPaid {
		return nil
	}
	if h.billSvc == nil {
		return nil
	}

	var goodsAmount money.Amount
	if shipment != nil {
		goodsAmount = shipment.TotalAmount
		if shipment.ShippingFee.IsPositive() && shipment.ShippingFee.Minor() < shipment.TotalAmount.Minor() {
			goodsAmount, _ = shipment.TotalAmount.Sub(shipment.ShippingFee)
		} else if shipment.Subtotal.IsPositive() {
			goodsAmount = shipment.Subtotal
		}
	} else {
		goodsAmount = order.TotalAmount
		if order.ShippingFee.IsPositive() && order.ShippingFee.Minor() < order.TotalAmount.Minor() {
			goodsAmount, _ = order.TotalAmount.Sub(order.ShippingFee)
		} else if order.Subtotal.IsPositive() {
			if order.DiscountAmount.IsPositive() && order.DiscountAmount.Minor() < order.Subtotal.Minor() {
				goodsAmount, _ = order.Subtotal.Sub(order.DiscountAmount)
			} else {
				goodsAmount = order.Subtotal
			}
		}
	}

	if !goodsAmount.IsPositive() {
		return nil
	}

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

	orderNum := order.OrderNumber
	if orderNum == "" {
		orderNum = fmt.Sprintf("ORD-%d", order.ID)
	}
	desc := fmt.Sprintf("استرداد قيمة مشتريات للطلب الملغي رقم %s إلى المحفظة المؤسسية", orderNum)
	if reason != "" {
		desc += " (" + reason + ")"
	}
	_, err := h.billSvc.Deposit(ctx, walletUserID, "EGP", goodsAmount, "refund", &order.ID, desc)
	if err == nil && h.commSvc != nil {
		_ = h.commSvc.UpdateOrderPaymentStatus(ctx, order.ID, commerce.PaymentRefunded)
		order.PaymentStatus = commerce.PaymentRefunded
	}
	return err
}
