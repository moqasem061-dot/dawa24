package erp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
)

// recordingDecider is the OrderDecider the decide tests use. It keeps every
// decision it was asked for and answers with a fixed result.
type recordingDecider struct {
	got    []ShipmentDecision
	result ShipmentDecisionResult
	err    error
}

func (d *recordingDecider) Decide(_ context.Context, sd ShipmentDecision) (ShipmentDecisionResult, error) {
	d.got = append(d.got, sd)
	return d.result, d.err
}

func decideService(repo Repository, decider OrderDecider) *Service {
	return NewService(repo, Ports{Decider: decider}, Config{Provider: provider.DefaultProvider, Pepper: []byte("test-pepper-0123456789abcdef0123"), Tag: "test"}, nil)
}

func decidePrincipal(org, keyID int64, scopes ...string) Principal {
	if len(scopes) == 0 {
		scopes = []string{ScopeOrdersDecide}
	}
	return Principal{OrganizationID: org, IntegrationID: org + 1000, KeyID: keyID, Provider: provider.DefaultProvider, Scopes: scopes, OrgType: "vendor", KeyPrefix: "vendor01"}
}

// decideFixture is a fake with one pending shipment of supplier 7.
func decideFixture(t *testing.T) (*fakeRepo, SalesRow) {
	t.Helper()
	repo := newFakeRepo()
	repo.addSale(7, "ORD-100", "pending", time.Now())
	row, err := repo.GetSalesShipmentAsSystem(context.Background(), 7, "ORD-100")
	if err != nil {
		t.Fatalf("seed shipment: %v", err)
	}
	return repo, row
}

func TestDecideForwardsConfirmAndReject(t *testing.T) {
	repo, row := decideFixture(t)
	decider := &recordingDecider{result: ShipmentDecisionResult{Result: "confirmed", OrderStatus: "confirmed"}}
	svc := decideService(repo, decider)
	p := decidePrincipal(7, 42)

	res, err := svc.Decide(context.Background(), p, "ORD-100", "confirmed", "")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if res.OrderNumber != "ORD-100" || res.Result != "confirmed" || res.OrderStatus != "confirmed" {
		t.Fatalf("result = %+v", res)
	}
	if len(decider.got) != 1 {
		t.Fatalf("decider calls = %d, want 1", len(decider.got))
	}
	if sd := decider.got[0]; sd.ShipmentID != row.ShipmentID || sd.OrganizationID != 7 || sd.Status != "confirmed" || sd.Reason != "" {
		t.Fatalf("decision = %+v", sd)
	}

	decider.result = ShipmentDecisionResult{Result: "rejected", OrderStatus: "cancelled"}
	if _, err := svc.Decide(context.Background(), p, "ORD-100", "rejected", "out of stock"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if sd := decider.got[1]; sd.Status != "rejected" || sd.Reason != "out of stock" {
		t.Fatalf("reject decision = %+v", sd)
	}
}

func TestDecideRefusesDeliverySteps(t *testing.T) {
	repo, _ := decideFixture(t)
	decider := &recordingDecider{}
	svc := decideService(repo, decider)
	for _, status := range []string{"delivered", "shipped", "completed", "out_for_delivery", "in_transit"} {
		_, err := svc.Decide(context.Background(), decidePrincipal(7, 42), "ORD-100", status, "")
		wantErpCode(t, err, "ERP_DELIVERY_NOT_ALLOWED")
	}
	if len(decider.got) != 0 {
		t.Fatalf("a delivery step reached the decider %d times", len(decider.got))
	}
}

func TestDecideRefusesOtherStatuses(t *testing.T) {
	repo, _ := decideFixture(t)
	decider := &recordingDecider{}
	svc := decideService(repo, decider)
	for _, status := range []string{"pending", "cancelled", "", "CONFIRMED"} {
		err := func() error {
			_, e := svc.Decide(context.Background(), decidePrincipal(7, 42), "ORD-100", status, "")
			return e
		}()
		e := wantErpCode(t, err, "ERP_STATUS_NOT_ALLOWED")
		if e.Status != 422 {
			t.Fatalf("status %q: HTTP %d, want 422", status, e.Status)
		}
	}
	if len(decider.got) != 0 {
		t.Fatalf("a refused status reached the decider")
	}
}

func TestDecideNeedsTheDecideScope(t *testing.T) {
	repo, _ := decideFixture(t)
	decider := &recordingDecider{}
	svc := decideService(repo, decider)

	_, err := svc.Decide(context.Background(), decidePrincipal(7, 42, ScopeOrdersRead), "ORD-100", "confirmed", "")
	e := wantErpCode(t, err, "ERP_SCOPE_MISSING")
	if e.Details["requiredScope"] != ScopeOrdersDecide {
		t.Fatalf("details = %v, want the decide scope named", e.Details)
	}
	if len(decider.got) != 0 {
		t.Fatal("a key without the scope reached the decider")
	}
}

func TestDecideIsForSuppliersOnly(t *testing.T) {
	repo, _ := decideFixture(t)
	decider := &recordingDecider{}
	svc := decideService(repo, decider)
	p := decidePrincipal(7, 42)
	p.OrgType = "pharmacy"

	_, err := svc.Decide(context.Background(), p, "ORD-100", "confirmed", "")
	wantErpCode(t, err, "ERP_CAPABILITY_REQUIRED")
	if len(decider.got) != 0 {
		t.Fatal("a pharmacy key reached the decider")
	}
}

func TestDecideResolvesOnlyTheOwnShipment(t *testing.T) {
	repo, _ := decideFixture(t)
	decider := &recordingDecider{}
	svc := decideService(repo, decider)

	_, err := svc.Decide(context.Background(), decidePrincipal(8, 42), "ORD-100", "confirmed", "")
	wantErpCode(t, err, "ERP_ORDER_NOT_FOUND")
	if len(decider.got) != 0 {
		t.Fatal("another supplier's shipment reached the decider")
	}
}

func TestDecideAuditRecordCarriesTheKey(t *testing.T) {
	repo, _ := decideFixture(t)
	decider := &recordingDecider{}
	svc := decideService(repo, decider)
	p := decidePrincipal(7, 42)

	if _, err := svc.Decide(context.Background(), p, "ORD-100", "confirmed", ""); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if _, err := svc.Decide(context.Background(), p, "ORD-100", "rejected", "no"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	confirm, reject := decider.got[0].Audit, decider.got[1].Audit
	if confirm.Action != "erp.order.confirm" || reject.Action != "erp.order.reject" {
		t.Fatalf("actions = %q, %q", confirm.Action, reject.Action)
	}
	for _, a := range []AuditRecord{confirm, reject} {
		if a.ActorKind != "api_key" || a.ActorKeyID == nil || *a.ActorKeyID != 42 || a.ActorUserID != 0 {
			t.Fatalf("audit actor = %+v, want api_key key 42 and no user", a)
		}
		if a.EntityID != "ORD-100-1" {
			t.Fatalf("audit entity = %q, want the shipment number", a.EntityID)
		}
	}
}

func TestDecideWithoutADeciderIsUnavailable(t *testing.T) {
	repo, _ := decideFixture(t)
	svc := decideService(repo, nil)

	_, err := svc.Decide(context.Background(), decidePrincipal(7, 42), "ORD-100", "confirmed", "")
	e := wantErpCode(t, err, "ERP_UNAVAILABLE")
	if e.Status != 503 {
		t.Fatalf("HTTP %d, want 503", e.Status)
	}
}

func TestDecideKeepsTheDeciderError(t *testing.T) {
	repo, _ := decideFixture(t)
	shortfall := NewError("ERP_WALLET_INSUFFICIENT", 409, map[string]any{"requiredAmount": "1093.80", "availableAmount": "10.00"})
	svc := decideService(repo, &recordingDecider{err: shortfall})

	_, err := svc.Decide(context.Background(), decidePrincipal(7, 42), "ORD-100", "confirmed", "")
	e := wantErpCode(t, err, "ERP_WALLET_INSUFFICIENT")
	if e.Details["requiredAmount"] != "1093.80" {
		t.Fatalf("details = %v, want the decider's amounts", e.Details)
	}

	svc = decideService(repo, &recordingDecider{err: errors.New("database went away")})
	_, err = svc.Decide(context.Background(), decidePrincipal(7, 42), "ORD-100", "confirmed", "")
	wantErpCode(t, err, "ERP_UNAVAILABLE")
}

func TestDecideReturnsARepeatResult(t *testing.T) {
	repo, _ := decideFixture(t)
	decider := &recordingDecider{result: ShipmentDecisionResult{Result: "already_confirmed", OrderStatus: "confirmed"}}
	svc := decideService(repo, decider)

	res, err := svc.Decide(context.Background(), decidePrincipal(7, 42), "ORD-100", "confirmed", "")
	if err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if res.Result != "already_confirmed" {
		t.Fatalf("result = %q, want already_confirmed", res.Result)
	}
}
