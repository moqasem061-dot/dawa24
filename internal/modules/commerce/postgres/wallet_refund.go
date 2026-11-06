package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Idempotency keys for wallet credits made by commerce. A cancellation event gets
// one key, so it credits at most once even if it were run twice.
func orderCancelKey(orderID int64) string {
	return fmt.Sprintf("order-cancel:%d", orderID)
}

func orderRefundKey(orderID int64) string {
	return fmt.Sprintf("order-refund:%d", orderID)
}

func shipmentCancelKey(orderID, shipmentID int64) string {
	return fmt.Sprintf("shipment-cancel:%d:%d", orderID, shipmentID)
}

func shipmentRefundKey(orderID, shipmentID int64) string {
	return fmt.Sprintf("shipment-refund:%d:%d", orderID, shipmentID)
}

// refundWalletDebits returns to the buyer's wallet what the cancelled or refunded
// part of a wallet order was charged. It runs inside tx, which must hold the order
// lock. shipmentID nil means the whole order; a shipment ID refunds only that
// supplier's charge (Batch 2, S02). Orders paid another way have no debit and
// return at once. A wallet order needs a refunder; without one the refund fails
// and the whole transaction rolls back. The caller decides what the credit means
// for its status: a credit of zero on a wallet order is a refusal for refunds.
func (r *Repository) refundWalletDebits(
	ctx context.Context,
	tx pgx.Tx,
	order *commerce.Order,
	shipmentID *int64,
	key string,
	actorUserID int64,
	reason string,
) (money.Amount, error) {
	if !commerce.IsWalletPayment(order.PaymentMethod) {
		return money.Zero, nil
	}
	if r.refunder == nil {
		return money.Zero, apperr.Internal(fmt.Errorf("commerce: wallet refunds are not configured for order %d", order.ID))
	}
	desc := fmt.Sprintf("استرداد قيمة مشتريات للطلب الملغي رقم %s إلى المحفظة المؤسسية", orderNumberOrID(order))
	if reason != "" {
		desc += " (" + reason + ")"
	}
	if !hasShipmentCharges(order) {
		return r.refundLegacyDebit(ctx, tx, order, shipmentID, key, actorUserID, desc)
	}

	items, capAmount, err := refundableCharges(order, shipmentID)
	if err != nil {
		return money.Zero, err
	}
	credited, err := r.refunder.CreditOrderRefund(ctx, tx, order.ID, &capAmount, key, desc, actorUserID)
	if err != nil {
		return money.Zero, err
	}
	if !credited.IsPositive() {
		return money.Zero, nil
	}
	if err := allocateShipmentRefunds(ctx, tx, items, credited); err != nil {
		return money.Zero, err
	}
	if err := settleWalletPayment(ctx, tx, order.ID); err != nil {
		return money.Zero, err
	}
	return credited, nil
}

// refundLegacyDebit refunds a wallet order that was debited before per-shipment
// payments existed: one debit for the order, with no shipment charge on record.
// A whole-order cancel refunds the net debit, as Batch 1B did. A shipment cancel
// is refused while other shipments are still live and a debit remains, because
// that debit may belong to the shipment being cancelled. Nothing is credited
// until an administrator has reviewed the order.
func (r *Repository) refundLegacyDebit(
	ctx context.Context,
	tx pgx.Tx,
	order *commerce.Order,
	shipmentID *int64,
	key string,
	actorUserID int64,
	desc string,
) (money.Amount, error) {
	if shipmentID != nil && hasOtherLiveShipment(order, *shipmentID) {
		net, err := r.refunder.OrderNetDebit(ctx, tx, order.ID)
		if err != nil {
			return money.Zero, err
		}
		if net.IsPositive() {
			return money.Zero, apperr.Conflict("order.wallet_review_required",
				"هذا الطلب مدفوع من المحفظة قبل نظام الدفع لكل مورد، ويحتاج مراجعة الإدارة قبل إلغاء شحنة منه. This wallet order was paid before per-supplier payments and needs administrator review before one of its shipments can be cancelled.")
		}
	}
	credited, err := r.refunder.CreditOrderRefund(ctx, tx, order.ID, nil, key, desc, actorUserID)
	if err != nil {
		return money.Zero, err
	}
	if credited.IsPositive() {
		if _, err := tx.Exec(ctx, `UPDATE commerce.orders SET payment_status = 'refunded', updated_at = now() WHERE id = $1;`, order.ID); err != nil {
			return money.Zero, err
		}
		order.PaymentStatus = commerce.PaymentRefunded
	}
	return credited, nil
}

// shipmentRefund is the part of one shipment's charge that is still open.
type shipmentRefund struct {
	id     int64
	amount money.Amount
}

// refundableCharges lists the open charges of the shipments in scope and their
// total. A shipment that was never charged has nothing to refund.
func refundableCharges(order *commerce.Order, shipmentID *int64) ([]shipmentRefund, money.Amount, error) {
	var items []shipmentRefund
	total := money.Zero
	for _, s := range order.Shipments {
		if s == nil || s.WalletPaidAt == nil {
			continue
		}
		if shipmentID != nil && s.ID != *shipmentID {
			continue
		}
		open, err := s.WalletPaidAmount.Sub(s.WalletRefundedAmount)
		if err != nil {
			return nil, money.Zero, apperr.Internal(err)
		}
		if !open.IsPositive() {
			continue
		}
		items = append(items, shipmentRefund{id: s.ID, amount: open})
		if total, err = total.Add(open); err != nil {
			return nil, money.Zero, apperr.Internal(err)
		}
	}
	return items, total, nil
}

// allocateShipmentRefunds records a credit against the shipments it refunds, in
// shipment order. The credit can never exceed the open charges it was capped by.
func allocateShipmentRefunds(ctx context.Context, tx pgx.Tx, items []shipmentRefund, credited money.Amount) error {
	left := credited
	for _, item := range items {
		if !left.IsPositive() {
			break
		}
		take := item.amount
		if left.Minor() < take.Minor() {
			take = left
		}
		if _, err := tx.Exec(ctx, `
			UPDATE commerce.order_shipments
			SET wallet_refunded_amount = wallet_refunded_amount + $2, updated_at = now()
			WHERE id = $1;`, item.id, take); err != nil {
			return err
		}
		var err error
		if left, err = left.Sub(take); err != nil {
			return apperr.Internal(err)
		}
	}
	if left.IsPositive() {
		return apperr.Internal(fmt.Errorf("commerce: refund credit %s exceeds the open shipment charges", credited.String()))
	}
	return nil
}

// hasShipmentCharges reports whether any shipment of the order was charged on its
// own, which marks the order as paid per shipment rather than by a legacy debit.
func hasShipmentCharges(order *commerce.Order) bool {
	for _, s := range order.Shipments {
		if s != nil && s.WalletPaidAt != nil {
			return true
		}
	}
	return false
}

// hasOtherLiveShipment reports whether a shipment other than id is still open.
func hasOtherLiveShipment(order *commerce.Order, id int64) bool {
	for _, s := range order.Shipments {
		if s == nil || s.ID == id {
			continue
		}
		switch s.Status {
		case commerce.StatusCancelled, commerce.StatusFailed, commerce.StatusReturned, commerce.StatusRefunded:
		default:
			return true
		}
	}
	return false
}

func orderNumberOrID(order *commerce.Order) string {
	if order.OrderNumber != "" {
		return order.OrderNumber
	}
	return fmt.Sprintf("ORD-%d", order.ID)
}

func actorOrZero(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}
