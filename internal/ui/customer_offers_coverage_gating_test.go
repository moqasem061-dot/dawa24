package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// TestOffersPage_CoverageAndGating verifies that out-of-coverage offers are completely
// filtered out and hidden from buyers, while guest users can browse active offers.
func TestOffersPage_CoverageAndGating(t *testing.T) {
	handler, buyerActor := setupGatingTestFixture()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/offers", nil)
	ctx := authctx.WithActor(req.Context(), buyerActor)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.OffersPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	body := rr.Body.String()

	// 1. In-coverage Cairo offer must appear
	if !strings.Contains(body, "عرض القاهرة المتوفر") {
		t.Errorf("expected Cairo offer 101 to appear on offers page")
	}

	// 2. Out-of-coverage Alexandria offer must NOT appear from the start
	if strings.Contains(body, "عرض الإسكندرية خارج التغطية") {
		t.Errorf("out-of-coverage Alexandria offer 102 must NOT appear on offers page for Cairo buyer")
	}

	// 3. Out-of-coverage badge must NOT appear anywhere on the page
	if strings.Contains(body, "خارج التغطية") {
		t.Errorf("expected NO 'خارج التغطية' badge on offers page")
	}

	// 4. Covered badge is displayed for the covered offer
	if !strings.Contains(body, "مشمول بالتغطية") {
		t.Errorf("expected 'مشمول بالتغطية' badge for covered offer")
	}

	// 5. Guest user (not logged in as a buyer) can still browse active offers
	guestReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/offers", nil)
	guestRR := httptest.NewRecorder()
	handler.OffersPage(guestRR, guestReq)
	if guestRR.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for guest, got %d", guestRR.Code)
	}
	guestBody := guestRR.Body.String()
	if !strings.Contains(guestBody, "عرض القاهرة المتوفر") {
		t.Errorf("expected Cairo offer to appear for guest")
	}
	if !strings.Contains(guestBody, "عرض الإسكندرية خارج التغطية") {
		t.Errorf("expected Alexandria offer to appear for guest browsing public catalog")
	}
}

func TestOfferDetailPage_CoverageAndCartAction(t *testing.T) {
	handler, buyerActor := setupGatingTestFixture()

	// Subtest 1: In-coverage offer opens with green banner and enabled cart button
	t.Run("covered offer displays success alert and enabled button", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/offers/101", nil)
		ctx := authctx.WithActor(req.Context(), buyerActor)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "101")
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.OfferDetailPage(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "عرض القاهرة المتوفر") {
			t.Errorf("expected offer title on detail page")
		}
		if !strings.Contains(body, "فرع صيدليتك مشمول في نطاق توصيل هذا العرض") {
			t.Errorf("expected covered banner on detail page")
		}
		if !strings.Contains(body, "إضافة العرض إلى السلة والشراء الآن") {
			t.Errorf("expected enabled add-to-cart button")
		}
	})

	// Subtest 2: Out-of-coverage offer opens with warning banner and disabled button
	t.Run("out-of-coverage offer displays warning alert and disabled button", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/offers/102", nil)
		ctx := authctx.WithActor(req.Context(), buyerActor)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "102")
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.OfferDetailPage(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "عرض الإسكندرية خارج التغطية") {
			t.Errorf("expected offer title on detail page")
		}
		if !strings.Contains(body, "فرع صيدليتك خارج نطاق التغطية الجغرافية لهذا العرض") {
			t.Errorf("expected out-of-coverage warning banner")
		}
		if !strings.Contains(body, "غير متاح لفرعك (خارج التغطية)") {
			t.Errorf("expected disabled add-to-cart button")
		}
	})
}

func TestOfferAddToCart_CoverageAndAvailabilityGating(t *testing.T) {
	handler, buyerActor := setupGatingTestFixture()

	// Subtest 1: Out-of-coverage bundle cannot be added to cart
	t.Run("out-of-coverage offer rejected at cart-add", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/cart/add-offer", strings.NewReader("offer_id=102&quantity=1"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		ctx := authctx.WithActor(req.Context(), buyerActor)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.AddOfferToCartSubmit(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}
		loc := rr.Header().Get("Location")
		if !strings.Contains(loc, "notice=error") {
			t.Errorf("expected error notice on redirect, got %s", loc)
		}
	})

	// Subtest 2: Products inside offer do not gate cart-add (bundle is ordered as promotional unit)
	t.Run("in-coverage offer succeeds at cart-add regardless of catalog stock", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/cart/add-offer", strings.NewReader("offer_id=103&quantity=1"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		ctx := authctx.WithActor(req.Context(), buyerActor)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.AddOfferToCartSubmit(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}
		loc := rr.Header().Get("Location")
		if !strings.Contains(loc, "notice=success") {
			t.Errorf("expected success notice on redirect, got %s", loc)
		}
	})
}
