package commerce

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// The delivery PIN rule: a shipment created under it cannot be closed without
// the buyer's PIN or a staff override; a shipment that predates it keeps the
// old flow.

const (
	pinTestOrg    = int64(7)
	pinTestUser   = int64(42)
	pinTestCode   = "482913"
	pinTestOrder  = int64(1)
	pinTestParcel = int64(10)
)

func pinService(t *testing.T, pinRequired bool, status OrderStatus) (*Service, *mockCommerceRepo) {
	t.Helper()
	repo := newMockCommerceRepo()
	shipment := &OrderShipment{
		ID: pinTestParcel, OrderID: pinTestOrder, OrganizationID: pinTestOrg,
		Status: status, DeliveryCode: pinTestCode, DeliveryPINRequired: pinRequired,
	}
	repo.shipments[pinTestOrder] = []*OrderShipment{shipment}
	repo.orders[pinTestOrder] = &Order{ID: pinTestOrder, Status: status, Shipments: []*OrderShipment{shipment}}
	return NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil))), repo
}

func pinTenantCtx() context.Context {
	return database.WithTenant(context.Background(), pinTestOrg)
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error %q, got nil", code)
	}
	ae, ok := apperr.As(err)
	if !ok {
		t.Fatalf("expected apperr %q, got %T: %v", code, err, err)
	}
	if ae.Code != code {
		t.Fatalf("error code = %q, want %q", ae.Code, code)
	}
}

func TestPlainDeliveredRefusedForPINShipment(t *testing.T) {
	svc, repo := pinService(t, true, StatusConfirmed)
	uid := pinTestUser

	_, err := svc.TransitionShipmentStatus(pinTenantCtx(), pinTestParcel, StatusDelivered, &uid, "")

	requireCode(t, err, "delivery.code_required")
	if got := repo.shipments[pinTestOrder][0].Status; got != StatusConfirmed {
		t.Fatalf("shipment status = %s, want confirmed (refused change must not write)", got)
	}
}

func TestPlainDeliveredStillWorksForLegacyShipment(t *testing.T) {
	svc, repo := pinService(t, false, StatusConfirmed)
	uid := pinTestUser

	got, err := svc.TransitionShipmentStatus(pinTenantCtx(), pinTestParcel, StatusDelivered, &uid, "")

	if err != nil {
		t.Fatalf("legacy shipment delivered: %v", err)
	}
	if got.Status != StatusDelivered || repo.shipments[pinTestOrder][0].Status != StatusDelivered {
		t.Fatalf("legacy shipment status not delivered")
	}
}

func TestOrderLevelDeliveredRefusedWhilePINShipmentOpen(t *testing.T) {
	svc, repo := pinService(t, true, StatusOutForDelivery)
	uid := pinTestUser

	err := svc.TransitionOrderStatus(pinTenantCtx(), pinTestOrder, StatusDelivered, &uid, "")

	requireCode(t, err, "delivery.code_required")
	if got := repo.orders[pinTestOrder].Status; got != StatusOutForDelivery {
		t.Fatalf("order status = %s, want out_for_delivery", got)
	}
}

func TestCompleteShipmentDeliveryChecksPIN(t *testing.T) {
	cases := []struct {
		name    string
		vendor  int64
		code    string
		wantErr string
		wantOK  bool
	}{
		{name: "empty code", vendor: pinTestOrg, code: "  ", wantErr: "delivery.code_required"},
		{name: "wrong code", vendor: pinTestOrg, code: "000000", wantErr: "delivery.invalid_code"},
		{name: "other supplier", vendor: 8, code: pinTestCode, wantErr: "shipment"},
		{name: "right code", vendor: pinTestOrg, code: pinTestCode, wantOK: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := pinService(t, true, StatusOutForDelivery)
			got, err := svc.CompleteShipmentDelivery(pinTenantCtx(), pinTestParcel, tc.vendor, tc.code, "")
			if tc.wantOK {
				if err != nil || got.Status != StatusDelivered {
					t.Fatalf("right code: err=%v status=%v", err, got)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected %s, got success", tc.wantErr)
			}
			if ae, ok := apperr.As(err); !ok || (ae.Code != tc.wantErr && ae.Kind != apperr.KindNotFound) {
				t.Fatalf("error = %v, want code %s", err, tc.wantErr)
			}
			if repo.shipments[pinTestOrder][0].Status == StatusDelivered {
				t.Fatalf("refused handover must not deliver the parcel")
			}
		})
	}
}

func TestOverrideShipmentDeliveryNeedsReasonAndActor(t *testing.T) {
	cases := []struct {
		name   string
		actor  int64
		reason string
		want   string
	}{
		{name: "blank reason", actor: pinTestUser, reason: "   ", want: "delivery.override_reason_required"},
		{name: "no actor", actor: 0, reason: "buyer lost the PIN", want: "delivery.override_actor_required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := pinService(t, true, StatusOutForDelivery)
			_, err := svc.OverrideShipmentDelivery(pinTenantCtx(), pinTestParcel, tc.actor, tc.reason)
			requireCode(t, err, tc.want)
			if len(repo.overrides) != 0 {
				t.Fatalf("refused override reached the repository")
			}
		})
	}
}

func TestOverrideShipmentDeliveryRecordsReasonAndActor(t *testing.T) {
	svc, repo := pinService(t, true, StatusOutForDelivery)

	got, err := svc.OverrideShipmentDelivery(pinTenantCtx(), pinTestParcel, pinTestUser, "  buyer lost the PIN  ")

	if err != nil {
		t.Fatalf("override: %v", err)
	}
	if got.Status != StatusDelivered {
		t.Fatalf("status = %s, want delivered", got.Status)
	}
	if len(repo.overrides) != 1 {
		t.Fatalf("overrides recorded = %d, want 1", len(repo.overrides))
	}
	want := overrideRecord{shipmentID: pinTestParcel, actorUserID: pinTestUser, reason: "buyer lost the PIN"}
	if repo.overrides[0] != want {
		t.Fatalf("override = %+v, want %+v", repo.overrides[0], want)
	}
}

func TestOverrideRefusedForClosedShipment(t *testing.T) {
	svc, repo := pinService(t, true, StatusDelivered)

	_, err := svc.OverrideShipmentDelivery(pinTenantCtx(), pinTestParcel, pinTestUser, "late confirmation")

	requireCode(t, err, "delivery.shipment_closed")
	if len(repo.overrides) != 0 {
		t.Fatalf("closed shipment was overridden")
	}
}
