package commerce

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// A supplier edits its own shipment only, and only before accepting it.
func TestUpdateVendorPendingShipment(t *testing.T) {
	buyerOrg := int64(100)
	newSvc := func(shipmentStatus OrderStatus) *Service {
		repo := newMockCommerceRepo()
		repo.orders[7] = &Order{
			ID:             7,
			OrganizationID: &buyerOrg,
			Status:         StatusPending,
			Shipments: []*OrderShipment{
				{ID: 70, OrderID: 7, OrganizationID: 200, Status: shipmentStatus},
				{ID: 71, OrderID: 7, OrganizationID: 300, Status: StatusConfirmed},
			},
		}
		return NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	input := UpdateCustomerOrderInput{OrderID: 7, Lines: []OrderLineEditItem{{ID: 1, Quantity: 2}}}

	tests := []struct {
		name     string
		status   OrderStatus
		actorOrg int64
		wantKind apperr.Kind
	}{
		{"own pending shipment", StatusPending, 200, ""},
		{"own shipment already accepted", StatusConfirmed, 200, apperr.KindForbidden},
		{"supplier not on the order", StatusPending, 999, apperr.KindForbidden},
		{"buyer is not a supplier here", StatusPending, 100, apperr.KindForbidden},
		{"no organisation", StatusPending, 0, apperr.KindForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actor := authctx.Actor{UserID: 5, OrganizationID: tc.actorOrg}
			res, err := newSvc(tc.status).UpdateVendorPendingShipment(context.Background(), actor, input)
			if tc.wantKind == "" {
				if err != nil || res == nil || res.ID != 7 {
					t.Fatalf("want success, got res=%v err=%v", res, err)
				}
				return
			}
			var ae *apperr.Error
			if !errors.As(err, &ae) || ae.Kind != tc.wantKind {
				t.Fatalf("want %s error, got %v", tc.wantKind, err)
			}
		})
	}
}
