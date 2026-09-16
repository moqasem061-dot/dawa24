package ui_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

func TestCustomerOrderEdit_QuantityAndDatabaseSync_E2E(t *testing.T) {
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
		VALUES ('{"ar":"صيدلية اختبار التعديل"}', 'صيدلية اختبار التعديل', '{"ar":"صيدلية اختبار التعديل"}', 'TAX-EDIT-101', 'CR-EDIT-101', 'customer', 'approved')
		RETURNING id
	`).Scan(&customerOrgID)
	if err != nil {
		t.Fatalf("failed to insert customer org: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM org.organizations WHERE id = $1`, customerOrgID)
	}()

	var vendorOrgID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"مورد اختبار التعديل"}', 'مورد اختبار التعديل', '{"ar":"مورد اختبار التعديل"}', 'TAX-VEDIT-102', 'CR-VEDIT-102', 'vendor', 'approved')
		RETURNING id
	`).Scan(&vendorOrgID)
	if err != nil {
		t.Fatalf("failed to insert vendor org: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM org.organizations WHERE id = $1`, vendorOrgID)
	}()

	var userID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO identity.users (name, email, password_hash, role, status)
		VALUES ('{"ar":"د. صيدلي التعديل"}', 'order_editor@dawa24.eg', '$2a$10$abcdefghijklmnopqrstuu', 'user', 'active')
		RETURNING id
	`).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM identity.users WHERE id = $1`, userID)
	}()

	// Create warehouse and product variant with stock = 50
	var warehouseID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO inventory.warehouses (organization_id, name, is_active)
		VALUES ($1, 'مخزن المورد الرئيسي', true)
		RETURNING id
	`, vendorOrgID).Scan(&warehouseID)
	if err != nil {
		t.Fatalf("failed to insert warehouse: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM inventory.warehouses WHERE id = $1`, warehouseID)
	}()

	var productID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO catalog.products (organization_id, name, price, status)
		VALUES ($1, '{"ar":"بانادول إكسترا 500 مجم", "en":"Panadol Extra 500mg"}', 50.00, 'active')
		RETURNING id
	`, vendorOrgID).Scan(&productID)
	if err != nil {
		t.Fatalf("failed to insert product: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM catalog.products WHERE id = $1`, productID)
	}()

	var variantID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO catalog.product_variants (organization_id, product_id, name, sku, price, min_order_qty)
		VALUES ($1, $2, '{"ar":"علبة 24 قرص", "en":"Box 24 Tablets"}', 'SKU-PAN-01', 50.00, 1)
		RETURNING id
	`, vendorOrgID, productID).Scan(&variantID)
	if err != nil {
		t.Fatalf("failed to insert variant: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM catalog.product_variants WHERE id = $1`, variantID)
	}()

	// Stock of 50 units for this variant
	_, err = db.Pool().Exec(ctx, `
		INSERT INTO inventory.stocks (organization_id, warehouse_id, product_id, product_variant_id, quantity)
		VALUES ($1, $2, $3, $4, 50)
	`, vendorOrgID, warehouseID, productID, variantID)
	if err != nil {
		t.Fatalf("failed to insert stock: %v", err)
	}

	// 2. Create a pending order with 2 items (Item 1: 5 units @ 50 EGP each with 5 EGP discount each; Item 2: 2 units @ 100 EGP each)
	var orderID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO commerce.orders (
			order_number, customer_id, organization_id, status,
			subtotal, discount_amount, total_discount, tax_amount, shipping_fee, total_amount, final_price,
			payment_method, payment_status
		) VALUES (
			'ORD-EDIT-TEST-01', $1, $2, 'pending',
			450.00, 25.00, 25.00, 0.00, 0.00, 425.00, 425.00,
			'cod', 'unpaid'
		)
		RETURNING id
	`, userID, customerOrgID).Scan(&orderID)
	if err != nil {
		t.Fatalf("failed to insert test order: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM commerce.orders WHERE id = $1`, orderID)
	}()

	var shipmentID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO commerce.order_shipments (
			order_id, organization_id, shipment_number, status, subtotal, total_amount
		) VALUES (
			$1, $2, 'SHP-EDIT-01', 'pending', 450.00, 425.00
		)
		RETURNING id
	`, orderID, vendorOrgID).Scan(&shipmentID)
	if err != nil {
		t.Fatalf("failed to insert shipment: %v", err)
	}

	var line1ID, line2ID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO commerce.order_lines (
			order_id, shipment_id, organization_id, product_id, product_variant_id,
			product_name, sku, unit_price, quantity, discount_amount, total_price, original_discount
		) VALUES (
			$1, $2, $3, $4, $5,
			'{"ar":"بانادول إكسترا 500 مجم"}', 'SKU-PAN-01', 50.00, 5, 25.00, 225.00, 5.00
		)
		RETURNING id
	`, orderID, shipmentID, vendorOrgID, productID, variantID).Scan(&line1ID)
	if err != nil {
		t.Fatalf("failed to insert line 1: %v", err)
	}

	err = db.Pool().QueryRow(ctx, `
		INSERT INTO commerce.order_lines (
			order_id, shipment_id, organization_id,
			product_name, sku, unit_price, quantity, discount_amount, total_price
		) VALUES (
			$1, $2, $3,
			'{"ar":"فيتامين سي 1000 مجم"}', 'SKU-VITC', 100.00, 2, 0.00, 200.00
		)
		RETURNING id
	`, orderID, shipmentID, vendorOrgID).Scan(&line2ID)
	if err != nil {
		t.Fatalf("failed to insert line 2: %v", err)
	}

	customerActor := authctx.Actor{
		UserID:         userID,
		OrganizationID: customerOrgID,
		OrgType:        "customer",
		Role:           "owner",
		IsOwner:        true,
		Permissions:    []string{"pharmacy.order.view", "pharmacy.order.update", "pharmacy.order.manage"},
	}

	// 3. Test GET /customer/orders/{id} renders the editable table with correct data attributes
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/customer/orders/%d", orderID), nil)
	getReq = getReq.WithContext(authctx.WithActor(getReq.Context(), customerActor))
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /customer/orders/%d returned %d, expected 200", orderID, getRec.Code)
	}
	body := getRec.Body.String()
	if !strings.Contains(body, "بانادول إكسترا") {
		t.Errorf("expected product 1 in body")
	}
	if !strings.Contains(body, "حفظ تعديلات الكميات") {
		t.Errorf("expected save button in body")
	}
	if !strings.Contains(body, "data-unit-price-minor") {
		t.Errorf("expected real-time unit price data attribute on table rows")
	}

	// 4. Test POST /orders/{id}/edit updating Line 1 quantity to 10 (which is <= available stock of 50) and Line 2 quantity to 3
	// Line 1: 10 * 50 = 500, discount = 10 * 5 = 50 => Line 1 total = 450
	// Line 2: 3 * 100 = 300, discount = 0 => Line 2 total = 300
	// Order Subtotal: 800.00, Total Discount: 50.00, Net Total: 750.00
	editForm := url.Values{
		"line_id[]":         {fmt.Sprintf("%d", line1ID), fmt.Sprintf("%d", line2ID)},
		"product_name[]":    {"بانادول إكسترا 500 مجم", "فيتامين سي 1000 مجم"},
		"quantity[]":        {"10", "3"},
		"unit_price[]":      {"50.00", "100.00"},
		"discount_amount[]": {"25.00", "0.00"},
		"is_deleted[]":      {"0", "0"},
		"notes":             {"تعديل الكميات بالاتفاق مع المورد"},
	}

	postReq := httptest.NewRequest("POST", fmt.Sprintf("/orders/%d/edit", orderID), strings.NewReader(editForm.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq = postReq.WithContext(authctx.WithActor(postReq.Context(), customerActor))
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusSeeOther {
		t.Fatalf("POST /orders/%d/edit returned %d, expected 303 redirect", orderID, postRec.Code)
	}

	// 5. Verify database persistence and accurate calculations
	var dbSubtotal, dbDiscount, dbTotal float64
	err = db.Pool().QueryRow(ctx, `
		SELECT subtotal::float8, discount_amount::float8, total_amount::float8
		FROM commerce.orders
		WHERE id = $1
	`, orderID).Scan(&dbSubtotal, &dbDiscount, &dbTotal)
	if err != nil {
		t.Fatalf("failed to query updated order in DB: %v", err)
	}

	if dbSubtotal != 800.00 {
		t.Errorf("expected DB Subtotal 800.00, got: %.2f", dbSubtotal)
	}
	if dbDiscount != 50.00 {
		t.Errorf("expected DB Discount 50.00, got: %.2f", dbDiscount)
	}
	if dbTotal != 750.00 {
		t.Errorf("expected DB Net Total 750.00, got: %.2f", dbTotal)
	}

	// Verify line 1 in DB has quantity = 10, discount = 50.00, total = 450.00
	var l1Qty int
	var l1Disc, l1Total float64
	err = db.Pool().QueryRow(ctx, `
		SELECT quantity, discount_amount::float8, total_price::float8
		FROM commerce.order_lines
		WHERE id = $1
	`, line1ID).Scan(&l1Qty, &l1Disc, &l1Total)
	if err != nil {
		t.Fatalf("failed to query line 1 in DB: %v", err)
	}

	if l1Qty != 10 {
		t.Errorf("expected Line 1 quantity 10, got: %d", l1Qty)
	}
	if l1Disc != 50.00 {
		t.Errorf("expected Line 1 discount 50.00, got: %.2f", l1Disc)
	}
	if l1Total != 450.00 {
		t.Errorf("expected Line 1 total 450.00, got: %.2f", l1Total)
	}

	// 6. Test Stock Validation: attempting to order 100 units (exceeds vendor stock of 50)
	excessiveForm := url.Values{
		"line_id[]":         {fmt.Sprintf("%d", line1ID)},
		"product_name[]":    {"بانادول إكسترا 500 مجم"},
		"quantity[]":        {"100"},
		"unit_price[]":      {"50.00"},
		"discount_amount[]": {"50.00"},
		"is_deleted[]":      {"0"},
	}
	exReq := httptest.NewRequest("POST", fmt.Sprintf("/orders/%d/edit", orderID), strings.NewReader(excessiveForm.Encode()))
	exReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	exReq = exReq.WithContext(authctx.WithActor(exReq.Context(), customerActor))
	exRec := httptest.NewRecorder()
	handler.ServeHTTP(exRec, exReq)

	if exRec.Code != http.StatusSeeOther {
		t.Fatalf("POST exceeding stock returned %d, expected 303 redirect", exRec.Code)
	}
	loc := exRec.Header().Get("Location")
	if !strings.Contains(loc, "error") {
		t.Errorf("expected error notice in redirect when exceeding stock, got: %s", loc)
	}
}
