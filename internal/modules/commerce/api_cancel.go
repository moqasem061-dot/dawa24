package commerce

import (
	"context"
	"errors"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
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

// IsWalletPayment reports whether an order is paid from the buyer's wallet. The
// match ignores case and surrounding spaces, so a spelling variant cannot step
// around the API rules while the website still reads it as another method.
func IsWalletPayment(method string) bool {
	return strings.EqualFold(strings.TrimSpace(method), "wallet")
}

// ValidateAPIWalletOrder refuses a wallet-paid order on the JSON status routes.
// The website debits the wallet when a supplier confirms and refunds it when a
// buyer cancels; the API does neither, so it must not change these orders.
func ValidateAPIWalletOrder(order *Order) error {
	if IsWalletPayment(order.PaymentMethod) {
		return apperr.Conflict("order.wallet_on_website", "يرجى تعديل طلبات المحفظة من الموقع. Please change wallet orders on the website.")
	}
	return nil
}

// ValidateAPICancellation enforces the temporary API rule until wallet refunds
// and cancellation are made atomic in Batch 1B: the order must be pending or
// processing and no shipment may have left the supplier yet.
func ValidateAPICancellation(order *Order) error {
	if IsWalletPayment(order.PaymentMethod) {
		return apperr.Conflict("order.wallet_cancel_on_website", "يرجى إلغاء طلب المحفظة من الموقع. Please cancel wallet orders on the website.")
	}
	if order.Status != StatusPending && order.Status != StatusProcessing {
		return apperr.Conflict("order.cannot_cancel", "لا يمكن إلغاء هذا الطلب في حالته الحالية. This order cannot be cancelled in its current state.")
	}
	for _, shipment := range order.Shipments {
		if shipment == nil {
			continue
		}
		// Allow-list: anything not still before dispatch blocks the cancel.
		switch shipment.Status {
		case StatusPending, StatusProcessing, StatusConfirmed, StatusOnHold, StatusCancelled:
		default:
			return apperr.Conflict("order.shipment_in_progress", "لا يمكن إلغاء طلب به شحنة قيد التوصيل. An order with a shipment in progress cannot be cancelled.")
		}
	}
	return nil
}

type apiCanceller interface {
	CancelAPIOrder(context.Context, APICancelRequest) error
}

type apiVendorCanceller interface {
	CancelAPIVendorShipment(context.Context, APIVendorCancelRequest) error
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
