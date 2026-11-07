package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// VerifyAndCompleteDelivery validates the 6-digit delivery confirmation PIN and marks the shipment as delivered.
func (r *Repository) VerifyAndCompleteDelivery(
	ctx context.Context,
	shipmentID int64,
	deliveryCode string,
	notes string,
	collectedAmountMinor int64,
) (*commerce.OrderShipment, error) {
	return r.completeDelivery(ctx, shipmentID, deliveryCode, notes, collectedAmountMinor, nil)
}

// ConfirmDeliveryOverride marks a shipment delivered without the buyer's PIN.
// It is the staff path for a buyer who cannot give the PIN. The same transaction
// closes the shipment and syncs the order; the override itself is written to the
// audit log with the actor and the reason.
func (r *Repository) ConfirmDeliveryOverride(
	ctx context.Context,
	shipmentID, actorUserID int64,
	reason string,
) (*commerce.OrderShipment, error) {
	return r.completeDelivery(ctx, shipmentID, "", reason, -1, &deliveryOverride{actorUserID: actorUserID, reason: reason})
}

// deliveryOverride marks a completion that skipped the PIN check.
type deliveryOverride struct {
	actorUserID int64
	reason      string
}

// completeDelivery is the one transaction that closes a parcel as delivered. A
// nil override checks the PIN, with the attempt lockout; a non-nil override
// skips the PIN and audits the actor and reason.
func (r *Repository) completeDelivery(
	ctx context.Context,
	shipmentID int64,
	deliveryCode string,
	notes string,
	collectedAmountMinor int64,
	override *deliveryOverride,
) (*commerce.OrderShipment, error) {
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Lock shipment row for update and read payment method
		query := `
			SELECT s.id, s.order_id, s.organization_id, s.status, s.delivery_code, s.delivery_attempts,
			       s.delivery_locked_until, s.total_amount, s.shipping_fee,
			       COALESCE(ord.payment_method, 'cod'), COALESCE(ord.payment_status, 'unpaid'),
			       s.delivery_pin_required
			FROM commerce.order_shipments s
			JOIN commerce.orders ord ON ord.id = s.order_id
			WHERE s.id = $1
			FOR UPDATE;
		`
		var currentStatus string
		var actualCode string
		var attempts int
		var lockedUntil *time.Time
		var totalAmount money.Amount
		var shippingFee money.Amount
		var paymentMethod string
		var paymentStatus string
		var orderID int64
		var orgID int64
		var pinRequired bool

		err := tx.QueryRow(txCtx, query, shipmentID).Scan(
			&shipmentID, &orderID, &orgID, &currentStatus, &actualCode,
			&attempts, &lockedUntil, &totalAmount, &shippingFee, &paymentMethod, &paymentStatus,
			&pinRequired,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.NotFound("shipment")
			}
			return fmt.Errorf("commerce postgres: lock shipment: %w", err)
		}

		currentOrderStatus := commerce.OrderStatus(currentStatus)
		now := time.Now()

		// 2. Check if locked due to too many failed attempts
		if lockedUntil != nil && lockedUntil.After(now) {
			remainingMinutes := int(lockedUntil.Sub(now).Minutes()) + 1
			return apperr.Conflict("delivery.locked",
				fmt.Sprintf(i18n.TDefault("w4_mod.d_145"), remainingMinutes))
		}

		// 3. Check status
		if currentOrderStatus == commerce.StatusDelivered || currentOrderStatus == commerce.StatusCompleted {
			return nil // Already delivered
		}
		if currentOrderStatus == commerce.StatusCancelled || currentOrderStatus == commerce.StatusReturned {
			return apperr.Conflict("delivery.cancelled", i18n.TDefault("w4_mod.w4str_146_146"))
		}

		// A wallet order is charged when its shipment is confirmed. A parcel that
		// was never confirmed would be delivered without being paid for.
		if commerce.IsWalletPayment(paymentMethod) {
			switch currentOrderStatus {
			case commerce.StatusPending, commerce.StatusProcessing, commerce.StatusOnHold:
				return apperr.Conflict("delivery.not_confirmed",
					"يجب تأكيد الشحنة أولاً قبل تسليمها. The shipment must be confirmed before it is delivered.")
			}
		}

		// 4. Verify Delivery Code (PIN). A shipment under the PIN rule always
		// needs a code, so an empty one is refused rather than matched.
		// A staff override skips this check, and is audited below.
		needsPIN := pinRequired || actualCode != ""
		if override == nil && needsPIN && (deliveryCode == "" || deliveryCode != actualCode) {
			newAttempts := attempts + 1
			if newAttempts >= 5 {
				lockTime := now.Add(15 * time.Minute)
				_ = database.SoftExec(txCtx, tx, `
					UPDATE commerce.order_shipments
					SET delivery_attempts = $2, delivery_locked_until = $3, updated_at = now()
					WHERE id = $1;
				`, shipmentID, newAttempts, lockTime)
				return apperr.Conflict("delivery.locked",
					i18n.TDefault("w4_mod.5_15_147"))
			}

			_ = database.SoftExec(txCtx, tx, `
				UPDATE commerce.order_shipments
				SET delivery_attempts = $2, updated_at = now()
				WHERE id = $1;
			`, shipmentID, newAttempts)
			return apperr.Validation("delivery.invalid_code",
				fmt.Sprintf(i18n.TDefault("w4_mod.d_148"), 5-newAttempts),
				map[string]string{"delivery_code": "invalid"})
		}

		// 5. Success: update shipment to delivered.
		//
		// If collectedAmountMinor < 0, fallback to computed courier collection amount.
		// If collectedAmountMinor >= 0, the courier specified an exact collected amount.
		if collectedAmountMinor < 0 {
			view := &commerce.OrderShipment{
				PaymentMethod: paymentMethod,
				PaymentStatus: commerce.PaymentStatus(paymentStatus),
				ShippingFee:   shippingFee,
				TotalAmount:   totalAmount,
			}
			collectedAmountMinor = view.CourierCollection().Amount.Minor()
		}

		updateQuery := `
			UPDATE commerce.order_shipments
			SET status = 'delivered',
			    delivered_at = now(),
			    delivered_by_courier_at = now(),
			    delivery_notes = $2,
			    collected_amount_minor = $3,
			    delivery_attempts = 0,
			    delivery_locked_until = NULL,
			    updated_at = now()
			WHERE id = $1;
		`
		if _, err := tx.Exec(txCtx, updateQuery, shipmentID, notes, collectedAmountMinor); err != nil {
			return fmt.Errorf("commerce postgres: update delivery status: %w", err)
		}

		if override != nil {
			if err := database.WriteAudit(txCtx, tx, database.AuditEntry{
				OrganizationID: &orgID,
				ActorUserID:    override.actorUserID,
				Action:         "shipment.delivery_override",
				EntityType:     "shipment",
				EntityID:       strconv.FormatInt(shipmentID, 10),
				Before:         map[string]any{"status": currentStatus},
				After:          map[string]any{"status": "delivered", "reason": override.reason},
			}); err != nil {
				return fmt.Errorf("commerce postgres: audit delivery override: %w", err)
			}
		}

		// 6. Record audit trail in order_status_history
		fromStatus := currentStatus
		historyQuery := `
			INSERT INTO commerce.order_status_history
				(order_id, shipment_id, from_status, to_status, notes)
			VALUES ($1, $2, $3, 'delivered', $4);
		`
		auditNotes := i18n.TDefault("w4_mod.s_359_359")
		if notes != "" {
			auditNotes += i18n.TDefault("w4_mod.w4str_149_149") + notes
		}
		if _, err := tx.Exec(txCtx, historyQuery, orderID, shipmentID, fromStatus, auditNotes); err != nil {
			return fmt.Errorf("commerce postgres: insert delivery history: %w", err)
		}

		// 7. Synchronize parent order status if all shipments are delivered
		var openCount, deliveredCount int
		_ = database.Soft(txCtx, tx, func() error {
			return tx.QueryRow(txCtx, openAndDeliveredShipmentsSQL, orderID).Scan(&openCount, &deliveredCount)
		})
		if openCount == 0 && deliveredCount > 0 {
			var totalOrder money.Amount
			var totalCollectedMinor int64
			_ = database.Soft(txCtx, tx, func() error {
				return tx.QueryRow(txCtx, `SELECT COALESCE(total_amount, 0) FROM commerce.orders WHERE id = $1;`, orderID).Scan(&totalOrder)
			})
			// When another supplier's part was cancelled, only the parts that
			// went ahead are owed, so the order total is the sum of those.
			var liveTotal money.Amount
			var deadCount int
			_ = database.Soft(txCtx, tx, func() error {
				return tx.QueryRow(txCtx, `
					SELECT COALESCE(SUM(total_amount) FILTER (WHERE status NOT IN ('cancelled', 'failed', 'returned')), 0),
					       COUNT(*) FILTER (WHERE status IN ('cancelled', 'failed', 'returned'))
					FROM commerce.order_shipments WHERE order_id = $1;`, orderID).Scan(&liveTotal, &deadCount)
			})
			if deadCount > 0 {
				totalOrder = liveTotal
			}
			_ = database.Soft(txCtx, tx, func() error {
				return tx.QueryRow(txCtx, `SELECT COALESCE(SUM(collected_amount_minor), 0) FROM commerce.order_shipments WHERE order_id = $1;`, orderID).Scan(&totalCollectedMinor)
			})

			// Also check if payments were recorded in billing.payments
			var billingPaid money.Amount
			_ = database.Soft(txCtx, tx, func() error {
				return tx.QueryRow(txCtx, `SELECT COALESCE(SUM(amount), 0) FROM billing.payments WHERE order_id = $1 AND status IN ('paid', 'completed');`, orderID).Scan(&billingPaid)
			})

			effectivePaidMinor := totalCollectedMinor
			if billingPaid.Minor() > effectivePaidMinor {
				effectivePaidMinor = billingPaid.Minor()
			}

			newPaymentStatus := paymentStatus
			if paymentMethod == "cod" {
				if effectivePaidMinor >= totalOrder.Minor() && totalOrder.Minor() > 0 {
					newPaymentStatus = "paid"
				} else if effectivePaidMinor > 0 {
					newPaymentStatus = "partially_paid"
				} else {
					newPaymentStatus = "unpaid"
				}
			}

			if _, errExec := tx.Exec(txCtx, `
				UPDATE commerce.orders
				SET status = 'delivered',
				    payment_status = $2,
				    delivered_at = now(),
				    updated_at = now()
				WHERE id = $1;
			`, orderID, newPaymentStatus); errExec != nil {
				return fmt.Errorf("commerce postgres: update delivered order: %w", errExec)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return r.GetShipmentByID(database.AsSystem(ctx), shipmentID)
}
