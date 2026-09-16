package ui_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestAdminInvoicePaymentRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	// 1. Anonymous POST /admin/finance/invoices/payments redirects to login
	reqAnon := httptest.NewRequest(http.MethodPost, "/admin/finance/invoices/payments", nil)
	recAnon := httptest.NewRecorder()
	r.ServeHTTP(recAnon, reqAnon)
	if recAnon.Code != http.StatusSeeOther {
		t.Fatalf("expected anonymous POST to redirect (303), got %d", recAnon.Code)
	}

	// 2. Staff without billing permission receives redirect
	staffNoPerm := authctx.Actor{
		UserID:      2,
		IsStaff:     true,
		Permissions: []string{"unrelated.view"},
	}
	reqNoPerm := httptest.NewRequest(http.MethodPost, "/admin/finance/invoices/payments", nil)
	reqNoPerm = reqNoPerm.WithContext(authctx.WithActor(reqNoPerm.Context(), staffNoPerm))
	recNoPerm := httptest.NewRecorder()
	r.ServeHTTP(recNoPerm, reqNoPerm)
	if recNoPerm.Code != http.StatusSeeOther {
		t.Fatalf("expected unauthorized staff to redirect, got %d", recNoPerm.Code)
	}

	// 3. Staff with billing.invoice.view submits payment without invoice -> redirects with error notice to dest
	staffPerm := authctx.Actor{
		UserID:      2,
		IsStaff:     true,
		Permissions: []string{"billing.invoice.view"},
	}
	form := url.Values{}
	form.Set("redirect_to", "/admin/finance/invoices")
	form.Set("invoice_id", "0") // invalid invoice ID

	reqWithPerm := httptest.NewRequest(http.MethodPost, "/admin/finance/invoices/payments", strings.NewReader(form.Encode()))
	reqWithPerm.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqWithPerm = reqWithPerm.WithContext(authctx.WithActor(reqWithPerm.Context(), staffPerm))
	recWithPerm := httptest.NewRecorder()
	r.ServeHTTP(recWithPerm, reqWithPerm)

	if recWithPerm.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect (303), got %d", recWithPerm.Code)
	}
	loc := recWithPerm.Header().Get("Location")
	if !strings.HasPrefix(loc, "/admin/finance/invoices") {
		t.Fatalf("expected redirect to /admin/finance/invoices, got %s", loc)
	}
	if !strings.Contains(loc, "notice=error") {
		t.Fatalf("expected notice=error for invalid invoice, got %s", loc)
	}

	// 4. Also verify alias route /admin/payments/record works for staff
	reqAlias := httptest.NewRequest(http.MethodPost, "/admin/payments/record", strings.NewReader(form.Encode()))
	reqAlias.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqAlias = reqAlias.WithContext(authctx.WithActor(reqAlias.Context(), staffPerm))
	recAlias := httptest.NewRecorder()
	r.ServeHTTP(recAlias, reqAlias)

	if recAlias.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect (303), got %d", recAlias.Code)
	}
}
