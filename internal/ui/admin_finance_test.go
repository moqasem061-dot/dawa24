package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestAdminFinanceRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	tests := []struct {
		name       string
		path       string
		method     string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:       "Anonymous GET /admin/finance redirects to login",
			path:       "/admin/finance",
			method:     "GET",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin GET /admin/finance returns 200",
			path:   "/admin/finance",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/orders returns 200",
			path:   "/admin/orders",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/plans returns 200",
			path:   "/admin/plans",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/orders/offers returns 301",
			path:   "/admin/orders/offers",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/earnings/order returns 301",
			path:   "/admin/earnings/order",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/earnings/offers returns 301",
			path:   "/admin/earnings/offers",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/invoices returns 301",
			path:   "/admin/invoices",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/payments returns 301",
			path:   "/admin/payments",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/wallets returns 301",
			path:   "/admin/wallets",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/plans-info returns 301",
			path:   "/admin/plans-info",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/plan-types returns 301",
			path:   "/admin/plan-types",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/plan-features returns 301",
			path:   "/admin/plan-features",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/plans/subscriptions returns 301",
			path:   "/admin/plans/subscriptions",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin POST /admin/plans returns redirect",
			path:   "/admin/plans",
			method: "POST",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin POST /admin/plans/1/update returns redirect",
			path:   "/admin/plans/1/update",
			method: "POST",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin POST /admin/plans/1/toggle returns redirect",
			path:   "/admin/plans/1/toggle",
			method: "POST",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin POST /admin/plans/1/set-default returns redirect",
			path:   "/admin/plans/1/set-default",
			method: "POST",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin POST /admin/plans/1/delete returns redirect",
			path:   "/admin/plans/1/delete",
			method: "POST",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin GET /admin/finance?tab=withdrawals returns 200",
			path:   "/admin/finance?tab=withdrawals",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin POST /admin/finance/withdrawals/1/approve returns redirect",
			path:   "/admin/finance/withdrawals/1/approve",
			method: "POST",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin POST /admin/finance/withdrawals/1/reject returns redirect",
			path:   "/admin/finance/withdrawals/1/reject",
			method: "POST",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin GET /admin/finance?tab=earnings returns 301 redirect to wallets",
			path:   "/admin/finance?tab=earnings",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/finance?tab=wallets returns 200",
			path:   "/admin/finance?tab=wallets",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/finance?tab=transactions returns 200",
			path:   "/admin/finance?tab=transactions",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/finance?tab=deposits returns 200",
			path:   "/admin/finance?tab=deposits",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/finance?tab=invoices returns 200",
			path:   "/admin/finance?tab=invoices",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/finance?tab=payments returns 200",
			path:   "/admin/finance?tab=payments",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/finance/statement without org_id returns redirect",
			path:   "/admin/finance/statement",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin GET /admin/finance/wallets returns 200",
			path:   "/admin/finance/wallets",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/finance/transactions returns 200",
			path:   "/admin/finance/transactions",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/finance/deposits returns 200",
			path:   "/admin/finance/deposits",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/finance/withdrawals returns 200",
			path:   "/admin/finance/withdrawals",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/finance/invoices returns 200",
			path:   "/admin/finance/invoices",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/finance/payments returns 200",
			path:   "/admin/finance/payments",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "Anonymous GET /admin/finance/wallets returns redirect",
			path:       "/admin/finance/wallets",
			method:     "GET",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Staff with billing.wallet.read GET /admin/finance/wallets returns 200",
			path:   "/admin/finance/wallets",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      2,
				IsStaff:     true,
				Permissions: []string{"billing.wallet.read"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Staff without billing.wallet.read GET /admin/finance/wallets returns redirect",
			path:   "/admin/finance/wallets",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      2,
				IsStaff:     true,
				Permissions: []string{"promo.offer.view"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Staff with billing.payment.view GET /admin/finance/deposits returns 200",
			path:   "/admin/finance/deposits",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      2,
				IsStaff:     true,
				Permissions: []string{"billing.payment.view"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Staff with billing.invoice.view GET /admin/finance/invoices returns 200",
			path:   "/admin/finance/invoices",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      2,
				IsStaff:     true,
				Permissions: []string{"billing.invoice.view"},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.actor != nil {
				ctx = authctx.WithActor(ctx, *tt.actor)
			}

			req, _ := http.NewRequestWithContext(ctx, tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("path %s expected status %d, got %d", tt.path, tt.wantStatus, rr.Code)
			}
		})
	}
}
