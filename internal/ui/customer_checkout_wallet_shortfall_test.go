package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// walletShortfallRepo answers every wallet lookup with one fixed balance. The
// rest of billing.Repository is never reached by prepareWalletCharge.
type walletShortfallRepo struct {
	billing.Repository
	available money.Amount
}

func (r *walletShortfallRepo) GetOrCreateWallet(_ context.Context, userID int64, _ string) (*billing.Wallet, error) {
	return &billing.Wallet{ID: 1, UserID: userID, Currency: "EGP", AvailableBalance: r.available}, nil
}

func walletShortfallHandler(available money.Amount) *UIHandler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &UIHandler{log: log, billSvc: billing.NewService(&walletShortfallRepo{available: available}, log)}
}

func TestWalletShortfallError(t *testing.T) {
	ctx := context.Background()
	order := &commerce.Order{ID: 1, CustomerID: 7, PaymentMethod: "wallet"}
	goods := money.FromMinor(109380)   // 1093.80 of goods
	available := money.FromMinor(1000) // 10.00 in the wallet

	_, err := walletShortfallHandler(available).prepareWalletCharge(ctx, order, goods)
	var short walletShortfallError
	if !errors.As(err, &short) {
		t.Fatalf("short wallet error = %v, want walletShortfallError", err)
	}
	if short.Available.Minor() != 1000 || short.Required.Minor() != 109380 {
		t.Fatalf("amounts = %d / %d minor, want 1000 / 109380", short.Available.Minor(), short.Required.Minor())
	}

	// The message is the one the buyer has always seen; only the type is new.
	a, g := available.String(), goods.String()
	want := fmt.Sprintf("رصيد محفظة الصيدلية المتاح (%s ج.م) غير كافٍ لسداد قيمة الأصناف المطلوبة (%s ج.م). The pharmacy wallet balance (%s EGP) does not cover the goods amount (%s EGP)", a, g, a, g)
	if err.Error() != want {
		t.Fatalf("message = %q, want %q", err.Error(), want)
	}
}

func TestWalletShortfallErrorNotRaisedWithEnoughFunds(t *testing.T) {
	order := &commerce.Order{ID: 1, CustomerID: 7, PaymentMethod: "wallet"}
	uid, err := walletShortfallHandler(money.FromMinor(200000)).prepareWalletCharge(context.Background(), order, money.FromMinor(109380))
	if err != nil {
		t.Fatalf("enough funds: error = %v, want nil", err)
	}
	if uid != 7 {
		t.Fatalf("wallet user = %d, want 7", uid)
	}
}

func TestWalletShortfallErrorMissingBillingIsPlainError(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &UIHandler{log: log}
	order := &commerce.Order{ID: 1, CustomerID: 7, PaymentMethod: "wallet"}
	_, err := h.prepareWalletCharge(context.Background(), order, money.FromMinor(109380))
	if err == nil {
		t.Fatal("missing billing service: error = nil, want an error")
	}
	var short walletShortfallError
	if errors.As(err, &short) {
		t.Fatalf("missing billing service returned a shortfall error: %v", err)
	}
}
