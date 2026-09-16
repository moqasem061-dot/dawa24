package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

type mockCommerceVendorOrdersRepo struct {
	commerce.Repository
	failedShipment *commerce.OrderShipment
}

func (m *mockCommerceVendorOrdersRepo) ListShipmentsByVendorWithTotal(_ context.Context, _ int64, status string, _, _ int) ([]*commerce.OrderShipment, int, error) {
	if status == "failed" && m.failedShipment != nil {
		return []*commerce.OrderShipment{m.failedShipment}, 1, nil
	}
	if m.failedShipment != nil {
		return []*commerce.OrderShipment{m.failedShipment}, 1, nil
	}
	return nil, 0, nil
}

func (m *mockCommerceVendorOrdersRepo) CountVendorShipmentsByStatus(_ context.Context, _ int64, statuses []string) (int, error) {
	for _, s := range statuses {
		if s == string(commerce.StatusFailed) {
			return 3, nil
		}
	}
	return 0, nil
}

func TestVendorOrdersFailedFilterAndCards(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	now := time.Now()
	failedSh := &commerce.OrderShipment{
		ID:                    888,
		OrderID:               999,
		OrganizationID:        77,
		ShipmentNumber:        "SH-FAIL-01",
		OrderNumber:           "ORD-888",
		Status:                commerce.StatusFailed,
		TotalAmount:           money.FromMinor(50000),
		CustomerBranchName:    i18n.New("صيدلية السلام", "Al-Salam"),
		CustomerOrgName:       i18n.New("مجموعة السلام", "Salam Group"),
		CustomerBranchAddress: "شارع التحرير، الدقي",
		DeliveryNotes:         "تعذر التواصل مع الصيدلي",
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	repo := &mockCommerceVendorOrdersRepo{failedShipment: failedSh}
	commSvc := commerce.NewService(repo, logger)
	handler := ui.NewUIHandler(nil, nil, nil, commSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	vendorActor := authctx.Actor{
		UserID:         50,
		OrganizationID: 77,
		OrgID:          77,
		OrgType:        "vendor",
		OrgStatus:      "approved",
		Role:           "vendor",
		IsOwner:        true,
		Scope:          rbac.ScopeVendor,
	}

	t.Run("Vendor orders page renders failed filter tab and metric card", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/vendor/orders?status=failed", nil)
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rr := httptest.NewRecorder()

		handler.VendorOrdersPage(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()

		if !strings.Contains(body, "تعذر التسليم / مرتجع") {
			t.Error("vendor orders page must contain 'تعذر التسليم / مرتجع' tab and metric")
		}
		if !strings.Contains(body, "status=failed") {
			t.Error("vendor orders page must link to ?status=failed")
		}
	})

	t.Run("Vendor order shipment card for failed shipment shows failure badge not success", func(t *testing.T) {
		data := pages.VendorOrdersData{
			FilterStatus: "failed",
			CanAssign:    false,
		}
		var buf strings.Builder
		if err := pages.VendorOrderShipmentCard(failedSh, data, "ar").Render(context.Background(), &buf); err != nil {
			t.Fatalf("render failed: %v", err)
		}
		cardHtml := buf.String()

		if !strings.Contains(cardHtml, "تعذّر تسليم الشحنة") {
			t.Error("shipment card for failed shipment must display 'تعذّر تسليم الشحنة'")
		}
		if strings.Contains(cardHtml, "تم تسليم الشحنة وتوثيق الاستلام") {
			t.Error("shipment card for failed shipment must NOT display 'تم تسليم الشحنة وتوثيق الاستلام'")
		}
	})
}
