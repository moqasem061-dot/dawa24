package ui_test

import (
	"net/http"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

var adminFinanceRouteCasesA = []adminFinanceRouteCase{
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
}
