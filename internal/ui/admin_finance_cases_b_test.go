package ui_test

import (
	"net/http"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

var adminFinanceRouteCasesB = []adminFinanceRouteCase{
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
