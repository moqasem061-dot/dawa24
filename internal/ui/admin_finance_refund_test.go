package ui_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type stubRefundRepo struct {
	billing.Repository
	refundTxCalled   bool
	refundDepCalled  bool
	lastTxID         int64
	lastDepID        int64
	lastReason       string
	lastActorID      int64
	shouldErrorOnTx  bool
	shouldErrorOnDep bool
	txWalletID       int64
	txAmount         money.Amount
	depUserID        int64
	depOrgID         int64
	depTxID          int64
}

func (s *stubRefundRepo) GetWallet(_ context.Context, id int64) (*billing.Wallet, error) {
	orgID := int64(42)
	return &billing.Wallet{
		ID:             id,
		UserID:         100,
		OrganizationID: &orgID,
		Currency:       "EGP",
		Balance:        money.FromMajor(1000),
	}, nil
}

func (s *stubRefundRepo) AdminRefundTransaction(_ context.Context, transactionID int64, reason string, actorID int64) (*billing.WalletTransaction, error) {
	s.refundTxCalled = true
	s.lastTxID = transactionID
	s.lastReason = reason
	s.lastActorID = actorID

	if s.shouldErrorOnTx {
		return nil, errors.New("transaction already refunded or invalid")
	}

	revID, refundAmt := transactionID, s.txAmount
	if refundAmt.IsZero() {
		refundAmt = money.FromMajor(250)
	}
	return &billing.WalletTransaction{
		ID: 999, WalletID: s.txWalletID, Type: billing.TxRefund, Amount: refundAmt,
		BalanceAfter: money.FromMajor(1250), ReversesTransactionID: &revID, RefundedBy: &actorID, CreatedAt: time.Now(),
	}, nil
}

func (s *stubRefundRepo) AdminRefundDeposit(_ context.Context, depositID int64, reason string, actorID int64) (*billing.WalletDeposit, *billing.WalletTransaction, error) {
	s.refundDepCalled, s.lastDepID, s.lastReason, s.lastActorID = true, depositID, reason, actorID
	if s.shouldErrorOnDep {
		return nil, nil, errors.New("deposit already refunded or not approved")
	}

	txID := s.depTxID
	if txID == 0 {
		txID = 555
	}
	refundAmt := s.txAmount
	if refundAmt.IsZero() {
		refundAmt = money.FromMajor(250)
	}

	dep := &billing.WalletDeposit{
		ID: depositID, UserID: s.depUserID, OrganizationID: &s.depOrgID, Amount: refundAmt,
		Status: billing.DepositRefunded, TransactionID: &txID,
	}
	tx := &billing.WalletTransaction{
		ID: 888, WalletID: 10, Type: billing.TxRefund, Amount: refundAmt,
		BalanceAfter: money.FromMajor(1250), ReversesTransactionID: &txID, RefundedBy: &actorID, CreatedAt: time.Now(),
	}
	return dep, tx, nil
}

func setupRefundTestRouter(repo billing.Repository) (*chi.Mux, *ui.UIHandler) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	billSvc := billing.NewService(repo, logger)
	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil,
		billSvc,
		nil, nil, nil, nil,
		logger,
	)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)
	return r, handler
}

func TestAdminFinanceRefundRoutes_Permissions(t *testing.T) {
	repo := &stubRefundRepo{}
	r, _ := setupRefundTestRouter(repo)

	tests := []struct {
		name       string
		path       string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:       "Anonymous POST /admin/finance/transactions/1/refund redirects to login",
			path:       "/admin/finance/transactions/1/refund",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name: "Non-staff user POST /admin/finance/transactions/1/refund redirected",
			path: "/admin/finance/transactions/1/refund",
			actor: &authctx.Actor{
				UserID:      2,
				IsStaff:     false,
				Permissions: []string{"billing.wallet.read"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name: "Staff user without permission POST /admin/finance/transactions/1/refund redirected",
			path: "/admin/finance/transactions/1/refund",
			actor: &authctx.Actor{
				UserID:      2,
				IsStaff:     true,
				Permissions: []string{"billing.wallet.read"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name: "User with billing.wallet.manage POST /admin/finance/transactions/1/refund allowed",
			path: "/admin/finance/transactions/1/refund",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "finance_admin",
				Permissions: []string{"billing.wallet.manage"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name:       "Anonymous POST /admin/finance/deposits/1/refund redirects to login",
			path:       "/admin/finance/deposits/1/refund",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name: "Non-staff user POST /admin/finance/deposits/1/refund redirected",
			path: "/admin/finance/deposits/1/refund",
			actor: &authctx.Actor{
				UserID:      2,
				IsStaff:     false,
				Permissions: []string{"billing.payment.view"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name: "Staff user without permission POST /admin/finance/deposits/1/refund redirected",
			path: "/admin/finance/deposits/1/refund",
			actor: &authctx.Actor{
				UserID:      2,
				IsStaff:     true,
				Permissions: []string{"billing.payment.view"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name: "User with billing.payment.update POST /admin/finance/deposits/1/refund allowed",
			path: "/admin/finance/deposits/1/refund",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "finance_admin",
				Permissions: []string{"billing.payment.update"},
			},
			wantStatus: http.StatusSeeOther,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Set("reason", "Refund test")
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tt.actor != nil {
				req = req.WithContext(authctx.WithActor(req.Context(), *tt.actor))
			}

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestAdminFinanceRefund_TransactionFunctional(t *testing.T) {
	actor := &authctx.Actor{
		UserID:      55,
		IsStaff:     true,
		Permissions: []string{"billing.wallet.manage"},
	}

	t.Run("Refunding transaction with valid reason succeeds and redirects", func(t *testing.T) {
		repo := &stubRefundRepo{
			txWalletID: 10,
			txAmount:   money.FromMajor(250),
		}
		r, _ := setupRefundTestRouter(repo)

		form := url.Values{}
		form.Set("reason", "Order cancelled by supplier")
		req := httptest.NewRequest(http.MethodPost, "/admin/finance/transactions/101/refund", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), *actor))

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 SeeOther, got %d", rr.Code)
		}
		loc := rr.Header().Get("Location")
		if !strings.Contains(loc, "tab=transactions") {
			t.Fatalf("expected redirect to transactions tab, got %s", loc)
		}
		if !repo.refundTxCalled {
			t.Fatalf("expected AdminRefundTransaction to be called")
		}
		if repo.lastTxID != 101 {
			t.Fatalf("expected txID 101, got %d", repo.lastTxID)
		}
		if repo.lastReason != "Order cancelled by supplier" {
			t.Fatalf("expected reason, got %s", repo.lastReason)
		}
		if repo.lastActorID != 55 {
			t.Fatalf("expected actorID 55, got %d", repo.lastActorID)
		}
	})

	t.Run("Refunding transaction without reason returns error notice", func(t *testing.T) {
		repo := &stubRefundRepo{}
		r, _ := setupRefundTestRouter(repo)

		form := url.Values{}
		form.Set("reason", "   ")
		req := httptest.NewRequest(http.MethodPost, "/admin/finance/transactions/101/refund", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), *actor))

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 SeeOther, got %d", rr.Code)
		}
		if repo.refundTxCalled {
			t.Fatalf("expected AdminRefundTransaction NOT to be called on empty reason")
		}
	})

	t.Run("Already refunded transaction returns error redirect", func(t *testing.T) {
		repo := &stubRefundRepo{
			shouldErrorOnTx: true,
		}
		r, _ := setupRefundTestRouter(repo)

		form := url.Values{}
		form.Set("reason", "Repeated refund attempt")
		req := httptest.NewRequest(http.MethodPost, "/admin/finance/transactions/101/refund", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), *actor))

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 SeeOther, got %d", rr.Code)
		}
		loc := rr.Header().Get("Location")
		if !strings.Contains(loc, "notice=") {
			t.Fatalf("expected notice in redirect location on error, got %s", loc)
		}
	})
}

func TestAdminFinanceRefund_DepositFunctional(t *testing.T) {
	actor := &authctx.Actor{
		UserID:      55,
		IsStaff:     true,
		Permissions: []string{"billing.payment.update"},
	}

	t.Run("Refunding approved deposit with valid reason succeeds and redirects", func(t *testing.T) {
		repo := &stubRefundRepo{
			depUserID: 77,
			depOrgID:  88,
			depTxID:   444,
			txAmount:  money.FromMajor(500),
		}
		r, _ := setupRefundTestRouter(repo)

		form := url.Values{}
		form.Set("reason", "Bank transfer chargeback reversal")
		req := httptest.NewRequest(http.MethodPost, "/admin/finance/deposits/201/refund", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), *actor))

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 SeeOther, got %d", rr.Code)
		}
		loc := rr.Header().Get("Location")
		if !strings.Contains(loc, "tab=deposits") {
			t.Fatalf("expected redirect to deposits tab, got %s", loc)
		}
		if !repo.refundDepCalled {
			t.Fatalf("expected AdminRefundDeposit to be called")
		}
		if repo.lastDepID != 201 {
			t.Fatalf("expected depID 201, got %d", repo.lastDepID)
		}
		if repo.lastReason != "Bank transfer chargeback reversal" {
			t.Fatalf("expected reason, got %s", repo.lastReason)
		}
	})

	t.Run("Refunding deposit without reason returns error notice", func(t *testing.T) {
		repo := &stubRefundRepo{}
		r, _ := setupRefundTestRouter(repo)

		form := url.Values{}
		form.Set("reason", "")
		req := httptest.NewRequest(http.MethodPost, "/admin/finance/deposits/201/refund", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), *actor))

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 SeeOther, got %d", rr.Code)
		}
		if repo.refundDepCalled {
			t.Fatalf("expected AdminRefundDeposit NOT to be called on empty reason")
		}
	})
}
