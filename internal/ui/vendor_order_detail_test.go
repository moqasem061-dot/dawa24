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

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

type mockOrderDetailCommerceRepo struct {
	commerce.Repository
	shipment *commerce.OrderShipment
	history  []*commerce.OrderStatusHistory
}

func (m *mockOrderDetailCommerceRepo) GetVendorShipment(_ context.Context, id, vendorOrgID int64) (*commerce.OrderShipment, error) {
	if m.shipment != nil && m.shipment.ID == id && m.shipment.OrganizationID == vendorOrgID {
		return m.shipment, nil
	}
	return nil, nil
}

func (m *mockOrderDetailCommerceRepo) GetVendorShipmentByOrderID(_ context.Context, orderID, vendorOrgID int64) (*commerce.OrderShipment, error) {
	if m.shipment != nil && m.shipment.OrderID == orderID && m.shipment.OrganizationID == vendorOrgID {
		return m.shipment, nil
	}
	return nil, nil
}

func (m *mockOrderDetailCommerceRepo) GetShipmentByID(_ context.Context, id int64) (*commerce.OrderShipment, error) {
	if m.shipment != nil && m.shipment.ID == id {
		return m.shipment, nil
	}
	return nil, nil
}

func (m *mockOrderDetailCommerceRepo) ListOrderHistory(_ context.Context, orderID int64) ([]*commerce.OrderStatusHistory, error) {
	return m.history, nil
}

func TestVendorOrderDetailPage_RenderAndData(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	now := time.Now()

	sh := &commerce.OrderShipment{
		ID:                    501,
		OrderID:               601,
		OrganizationID:        10,
		ShipmentNumber:        "SH-501-TEST",
		OrderNumber:           "PO-601-TEST",
		Status:                commerce.StatusConfirmed,
		PaymentMethod:         "cod",
		PaymentStatus:         commerce.PaymentUnpaid,
		Subtotal:              money.FromMinor(100000), // 1,000.00 EGP
		ShippingFee:           money.FromMinor(5000),   // 50.00 EGP
		TotalAmount:           money.FromMinor(105000), // 1,050.00 EGP
		DeliveryCode:          "7894",
		CustomerOrgName:       i18n.New("صيدلية الشفاء العالمية", "Al-Shifa Pharmacy"),
		CustomerBranchName:    i18n.New("فرع المعادي الرئيسي", "Maadi Main Branch"),
		CustomerBranchAddress: "شارع 9، المعادي، القاهرة",
		CustomerBranchPhone:   "01099887766",
		CustomerManagerName:   "د. أحمد محمود",
		Lines: []*commerce.OrderLine{
			{
				ID:          1,
				OrderID:     601,
				ShipmentID:  501,
				ProductName: i18n.New("بانادول إكسترا أقراص", "Panadol Extra Tablets"),
				VariantName: i18n.New("شريط 12 قرص", "Strip 12 tabs"),
				SKU:         "MED-PAN-01",
				Quantity:    10,
				UnitPrice:   money.FromMinor(5000),
				CostPrice:   func() *money.Amount { c := money.FromMinor(3500); return &c }(),
				TotalPrice:  money.FromMinor(50000),
			},
			{
				ID:               2,
				OrderID:          601,
				ShipmentID:       501,
				ProductName:      i18n.New("كونجستال أقراص", "Congestal Tablets"),
				VariantName:      i18n.New("علبة 20 قرص", "Box 20 tabs"),
				SKU:              "MED-CONG-02",
				Quantity:         5,
				ListPrice:        money.FromMinor(10000), // 100.00 EGP public price
				UnitPrice:        money.FromMinor(8000),  // 80.00 EGP net unit price (20% discount)
				DiscountAmount:   money.FromMinor(10000), // 100.00 EGP total discount (20.00 EGP/unit * 5)
				TotalPrice:       money.FromMinor(40000), // 400.00 EGP net total
				OriginalDiscount: money.FromMinor(2000),  // 20.00%
				CostPrice:        func() *money.Amount { c := money.FromMinor(6000); return &c }(),
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	fromStatus := "pending"
	hist := []*commerce.OrderStatusHistory{
		{
			ID:         1,
			OrderID:    601,
			FromStatus: &fromStatus,
			ToStatus:   "confirmed",
			Notes:      "تم قبول الطلب واعتماده بالمخزن",
			CreatedAt:  now.Add(-1 * time.Hour),
		},
	}

	repo := &mockOrderDetailCommerceRepo{
		shipment: sh,
		history:  hist,
	}
	commSvc := commerce.NewService(repo, logger)
	handler := ui.NewUIHandler(nil, nil, nil, commSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	vendorActor := authctx.Actor{
		UserID:         20,
		OrganizationID: 10,
		OrgID:          10,
		OrgType:        "vendor",
		OrgStatus:      "approved",
		Role:           "vendor",
		IsOwner:        true,
		Scope:          rbac.ScopeVendor,
	}

	t.Run("VendorOrderShipmentCard includes dedicated order page link and button", func(t *testing.T) {
		data := pages.VendorOrdersData{
			FilterStatus: "",
			CanAssign:    false,
		}
		var buf strings.Builder
		err := pages.VendorOrderShipmentCard(sh, data, "ar").Render(context.Background(), &buf)
		if err != nil {
			t.Fatalf("render failed: %v", err)
		}
		html := buf.String()

		if !strings.Contains(html, "/vendor/orders/501") {
			t.Errorf("card must link to /vendor/orders/501, got: %s", html)
		}
		if !strings.Contains(html, "عرض وإدارة الطلب") {
			t.Error("card must have 'عرض وإدارة الطلب' action button")
		}
	})

	t.Run("VendorOrderDetailPage renders full order metadata, items, and actions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/vendor/orders/501", nil)
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))

		// Route parameters using chi
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "501")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.VendorOrderDetailPage(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()

		// 1. Breadcrumb and headers
		if !strings.Contains(body, "أوامر التوريد") || !strings.Contains(body, "أمر توريد #SH-501-TEST") {
			t.Error("detail page must have breadcrumb with order number")
		}

		// 2. Customer and Branch details
		if !strings.Contains(body, "صيدلية الشفاء العالمية") {
			t.Error("detail page must display customer organization name")
		}
		if !strings.Contains(body, "فرع المعادي الرئيسي") {
			t.Error("detail page must display customer branch name")
		}
		if !strings.Contains(body, "د. أحمد محمود") {
			t.Error("detail page must display manager name")
		}
		if !strings.Contains(body, "01099887766") {
			t.Error("detail page must display branch phone")
		}

		// 3. Line items & Discount Percentages
		if !strings.Contains(body, "بانادول إكسترا أقراص") || !strings.Contains(body, "كونجستال أقراص") {
			t.Error("detail page must display all line item product names")
		}
		if !strings.Contains(body, "MED-PAN-01") || !strings.Contains(body, "MED-CONG-02") {
			t.Error("detail page must display line item SKUs")
		}
		if !strings.Contains(body, "15 عبوة") {
			t.Error("detail page must display total item quantity (15 عبوة)")
		}
		if !strings.Contains(body, "نسبة الخصم %") {
			t.Error("detail page must have 'نسبة الخصم %' column header")
		}
		if !strings.Contains(body, "20%") {
			t.Error("detail page must display 20% unit discount percentage badge")
		}
		if !strings.Contains(body, "20.00 ج.م/وحدة") {
			t.Error("detail page must display unit discount monetary deduction (20.00 ج.م/وحدة)")
		}

		// 4. Delivery Code (PIN) must NEVER be displayed to vendor
		if strings.Contains(body, "7894") || strings.Contains(body, "كود التسليم") {
			t.Error("detail page must NEVER display delivery verification code PIN to vendor")
		}

		// 5. Standard Egyptian Invoice Print (not waybill)
		if !strings.Contains(body, "طباعة الفاتورة") {
			t.Error("detail page must display standard invoice print button")
		}
		if strings.Contains(body, "طباعة بوليصة الشحن") {
			t.Error("detail page must not display broken waybill button")
		}

		// 6. Financial summary
		if !strings.Contains(body, "إجمالي سعر الجمهور") || !strings.Contains(body, "الخصم الممنوح") || !strings.Contains(body, "تكاليف الشحن") || !strings.Contains(body, "السعر الصافي") {
			t.Error("detail page must display clear financial breakdown in order: public price, discount, shipping, net")
		}

		// 7. Actions toolbar
		if !strings.Contains(body, "إجراءات إدارة حالة الطلب والتوريد") {
			t.Error("detail page must display order state management actions")
		}
	})
}

func TestVendorDeliveryAssign_PendingGated(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	now := time.Now()

	sh := &commerce.OrderShipment{
		ID:             502,
		OrderID:        602,
		OrganizationID: 10,
		ShipmentNumber: "SH-502-TEST",
		Status:         commerce.StatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	repo := &mockOrderDetailCommerceRepo{
		shipment: sh,
	}
	commSvc := commerce.NewService(repo, logger)
	handler := ui.NewUIHandler(nil, nil, nil, commSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	vendorActor := authctx.Actor{
		UserID:         20,
		OrganizationID: 10,
		OrgID:          10,
		OrgType:        "vendor",
		OrgStatus:      "approved",
		Role:           "vendor",
		IsOwner:        true,
		Scope:          rbac.ScopeVendor,
	}

	t.Run("VendorOrderDetailPage shows locked message when pending", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/vendor/orders/502", nil)
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "502")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.VendorOrderDetailPage(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "يلزم قبول وتأكيد أمر التوريد أولاً للبدء في الشحن وتعيين المندوب") {
			t.Error("expected pending lock notice on order detail page")
		}
		if !strings.Contains(body, "قبول وتأكيد أمر التوريد") {
			t.Error("expected accept order button in header")
		}
		if strings.Contains(body, "name=\"courier_user_id\"") {
			t.Error("courier selection must NOT be visible when status is pending")
		}
	})

	t.Run("VendorDeliveryAssignSubmit rejects assignment when shipment is pending", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/vendor/delivery/502/assign", strings.NewReader("courier_user_id=99&return_to=/vendor/orders/602"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "502")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.VendorDeliveryAssignSubmit(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}
		loc := rr.Header().Get("Location")
		if !strings.Contains(loc, "notice=error") {
			t.Errorf("expected error notice in redirect, got %s", loc)
		}
		if !strings.Contains(loc, "/vendor/orders/602") {
			t.Errorf("expected redirect back to /vendor/orders/602, got %s", loc)
		}
	})

	t.Run("VendorDeliveryAssign alias /vendor/orders/{id}/assign-courier works", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/vendor/orders/502/assign-courier", strings.NewReader("courier_user_id=99&return_to=/vendor/orders/602"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "502")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.VendorDeliveryAssignSubmit(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}
		loc := rr.Header().Get("Location")
		if !strings.Contains(loc, "/vendor/orders/602") {
			t.Errorf("expected redirect back to /vendor/orders/602, got %s", loc)
		}
	})
}

