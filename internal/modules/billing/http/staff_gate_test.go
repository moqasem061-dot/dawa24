package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	billingHttp "github.com/moqasem061-dot/dawa24/internal/modules/billing/http"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
)

// Being staff is not billing authority. A staff role that manages invoices but
// does not hold billing.admin must not credit wallets, settle invoices or
// activate subscriptions directly.
func TestDirectMoneyMovesRequireBillingAdminNotStaff(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			a := authctx.Actor{UserID: 2, OrganizationID: 1, Role: "support", IsStaff: true}
			a.Grants([]string{"billing.invoice.manage", "billing.wallet.manage", "billing.subscription_plan.update"})
			ctx := database.WithTenant(authctx.WithActor(req.Context(), a), 1)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	billingHttp.NewHandler(billing.NewService(happyRepo{}, log), log).RegisterRoutes(r)

	for _, tc := range []struct{ path, body string }{
		{"/api/v1/billing/wallet/deposit", `{"user_id":1,"currency":"EGP","amount":"50.00"}`},
		{"/api/v1/billing/wallet/withdraw", `{"user_id":1,"currency":"EGP","amount":"5.00"}`},
		{"/api/v1/billing/invoices/1/pay", `{"payment_method_id":1}`},
		{"/api/v1/billing/subscriptions", `{"plan_slug":"basic"}`},
	} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("POST %s by staff without billing.admin: status %d; want 403", tc.path, rec.Code)
		}
	}
}
