package ui_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

func TestCustomerOrderCancel_EligibleStatusAndShipments_E2E(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}

	handler := newRealUIHandler(t, db)
	ctx := context.Background()

	// 1. Create a customer organization and user
	var customerOrgID int64
	err := db.Pool().QueryRow(ctx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"صيدلية اختبار الإلغاء"}', 'صيدلية اختبار الإلغاء', '{"ar":"صيدلية اختبار الإلغاء"}', 'TAX-CNCL-101', 'CR-CNCL-101', 'customer', 'approved')
		RETURNING id
	`).Scan(&customerOrgID)
	if err != nil {
		t.Fatalf("failed to insert customer org: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), "DELETE FROM org.organizations WHERE id = $1", customerOrgID)
	}()

	var vendorOrgID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"مورد اختبار الإلغاء"}', 'مورد اختبار الإلغاء', '{"ar":"مورد اختبار الإلغاء"}', 'TAX-VCNCL-102', 'CR-VCNCL-102', 'vendor', 'approved')
		RETURNING id
	`).Scan(&vendorOrgID)
	if err != nil {
		t.Fatalf("failed to insert vendor org: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), "DELETE FROM org.organizations WHERE id = $1", vendorOrgID)
	}()

	var userID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO identity.users (name, email, password_hash, role, status)
		VALUES ('{"ar":"د. صيدلي الإلغاء"}', 'order_canceller@dawa24.eg', '$2a$10$abcdefghijklmnopqrstuu', 'user', 'active')
		RETURNING id
	`).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), "DELETE FROM identity.users WHERE id = $1", userID)
	}()

	// 2. Insert a pending order with a shipment
	var orderID int64
	orderNum := fmt.Sprintf("ORD-CNCL-%d", time.Now().UnixNano()%100000)
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO commerce.orders (customer_id, organization_id, order_number, status, subtotal, discount_amount, total_amount)
		VALUES ($1, $2, $3, 'pending', 50000, 0, 50000)
		RETURNING id
	`, userID, customerOrgID, orderNum).Scan(&orderID)
	if err != nil {
		t.Fatalf("failed to insert test order: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), "DELETE FROM commerce.orders WHERE id = $1", orderID)
	}()

	var shipmentID int64
	shipmentNum := fmt.Sprintf("SH-CNCL-%d", time.Now().UnixNano()%100000)
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO commerce.order_shipments (order_id, organization_id, shipment_number, status, subtotal, total_amount)
		VALUES ($1, $2, $3, 'pending', 50000, 50000)
		RETURNING id
	`, orderID, vendorOrgID, shipmentNum).Scan(&shipmentID)
	if err != nil {
		t.Fatalf("failed to insert test shipment: %v", err)
	}

	// 3. Test buyer cancellation
	formData := url.Values{}
	formData.Set("reason", "تغيير في الاحتياجات / طلب بالخطأ")
	formData.Set("notes", "تم الشراء من مورد محلي آخر")

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/orders/%d/cancel", orderID), strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	actor := authctx.Actor{
		UserID:         userID,
		OrganizationID: customerOrgID,
		OrgType:        "customer",
		Role:           "owner",
		IsOwner:        true,
		Permissions:    []string{"pharmacy.order.view", "pharmacy.order.update", "pharmacy.order.manage"},
	}
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", fmt.Sprintf("%d", orderID))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	t.Logf("Response code=%d body=%s location=%s", rec.Code, rec.Body.String(), rec.Header().Get("Location"))

	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 200 or 303, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Verify order status in database is 'cancelled'
	var newOrderStatus string
	err = db.Pool().QueryRow(ctx, "SELECT status FROM commerce.orders WHERE id = $1", orderID).Scan(&newOrderStatus)
	if err != nil {
		t.Fatalf("failed to query updated order status: %v", err)
	}
	if newOrderStatus != "cancelled" {
		t.Errorf("expected order status 'cancelled', got '%s'", newOrderStatus)
	}

	// 5. Verify shipment status in database is 'cancelled'
	var newShipmentStatus string
	err = db.Pool().QueryRow(ctx, "SELECT status FROM commerce.order_shipments WHERE id = $1", shipmentID).Scan(&newShipmentStatus)
	if err != nil {
		t.Fatalf("failed to query updated shipment status: %v", err)
	}
	if newShipmentStatus != "cancelled" {
		t.Errorf("expected shipment status 'cancelled', got '%s'", newShipmentStatus)
	}

	// 6. Test re-cancelling the already-cancelled order should be rejected
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/orders/%d/cancel", orderID), strings.NewReader(formData.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("X-Requested-With", "XMLHttpRequest")
	req2 = req2.WithContext(authctx.WithActor(req2.Context(), actor))
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusBadRequest && rec2.Code != http.StatusSeeOther {
		t.Errorf("expected status 400 Bad Request when cancelling already cancelled order, got %d", rec2.Code)
	}
}

func TestCustomerOrderCancel_DisallowedWhenShipped(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}

	handler := newRealUIHandler(t, db)
	ctx := context.Background()

	var customerOrgID int64
	_ = db.Pool().QueryRow(ctx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"صيدلية اختبار الشحن"}', 'صيدلية اختبار الشحن', '{"ar":"صيدلية اختبار الشحن"}', 'TAX-SHIP-101', 'CR-SHIP-101', 'customer', 'approved')
		RETURNING id
	`).Scan(&customerOrgID)
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), "DELETE FROM org.organizations WHERE id = $1", customerOrgID)
	}()

	var vendorOrgID int64
	_ = db.Pool().QueryRow(ctx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"مورد اختبار الشحن"}', 'مورد اختبار الشحن', '{"ar":"مورد اختبار الشحن"}', 'TAX-VSHIP-102', 'CR-VSHIP-102', 'vendor', 'approved')
		RETURNING id
	`).Scan(&vendorOrgID)
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), "DELETE FROM org.organizations WHERE id = $1", vendorOrgID)
	}()

	var userID int64
	_ = db.Pool().QueryRow(ctx, `
		INSERT INTO identity.users (name, email, password_hash, role, status)
		VALUES ('{"ar":"د. صيدلي الشحن"}', 'order_shipped@dawa24.eg', '$2a$10$abcdefghijklmnopqrstuu', 'user', 'active')
		RETURNING id
	`).Scan(&userID)
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), "DELETE FROM identity.users WHERE id = $1", userID)
	}()

	// Create order that is in 'shipped' status
	var orderID int64
	orderNum := fmt.Sprintf("ORD-SHIP-%d", time.Now().UnixNano()%100000)
	_ = db.Pool().QueryRow(ctx, `
		INSERT INTO commerce.orders (customer_id, organization_id, order_number, status, subtotal, discount_amount, total_amount)
		VALUES ($1, $2, $3, 'shipped', 30000, 0, 30000)
		RETURNING id
	`, userID, customerOrgID, orderNum).Scan(&orderID)
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), "DELETE FROM commerce.orders WHERE id = $1", orderID)
	}()

	formData := url.Values{}
	formData.Set("reason", "طلب بالخطأ")

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/orders/%d/cancel", orderID), strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	actor := authctx.Actor{
		UserID:         userID,
		OrganizationID: customerOrgID,
		OrgType:        "customer",
		Role:           "owner",
		IsOwner:        true,
		Permissions:    []string{"pharmacy.order.view", "pharmacy.order.update", "pharmacy.order.manage"},
	}
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", fmt.Sprintf("%d", orderID))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 400 Bad Request or redirect when order is shipped, got %d", rec.Code)
	}

	// Verify order is still 'shipped'
	var status string
	_ = db.Pool().QueryRow(ctx, "SELECT status FROM commerce.orders WHERE id = $1", orderID).Scan(&status)
	if status != "shipped" {
		t.Errorf("expected order status to remain 'shipped', got '%s'", status)
	}
}
