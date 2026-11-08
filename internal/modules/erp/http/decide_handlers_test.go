package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
)

// stubDecider is the OrderDecider behind the handler. It records each decision
// and answers with a fixed result.
type stubDecider struct {
	got    []erp.ShipmentDecision
	result erp.ShipmentDecisionResult
	err    error
}

func (d *stubDecider) Decide(_ context.Context, sd erp.ShipmentDecision) (erp.ShipmentDecisionResult, error) {
	d.got = append(d.got, sd)
	return d.result, d.err
}

// newDecideFixture is a supplier key with the decide scope and one pending sale.
func newDecideFixture(t *testing.T, decider erp.OrderDecider) *orderFixture {
	t.Helper()
	pepper := []byte("decide-handler-pepper-32-bytes-x")
	full, prefix, hash, err := erp.GenerateKey("test", pepper)
	if err != nil {
		t.Fatal(err)
	}
	repo := &orderHandlerRepo{keys: map[string]erp.KeyAuthRow{prefix: {
		APIKey: erp.APIKey{ID: 1, OrganizationID: 7, IntegrationID: 3, KeyPrefix: prefix, Status: "active",
			Scopes: []string{erp.ScopeOrdersDecide}, WarehouseID: 40, BranchID: 41, RateLimitPerMinute: 600, MaxBatchItems: 50000},
		KeyHash: hash, IntegrationStatus: "active", Provider: provider.DefaultProvider,
	}}}
	repo.sales = []erp.SalesRow{testSale(10, "10542", "")}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := erp.NewService(repo, erp.Ports{Decider: decider}, erp.Config{Pepper: pepper, Tag: "test", DefaultRate: 600, MaxBatchItems: 50000, Provider: provider.DefaultProvider}, logger)
	return &orderFixture{t: t, repo: repo, routes: NewHandler(svc, erp.Config{}, nil, logger).Routes(), key: full}
}

func TestDecideHandlerConfirm(t *testing.T) {
	decider := &stubDecider{result: erp.ShipmentDecisionResult{Result: "confirmed", OrderStatus: "confirmed"}}
	f := newDecideFixture(t, decider)

	w := f.do("POST", "/orders/sales/10542/status", `{"status":"confirmed"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	data := decodeEnvelope(t, w)["data"].(map[string]any)
	if data["orderNumber"] != "10542" || data["result"] != "confirmed" || data["orderStatus"] != "confirmed" {
		t.Fatalf("data = %+v", data)
	}
	if len(decider.got) != 1 || decider.got[0].Status != "confirmed" || decider.got[0].ShipmentID != 10 {
		t.Fatalf("decisions = %+v", decider.got)
	}
}

func TestDecideHandlerRejectPassesTheReason(t *testing.T) {
	decider := &stubDecider{result: erp.ShipmentDecisionResult{Result: "rejected", OrderStatus: "cancelled"}}
	f := newDecideFixture(t, decider)

	w := f.do("POST", "/orders/sales/10542/status", `{"status":"rejected","reason":"الصنف غير متوفر"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if len(decider.got) != 1 || decider.got[0].Status != "rejected" || decider.got[0].Reason != "الصنف غير متوفر" {
		t.Fatalf("decisions = %+v", decider.got)
	}
}

func TestDecideHandlerDeliveryIsForbidden(t *testing.T) {
	decider := &stubDecider{}
	f := newDecideFixture(t, decider)

	w := f.do("POST", "/orders/sales/10542/status", `{"status":"delivered"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (body %s)", w.Code, w.Body.String())
	}
	if code := decodeEnvelope(t, w)["error"].(map[string]any)["code"]; code != "ERP_DELIVERY_NOT_ALLOWED" {
		t.Fatalf("code = %v, want ERP_DELIVERY_NOT_ALLOWED", code)
	}
	if len(decider.got) != 0 {
		t.Fatal("a delivery step reached the decider")
	}
}

func TestDecideHandlerOtherStatusIsUnprocessable(t *testing.T) {
	decider := &stubDecider{}
	f := newDecideFixture(t, decider)

	w := f.do("POST", "/orders/sales/10542/status", `{"status":"pending"}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (body %s)", w.Code, w.Body.String())
	}
}

func TestDecideHandlerUnknownOrderIsNotFound(t *testing.T) {
	decider := &stubDecider{}
	f := newDecideFixture(t, decider)

	w := f.do("POST", "/orders/sales/99999/status", `{"status":"confirmed"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", w.Code, w.Body.String())
	}
	if len(decider.got) != 0 {
		t.Fatal("an unknown order reached the decider")
	}
}

func TestDecideHandlerRepeatIsOK(t *testing.T) {
	decider := &stubDecider{result: erp.ShipmentDecisionResult{Result: "already_confirmed", OrderStatus: "confirmed"}}
	f := newDecideFixture(t, decider)

	w := f.do("POST", "/orders/sales/10542/status", `{"status":"confirmed"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	if data := decodeEnvelope(t, w)["data"].(map[string]any); data["result"] != "already_confirmed" {
		t.Fatalf("result = %v, want already_confirmed", data["result"])
	}
}

func TestDecideHandlerBadJSONIsRefused(t *testing.T) {
	decider := &stubDecider{}
	f := newDecideFixture(t, decider)

	w := f.do("POST", "/orders/sales/10542/status", `{"status":`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", w.Code, w.Body.String())
	}
	if len(decider.got) != 0 {
		t.Fatal("a broken body reached the decider")
	}
}
