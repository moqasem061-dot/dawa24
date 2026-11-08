package commerce

import (
	"context"
	"errors"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// APICancelRequest carries the authenticated caller to the locked database check.
type APICancelRequest struct {
	OrderID        int64
	UserID         int64
	OrganizationID int64
	Staff          bool
	Reason         string
}

type APIVendorCancelRequest struct {
	OrderID     int64
	ShipmentID  int64
	VendorOrgID int64
	UserID      int64
	Reason      string
}

// ValidateAPIWalletOrder refuses a wallet-paid order for a status change other
// than a cancellation. The API cannot resolve the buyer's wallet owner (the payer), so
// the website makes those changes. Cancellation goes through the
// shared rule and refunds the wallet (see CheckCancellation).
func ValidateAPIWalletOrder(order *Order) error {
	if IsWalletPayment(order.PaymentMethod) {
		return apperr.Conflict("order.wallet_on_website", "يرجى تعديل طلبات المحفظة من الموقع. Please change wallet orders on the website.")
	}
	return nil
}

// CancellationRefusal says why an order cannot be cancelled. Unwrap gives the
// API error, so the JSON routes render a 409 from it. The website reads the
// typed value to choose its own messages.
type CancellationRefusal struct {
	// ShipmentInProgress is true when a shipment blocks the cancel. Otherwise
	// the order status does.
	ShipmentInProgress bool
	// Status is the order status the refusal was judged on.
	Status OrderStatus
	err    *apperr.Error
}

func (r *CancellationRefusal) Error() string { return r.err.Error() }

func (r *CancellationRefusal) Unwrap() error { return r.err }

// CheckCancellation is the one cancellation rule, shared by the website and
// every API route. An order can be cancelled only while its status is pending,
// processing, confirmed or on hold, and no shipment is shipped, in transit, out
// for delivery, delivered or completed. Callers must judge the order as read
// under the row lock. It returns nil when the cancellation is allowed.
func CheckCancellation(order *Order) error {
	switch order.Status {
	case StatusPending, StatusProcessing, StatusConfirmed, StatusOnHold:
	default:
		return &CancellationRefusal{Status: order.Status, err: apperr.Conflict("order.cannot_cancel",
			"لا يمكن إلغاء هذا الطلب في حالته الحالية. This order cannot be cancelled in its current state.")}
	}
	for _, shipment := range order.Shipments {
		if shipment == nil {
			continue
		}
		switch shipment.Status {
		case StatusShipped, StatusInTransit, StatusOutForDelivery, StatusDelivered, StatusCompleted:
			return &CancellationRefusal{ShipmentInProgress: true, Status: order.Status, err: apperr.Conflict("order.shipment_in_progress",
				"لا يمكن إلغاء طلب به شحنة قيد التوصيل. An order with a shipment in progress cannot be cancelled.")}
		}
	}
	return nil
}

// ValidateAPICancellation applies the shared cancellation rule to a JSON route.
func ValidateAPICancellation(order *Order) error {
	return CheckCancellation(order)
}

// WalletGoodsAmount is the goods part of a shipment's total: the total less its
// delivery fee, or the subtotal when there is no fee. A partial wallet refund
// for one shipment is capped at this amount.
func (s *OrderShipment) WalletGoodsAmount() (money.Amount, error) {
	goods := s.TotalAmount
	if s.ShippingFee.IsPositive() && s.ShippingFee.Minor() < s.TotalAmount.Minor() {
		return s.TotalAmount.Sub(s.ShippingFee)
	}
	if s.Subtotal.IsPositive() {
		goods = s.Subtotal
	}
	return goods, nil
}

type apiCanceller interface {
	CancelAPIOrder(context.Context, APICancelRequest) error
}

type apiVendorCanceller interface {
	CancelAPIVendorShipment(context.Context, APIVendorCancelRequest) error
}

// shipmentWalletUpdater is implemented by a repository that can charge a wallet
// order's shipment in the same transaction as its status change.
type shipmentWalletUpdater interface {
	UpdateShipmentStatusWithWallet(ctx context.Context, id int64, from, to OrderStatus, history OrderStatusHistory, walletUserID int64, hook ShipmentTxHook) error
}

func (s *Service) CancelAPIVendorShipment(ctx context.Context, request APIVendorCancelRequest) error {
	order, err := s.repo.GetOrderByID(ctx, request.OrderID)
	if err != nil {
		return err
	}
	if err := ValidateAPICancellation(order); err != nil {
		return err
	}
	canceller, ok := s.repo.(apiVendorCanceller)
	if !ok {
		return apperr.Internal(errors.New("repository does not support locked API shipment cancellation"))
	}
	return canceller.CancelAPIVendorShipment(ctx, request)
}

func (s *Service) CancelAPIOrder(ctx context.Context, request APICancelRequest) error {
	order, err := s.repo.GetOrderByID(ctx, request.OrderID)
	if err != nil {
		return err
	}
	if err := ValidateAPICancellation(order); err != nil {
		return err
	}
	canceller, ok := s.repo.(apiCanceller)
	if !ok {
		return apperr.Internal(errors.New("repository does not support locked API cancellation"))
	}
	return canceller.CancelAPIOrder(ctx, request)
}
