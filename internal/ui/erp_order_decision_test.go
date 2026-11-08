package ui

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/modules/notifications"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// The ERP confirm and reject method (S4). These tests drive the website's own
// transition through fakes: one shipment and its order, a wallet with a fixed
// balance, and an in-app notice recorder. The recorder is the only way to see the
// buyer's text, because the buyer notice is sent in the background.

const (
	erpDecisionShipmentID int64 = 501
	erpDecisionOrderID    int64 = 601
	erpDecisionSupplier   int64 = 10
	erpDecisionBuyer      int64 = 20
)

// erpDecisionCommerce serves one shipment and its order and records the last
// transition the commerce service asked it to write.
type erpDecisionCommerce struct {
	commerce.Repository
	shipment *commerce.OrderShipment
	order    *commerce.Order
	written  bool
	to       commerce.OrderStatus
	history  commerce.OrderStatusHistory
	hookSet  bool
}

func (r *erpDecisionCommerce) GetShipmentByID(_ context.Context, id int64) (*commerce.OrderShipment, error) {
	if r.shipment == nil || r.shipment.ID != id {
		return nil, errors.New("shipment not found")
	}
	return r.shipment, nil
}

func (r *erpDecisionCommerce) GetOrderByID(_ context.Context, _ int64) (*commerce.Order, error) {
	return r.order, nil
}

func (r *erpDecisionCommerce) UpdateShipmentStatusWithWallet(_ context.Context, _ int64, _, to commerce.OrderStatus, history commerce.OrderStatusHistory, _ int64, hook commerce.ShipmentTxHook) error {
	r.written, r.to, r.history, r.hookSet = true, to, history, hook != nil
	return nil
}

// erpDecisionWallet is the buyer's wallet with a fixed available balance.
type erpDecisionWallet struct {
	billing.Repository
	available money.Amount
}

func (w erpDecisionWallet) GetOrCreateWallet(_ context.Context, userID int64, _ string) (*billing.Wallet, error) {
	return &billing.Wallet{UserID: userID, AvailableBalance: w.available, Currency: "EGP"}, nil
}

// erpDecisionNotices is the notification repository behind the in-app service.
type erpDecisionNotices struct {
	notifications.Repository
	mu     sync.Mutex
	bodies map[int64][]string
}

// record stores one body for one user; the enqueue hook calls it.
func (n *erpDecisionNotices) record(userID int64, body string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.bodies == nil {
		n.bodies = map[int64][]string{}
	}
	n.bodies[userID] = append(n.bodies[userID], body)
}

// buyerBody waits for the buyer's notice, which is sent in the background.
func (n *erpDecisionNotices) buyerBody(t *testing.T) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		n.mu.Lock()
		got := n.bodies[erpDecisionBuyer]
		n.mu.Unlock()
		if len(got) > 0 {
			return got[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("buyer notice was not sent")
	return ""
}

// erpDecisionHandler builds the handler with the fakes. walletBalance is nil for
// a handler without a billing service, which the wallet path reports as a
// plain error.
func erpDecisionHandler(t *testing.T, status commerce.OrderStatus, payment string, walletBalance *money.Amount) (*UIHandler, *erpDecisionCommerce, *erpDecisionNotices) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := &erpDecisionCommerce{
		shipment: &commerce.OrderShipment{
			ID: erpDecisionShipmentID, OrderID: erpDecisionOrderID, OrganizationID: erpDecisionSupplier,
			Status: status, Subtotal: money.FromMinor(109380), ShippingFee: money.FromMinor(2500), TotalAmount: money.FromMinor(111880),
		},
		order: &commerce.Order{
			ID: erpDecisionOrderID, OrderNumber: "DEC-601", CustomerID: erpDecisionBuyer,
			PaymentMethod: payment, NegotiationStatus: "none",
		},
	}
	notices := &erpDecisionNotices{}
	notifSvc := notifications.NewService(notices, log)
	var billSvc *billing.Service
	if walletBalance != nil {
		billSvc = billing.NewService(erpDecisionWallet{available: *walletBalance}, log)
	}
	h := NewUIHandler(nil, nil, nil, commerce.NewService(repo, log), nil, nil, notifSvc, nil, nil, billSvc, nil, nil, nil, nil, log)
	h.SetNotificationEnqueue(func(_ context.Context, args queue.NotificationDeliverArgs) error {
		notices.record(args.UserID, args.Body)
		return nil
	})
	return h, repo, notices
}

// erpDecisionFor is the decision the ERP sends for the fixture shipment.
func erpDecisionFor(status, reason string) erp.ShipmentDecision {
	return erp.ShipmentDecision{ShipmentID: erpDecisionShipmentID, OrganizationID: erpDecisionSupplier, Status: status, Reason: reason}
}

func TestErpDecideConfirmPendingCash(t *testing.T) {
	h, repo, _ := erpDecisionHandler(t, commerce.StatusPending, "cash", nil)

	res, err := h.ErpDecideShipment(context.Background(), erpDecisionFor("confirmed", ""))
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if res.Result != "confirmed" || res.OrderStatus != string(commerce.StatusConfirmed) {
		t.Fatalf("result = %+v, want confirmed / confirmed", res)
	}
	if !repo.written || repo.to != commerce.StatusConfirmed {
		t.Fatalf("transition written=%v to=%q, want confirmed", repo.written, repo.to)
	}
	if repo.history.ChangedByUserID != nil {
		t.Fatalf("changed_by = %v, want nil for the ERP", *repo.history.ChangedByUserID)
	}
	if !repo.hookSet {
		t.Fatal("the audit hook was not passed into the transition")
	}
}

func TestErpDecideRejectMapsToCancelledWithPrefixedNote(t *testing.T) {
	h, repo, notices := erpDecisionHandler(t, commerce.StatusPending, "cash", nil)

	res, err := h.ErpDecideShipment(context.Background(), erpDecisionFor("rejected", "الصنف غير متوفر حالياً"))
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if res.Result != "rejected" || res.OrderStatus != string(commerce.StatusCancelled) {
		t.Fatalf("result = %+v, want rejected / cancelled", res)
	}
	if repo.to != commerce.StatusCancelled {
		t.Fatalf("transition to %q, want cancelled", repo.to)
	}
	if want := "ERP: الصنف غير متوفر حالياً"; repo.history.Notes != want {
		t.Fatalf("history note = %q, want %q", repo.history.Notes, want)
	}
	body := notices.buyerBody(t)
	if !strings.Contains(body, "الصنف غير متوفر حالياً") {
		t.Fatalf("buyer notice %q does not carry the reason", body)
	}
	if strings.Contains(body, "ERP:") {
		t.Fatalf("buyer notice %q carries the ERP prefix", body)
	}
}

func TestErpDecideConfirmWithoutReasonSendsDefaultBuyerText(t *testing.T) {
	h, _, notices := erpDecisionHandler(t, commerce.StatusPending, "cash", nil)

	if _, err := h.ErpDecideShipment(context.Background(), erpDecisionFor("confirmed", "")); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	body := notices.buyerBody(t)
	if strings.Contains(body, "ملاحظات") || strings.Contains(body, "ERP") {
		t.Fatalf("buyer notice %q carries a reason it was not given", body)
	}
}

func TestErpDecideRepeatReturnsAlreadyResult(t *testing.T) {
	h, repo, _ := erpDecisionHandler(t, commerce.StatusConfirmed, "cash", nil)

	res, err := h.ErpDecideShipment(context.Background(), erpDecisionFor("confirmed", ""))
	if err != nil {
		t.Fatalf("repeat confirm: %v", err)
	}
	if res.Result != "already_confirmed" {
		t.Fatalf("result = %q, want already_confirmed", res.Result)
	}
	if repo.written {
		t.Fatal("a repeat confirm wrote a transition")
	}
}

func TestErpDecideRepeatRejectReturnsAlreadyRejected(t *testing.T) {
	h, repo, _ := erpDecisionHandler(t, commerce.StatusCancelled, "cash", nil)

	res, err := h.ErpDecideShipment(context.Background(), erpDecisionFor("rejected", ""))
	if err != nil {
		t.Fatalf("repeat reject: %v", err)
	}
	if res.Result != "already_rejected" || repo.written {
		t.Fatalf("result = %+v written=%v, want already_rejected and no write", res, repo.written)
	}
}

func TestErpDecideNonPendingIsStateConflict(t *testing.T) {
	h, repo, _ := erpDecisionHandler(t, commerce.StatusShipped, "cash", nil)

	_, err := h.ErpDecideShipment(context.Background(), erpDecisionFor("confirmed", ""))
	var apiErr *erp.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "ERP_ORDER_STATE_CONFLICT" {
		t.Fatalf("error = %v, want ERP_ORDER_STATE_CONFLICT", err)
	}
	if apiErr.Details["currentStatus"] != "shipped" {
		t.Fatalf("details = %v, want currentStatus shipped", apiErr.Details)
	}
	if repo.written {
		t.Fatal("a refused decision wrote a transition")
	}
}

func TestErpDecideNegotiationPendingIsRefused(t *testing.T) {
	h, repo, _ := erpDecisionHandler(t, commerce.StatusPending, "cash", nil)
	repo.order.IsNegotiation = true
	repo.order.NegotiationStatus = "pending"

	_, err := h.ErpDecideShipment(context.Background(), erpDecisionFor("confirmed", ""))
	var apiErr *erp.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "ERP_ORDER_NEGOTIATION_PENDING" {
		t.Fatalf("error = %v, want ERP_ORDER_NEGOTIATION_PENDING", err)
	}
	if repo.written {
		t.Fatal("a negotiation refusal wrote a transition")
	}
}

func TestErpDecideWalletShortfallIsInsufficient(t *testing.T) {
	balance := money.FromMinor(1000)
	h, repo, _ := erpDecisionHandler(t, commerce.StatusPending, "wallet", &balance)

	_, err := h.ErpDecideShipment(context.Background(), erpDecisionFor("confirmed", ""))
	var apiErr *erp.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "ERP_WALLET_INSUFFICIENT" {
		t.Fatalf("error = %v, want ERP_WALLET_INSUFFICIENT", err)
	}
	if apiErr.Details["requiredAmount"] != "1093.80" || apiErr.Details["availableAmount"] != "10.00" {
		t.Fatalf("details = %v, want requiredAmount 1093.80 and availableAmount 10.00", apiErr.Details)
	}
	if repo.written {
		t.Fatal("a shortfall wrote a transition")
	}
}

func TestErpDecideOtherWalletErrorIsUnavailable(t *testing.T) {
	// No billing service: the wallet pre-check fails with a plain error.
	h, repo, _ := erpDecisionHandler(t, commerce.StatusPending, "wallet", nil)

	_, err := h.ErpDecideShipment(context.Background(), erpDecisionFor("confirmed", ""))
	var apiErr *erp.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "ERP_UNAVAILABLE" {
		t.Fatalf("error = %v, want ERP_UNAVAILABLE", err)
	}
	if repo.written {
		t.Fatal("a wallet error wrote a transition")
	}
}

func TestErpDecideForAnotherOrganizationIsNotFound(t *testing.T) {
	h, repo, _ := erpDecisionHandler(t, commerce.StatusPending, "cash", nil)

	_, err := h.ErpDecideShipment(context.Background(), erp.ShipmentDecision{ShipmentID: erpDecisionShipmentID, OrganizationID: 999, Status: "confirmed"})
	var apiErr *erp.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "ERP_ORDER_NOT_FOUND" {
		t.Fatalf("error = %v, want ERP_ORDER_NOT_FOUND", err)
	}
	if repo.written {
		t.Fatal("another organization's shipment was changed")
	}
}
