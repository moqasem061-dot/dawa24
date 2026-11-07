package commerce

import (
	"context"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// The delivery PIN rule.
//
// A shipment created under the rule (DeliveryPINRequired) is closed as delivered
// only through the buyer's six-digit PIN, or through a staff override that gives
// a reason and is audit logged. Shipments created before the rule keep the old
// flow. Every path that could mark a parcel delivered goes through this file or
// the checks it names, so the rule is stated once.

// ErrDeliveryPINRequired refuses a plain delivered status on a parcel under the rule.
var ErrDeliveryPINRequired = apperr.Conflict("delivery.code_required",
	"تأكيد التسليم يتطلب رمز التسليم المكون من 6 أرقام الذي يعطيه المشتري. Delivery needs the buyer's six-digit delivery PIN.")

// hasOpenPINShipment reports whether a parcel of the order is still open under the rule.
func (o *Order) hasOpenPINShipment() bool {
	for _, sh := range o.Shipments {
		if sh != nil && sh.DeliveryPINRequired && !sh.IsClosed() {
			return true
		}
	}
	return false
}

// CompleteShipmentDelivery closes a supplier's own parcel against the buyer's PIN.
// The vendor's screens and API use it. The PIN attempt lockout is the one the
// courier path uses, because both end in the same repository transaction.
func (s *Service) CompleteShipmentDelivery(
	ctx context.Context,
	shipmentID, vendorOrgID int64,
	deliveryCode, notes string,
) (*OrderShipment, error) {
	code := strings.TrimSpace(deliveryCode)
	if code == "" {
		return nil, apperr.Validation("delivery.code_required",
			"أدخل رمز التسليم المكون من 6 أرقام. Enter the buyer's six-digit delivery PIN.",
			map[string]string{"delivery_code": "required"})
	}
	shipment, err := s.repo.GetVendorShipment(ctx, shipmentID, vendorOrgID)
	if err != nil {
		return nil, err
	}
	if shipment.IsClosed() {
		return nil, apperr.Conflict("delivery.shipment_closed",
			"هذه الشحنة مغلقة بالفعل. This shipment has already been closed.")
	}
	completed, err := s.repo.VerifyAndCompleteDelivery(ctx, shipment.ID, code, strings.TrimSpace(notes), -1)
	if err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "shipment delivered with buyer pin",
		"shipment_id", shipment.ID, "organization_id", vendorOrgID)
	return completed, nil
}

// OverrideShipmentDelivery is the staff path for a buyer who cannot give the PIN.
// The reason is required, and the override is written to the audit log.
func (s *Service) OverrideShipmentDelivery(
	ctx context.Context,
	shipmentID, actorUserID int64,
	reason string,
) (*OrderShipment, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, apperr.Validation("delivery.override_reason_required",
			"اكتب سبب التأكيد اليدوي للتسليم. Enter the reason for confirming delivery without the PIN.",
			map[string]string{"reason": "required"})
	}
	if actorUserID <= 0 {
		return nil, apperr.Forbidden("delivery.override_actor_required",
			"يجب تسجيل الدخول كموظف. A staff user is required.")
	}
	shipment, err := s.repo.GetShipmentByID(ctx, shipmentID)
	if err != nil {
		return nil, err
	}
	if shipment.IsClosed() {
		return nil, apperr.Conflict("delivery.shipment_closed",
			"هذه الشحنة مغلقة بالفعل. This shipment has already been closed.")
	}
	completed, err := s.repo.ConfirmDeliveryOverride(ctx, shipmentID, actorUserID, reason)
	if err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "shipment delivery overridden by staff",
		"shipment_id", shipmentID, "actor_user_id", actorUserID)
	return completed, nil
}
