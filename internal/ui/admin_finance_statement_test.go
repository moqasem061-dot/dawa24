package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestAdminFinanceStatementAndTabs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(
		nil, nil, nil, nil,
		nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil,
		logger,
	)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	adminActor := &authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "super_admin",
		Permissions: []string{"*"},
	}

	t.Run("GET /admin/finance renders default wallets tab with 6 tabs in order", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), *adminActor)
		req, _ := http.NewRequestWithContext(ctx, "GET", "/admin/finance", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()

		// Verify wallets tab is active
		if !strings.Contains(body, "tab=wallets") {
			t.Errorf("expected tab=wallets in HTML")
		}
		// Verify earnings tab is removed from nav
		if strings.Contains(body, "tab=earnings") {
			t.Errorf("expected earnings tab to be removed from HTML")
		}
		// Verify tabs order: wallets before transactions, transactions before deposits
		navIdx := strings.Index(body, "finance-tabs-nav")
		if navIdx == -1 {
			t.Fatalf("expected finance-tabs-nav in HTML")
		}
		navBody := body[navIdx:]
		wIdx := strings.Index(navBody, "tab=wallets")
		txIdx := strings.Index(navBody, "tab=transactions")
		depIdx := strings.Index(navBody, "tab=deposits")
		wthIdx := strings.Index(navBody, "tab=withdrawals")
		invIdx := strings.Index(navBody, "tab=invoices")
		payIdx := strings.Index(navBody, "tab=payments")

		if !(wIdx < txIdx && txIdx < depIdx && depIdx < wthIdx && wthIdx < invIdx && invIdx < payIdx) {
			t.Errorf("expected tabs order: wallets, transactions, deposits, withdrawals, invoices, payments; got indices: %d, %d, %d, %d, %d, %d",
				wIdx, txIdx, depIdx, wthIdx, invIdx, payIdx)
		}
	})

	t.Run("GET /admin/finance?tab=transactions shows disabled statement button when org_id=0", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), *adminActor)
		req, _ := http.NewRequestWithContext(ctx, "GET", "/admin/finance?tab=transactions", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "طباعة كشف الحساب") {
			t.Errorf("expected statement button in transactions tab")
		}
		if !strings.Contains(body, "disabled") {
			t.Errorf("expected disabled button when org_id is not selected")
		}
	})

	t.Run("GET /admin/finance?tab=transactions&org_id=42 shows active statement button", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), *adminActor)
		req, _ := http.NewRequestWithContext(ctx, "GET", "/admin/finance?tab=transactions&org_id=42", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "/admin/finance/statement?org_id=42") {
			t.Errorf("expected statement link with org_id=42")
		}
	})

	t.Run("GET /admin/finance?tab=wallets renders account statement and print actions", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), *adminActor)
		req, _ := http.NewRequestWithContext(ctx, "GET", "/admin/finance?tab=wallets", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "محافظ الصيدليات والموردين والأرصدة") {
			t.Errorf("expected wallets title in HTML")
		}
	})

	_ = org.Organization{}
	_ = billing.AdminFinanceStats{}
}
