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

func TestVendorReviewAndReplyLifecycle_E2E(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}

	handler := newRealUIHandler(t, db)
	ctx := context.Background()

	// 1. Create a pharmacy customer organization & vendor organization
	var customerOrgID int64
	err := db.Pool().QueryRow(ctx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"صيدلية النور والشفاء"}', 'صيدلية النور والشفاء', '{"ar":"صيدلية النور والشفاء"}', 'TAX-REV-PHARM', 'CR-REV-PHARM', 'customer', 'approved')
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
		VALUES ('{"ar":"شركة الأندلس للأدوية"}', 'شركة الأندلس للأدوية', '{"ar":"شركة الأندلس للأدوية"}', 'TAX-REV-VEND', 'CR-REV-VEND', 'vendor', 'approved')
		RETURNING id
	`).Scan(&vendorOrgID)
	if err != nil {
		t.Fatalf("failed to insert vendor org: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM org.organizations WHERE id = $1`, vendorOrgID)
	}()

	// 2. Create customer and vendor users
	var customerUserID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO identity.users (name, email, password_hash, role, status)
		VALUES ('{"ar":"د. صيدلي الاختبار"}', 'pharmacy-rev@test.local', '$2a$10$abcdefghijklmnopqrstuu', 'user', 'active')
		RETURNING id
	`).Scan(&customerUserID)
	if err != nil {
		t.Fatalf("failed to insert customer user: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM identity.users WHERE id = $1`, customerUserID)
	}()

	var vendorUserID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO identity.users (name, email, password_hash, role, status)
		VALUES ('{"ar":"مسؤول مبيعات الأندلس"}', 'vendor-rev@test.local', '$2a$10$abcdefghijklmnopqrstuu', 'user', 'active')
		RETURNING id
	`).Scan(&vendorUserID)
	if err != nil {
		t.Fatalf("failed to insert vendor user: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM identity.users WHERE id = $1`, vendorUserID)
	}()

	// Bind members
	_, _ = db.Pool().Exec(ctx, `
		INSERT INTO org.members (organization_id, user_id, role_key)
		VALUES ($1, $2, 'pharmacy_owner'), ($3, $4, 'org_manager');
	`, customerOrgID, customerUserID, vendorOrgID, vendorUserID)

	// 3. Create a delivered master order with a delivered shipment
	var orderID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO commerce.orders (customer_id, organization_id, order_number, status, subtotal, discount_amount, tax_amount, total_amount, payment_method, payment_status)
		VALUES ($1, $2, 'ORD-REV-001', 'delivered', 10000, 0, 0, 10000, 'cod', 'paid')
		RETURNING id
	`, customerUserID, customerOrgID).Scan(&orderID)
	if err != nil {
		t.Fatalf("failed to insert order: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM commerce.orders WHERE id = $1`, orderID)
	}()

	var shipmentID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO commerce.order_shipments (order_id, organization_id, shipment_number, status, subtotal, total_amount, delivery_code)
		VALUES ($1, $2, 'SHP-REV-01', 'delivered', 10000, 10000, '772211')
		RETURNING id
	`, orderID, vendorOrgID).Scan(&shipmentID)
	if err != nil {
		t.Fatalf("failed to insert shipment: %v", err)
	}

	pharmActor := authctx.Actor{
		UserID:         customerUserID,
		OrganizationID: customerOrgID,
		Role:           "customer",
		Permissions:    []string{"pharmacy.review.write"},
	}

	// 4. Pharmacy submits 3-criteria rating review
	reviewForm := url.Values{
		"organization_id": {fmt.Sprintf("%d", vendorOrgID)},
		"order_id":        {fmt.Sprintf("%d", orderID)},
		"shipment_id":     {fmt.Sprintf("%d", shipmentID)},
		"rating_rep":      {"5"},
		"rating_quality":  {"4"},
		"rating_speed":    {"5"},
		"review_text":     {"تعامل راقي وسرعة ممتازة ومندوب محترف."},
		"redirect_url":    {fmt.Sprintf("/orders/%d", orderID)},
	}

	revReq := httptest.NewRequest(http.MethodPost, "/reviews/submit", strings.NewReader(reviewForm.Encode()))
	revReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	revReq = revReq.WithContext(authctx.WithActor(revReq.Context(), pharmActor))
	revRec := httptest.NewRecorder()

	handler.ServeHTTP(revRec, revReq)

	if revRec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303 after review submit, got %d, body: %s", revRec.Code, revRec.Body.String())
	}
	loc := revRec.Header().Get("Location")
	if !strings.Contains(loc, fmt.Sprintf("/orders/%d", orderID)) {
		t.Fatalf("expected redirect to /orders/%d, got %s", orderID, loc)
	}

	// 5. Verify database records
	var reviewID int64
	var dbRating int
	var dbReviewText string
	var dbReviewerOrgID int64
	err = db.Pool().QueryRow(ctx, `
		SELECT id, rating, review_text, reviewer_org_id
		FROM org.organization_reviews
		WHERE organization_id = $1 AND order_id = $2
	`, vendorOrgID, orderID).Scan(&reviewID, &dbRating, &dbReviewText, &dbReviewerOrgID)
	if err != nil {
		t.Fatalf("review not saved in DB: %v", err)
	}
	if dbRating != 5 {
		t.Fatalf("expected overall rating 5, got %d", dbRating)
	}
	if dbReviewerOrgID != customerOrgID {
		t.Fatalf("expected reviewer_org_id %d, got %d", customerOrgID, dbReviewerOrgID)
	}

	// Verify criteria ratings
	var repScore, qualScore, speedScore int
	_ = db.Pool().QueryRow(ctx, `SELECT score FROM org.review_ratings WHERE review_id = $1 AND criterion = 'rep'`, reviewID).Scan(&repScore)
	_ = db.Pool().QueryRow(ctx, `SELECT score FROM org.review_ratings WHERE review_id = $1 AND criterion = 'quality'`, reviewID).Scan(&qualScore)
	_ = db.Pool().QueryRow(ctx, `SELECT score FROM org.review_ratings WHERE review_id = $1 AND criterion = 'speed'`, reviewID).Scan(&speedScore)

	if repScore != 5 || qualScore != 4 || speedScore != 5 {
		t.Fatalf("expected criteria ratings rep:5, qual:4, speed:5, got rep:%d, qual:%d, speed:%d", repScore, qualScore, speedScore)
	}

	// 6. Vendor accesses reviews page (/vendor/reviews)
	vendActor := authctx.Actor{
		UserID:         vendorUserID,
		OrganizationID: vendorOrgID,
		Role:           "vendor",
		OrgType:        "vendor",
		Scope:          "vendor",
		OrgStatus:      "approved",
		Permissions:    []string{"vendor.review.view", "vendor.review.reply"},
	}

	vListReq := httptest.NewRequest(http.MethodGet, "/vendor/reviews", nil)
	vListReq = vListReq.WithContext(authctx.WithActor(vListReq.Context(), vendActor))
	vListRec := httptest.NewRecorder()

	handler.ServeHTTP(vListRec, vListReq)
	if vListRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on /vendor/reviews, got %d", vListRec.Code)
	}
	vListBody := vListRec.Body.String()
	if !strings.Contains(vListBody, "صيدلية النور والشفاء") {
		t.Fatalf("expected /vendor/reviews to show pharmacy name, got: %s", vListBody)
	}
	if !strings.Contains(vListBody, "تعامل راقي وسرعة ممتازة") {
		t.Fatalf("expected /vendor/reviews to show review comment, got: %s", vListBody)
	}

	// 7. Vendor replies to pharmacy review
	replyForm := url.Values{
		"response": {"شكراً جزيلاً دكتور، نسعد دائماً بخدمة صيدليتكم الموقرة."},
	}
	replyReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/vendor/reviews/%d/reply", reviewID), strings.NewReader(replyForm.Encode()))
	replyReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	replyReq = replyReq.WithContext(authctx.WithActor(replyReq.Context(), vendActor))
	replyRec := httptest.NewRecorder()

	handler.ServeHTTP(replyRec, replyReq)
	if replyRec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303 after reply submit, got %d", replyRec.Code)
	}

	// Verify reply in database
	var dbResponse string
	err = db.Pool().QueryRow(ctx, `SELECT response FROM org.organization_reviews WHERE id = $1`, reviewID).Scan(&dbResponse)
	if err != nil || dbResponse != "شكراً جزيلاً دكتور، نسعد دائماً بخدمة صيدليتكم الموقرة." {
		t.Fatalf("vendor reply not updated correctly in DB: %v, val: %s", err, dbResponse)
	}

	// 8. View public supplier profile (/suppliers/{id}?tab=reviews)
	pubReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/suppliers/%d?tab=reviews", vendorOrgID), nil)
	pubRec := httptest.NewRecorder()

	handler.ServeHTTP(pubRec, pubReq)
	if pubRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on /suppliers/%d?tab=reviews, got %d", vendorOrgID, pubRec.Code)
	}
	pubBody := pubRec.Body.String()
	if !strings.Contains(pubBody, "صيدلية النور والشفاء") {
		t.Fatalf("expected supplier profile to show reviewer pharmacy, got body: %s", pubBody)
	}
	if !strings.Contains(pubBody, "رد المورد الرسمي") {
		t.Fatalf("expected supplier profile to show vendor reply badge, got body: %s", pubBody)
	}
	if !strings.Contains(pubBody, "شكراً جزيلاً دكتور، نسعد دائماً بخدمة صيدليتكم الموقرة") {
		t.Fatalf("expected supplier profile to show vendor reply text, got body: %s", pubBody)
	}
}
