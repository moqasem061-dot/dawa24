package erp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
)

func ordersService(repo Repository) *Service {
	return NewService(repo, Ports{}, Config{Provider: provider.DefaultProvider, Pepper: []byte("test-pepper-0123456789abcdef0123"), Tag: "test"}, slog.Default())
}

func vendorOrdersPrincipal(org int64, scopes ...string) Principal {
	if len(scopes) == 0 {
		scopes = []string{ScopeOrdersRead, ScopeOrdersAck}
	}
	return Principal{OrganizationID: org, IntegrationID: org + 1000, KeyID: org + 2000, Provider: provider.DefaultProvider, Scopes: scopes, OrgType: "vendor", KeyPrefix: "vendor01"}
}

func customerOrdersPrincipal(org int64) Principal {
	return Principal{OrganizationID: org, IntegrationID: org + 1000, KeyID: org + 2000, Provider: provider.DefaultProvider, Scopes: []string{ScopeOrdersRead, ScopeOrdersAck, ScopePurchaseCreate}, OrgType: "pharmacy", KeyPrefix: "buyer001"}
}

func wantErpCode(t *testing.T, err error, code string) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
	return e
}

func TestOrdersStateMapping(t *testing.T) {
	cases := []struct {
		in, want string
		invalid  bool
	}{
		{in: "", want: "new"},
		{in: "new", want: "new"},
		{in: "imported", want: "imported"},
		{in: "changed", want: "changed"},
		{in: "all", want: "all"},
		{in: "bogus", invalid: true},
	}
	for _, tc := range cases {
		repo := newFakeRepo()
		svc := ordersService(repo)
		_, err := svc.ListSales(context.Background(), vendorOrdersPrincipal(7), OrderFilter{State: tc.in})
		if tc.invalid {
			e := wantErpCode(t, err, "ERP_VALIDATION")
			if e.Status != 422 {
				t.Fatalf("state %q status = %d, want 422", tc.in, e.Status)
			}
			continue
		}
		if err != nil {
			t.Fatalf("state %q: %v", tc.in, err)
		}
		if repo.lastSalesFilter.State != tc.want {
			t.Fatalf("state %q reached the repository as %q, want %q", tc.in, repo.lastSalesFilter.State, tc.want)
		}
	}
}

func TestOrdersAckResults(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	created := time.Date(2026, 11, 7, 8, 0, 0, 0, time.UTC)
	repo.addSale(7, "10542", "pending", created)
	svc := ordersService(repo)
	p := vendorOrdersPrincipal(7)

	ack := func(order, ref string) AckResult {
		t.Helper()
		out, err := svc.Ack(ctx, p, "sales", []provider.AckEntry{{OrderNumber: order, ErpReference: ref}})
		if err != nil || len(out) != 1 {
			t.Fatalf("ack %s/%s = %+v, %v", order, ref, out, err)
		}
		return out[0]
	}
	if got := ack("10542", "ORGA-77123"); got.Result != "acknowledged" {
		t.Fatalf("first ack = %+v, want acknowledged", got)
	}
	if got := ack("10542", "ORGA-77123"); got.Result != "already_acknowledged" {
		t.Fatalf("same reference = %+v, want already_acknowledged", got)
	}
	got := ack("10542", "ORGA-OTHER")
	if got.Result != "conflict" || got.Code != "ERP_ACK_REFERENCE_CONFLICT" || got.ExistingReference != "ORGA-77123" {
		t.Fatalf("different reference = %+v, want conflict naming ORGA-77123", got)
	}
	if missing := ack("99999", "X-1"); missing.Result != "not_found" {
		t.Fatalf("unknown order = %+v, want not_found", missing)
	}

	big := make([]provider.AckEntry, AckMax+1)
	for i := range big {
		big[i] = provider.AckEntry{OrderNumber: "10542", ErpReference: "R"}
	}
	_, err := svc.Ack(ctx, p, "sales", big)
	if e := wantErpCode(t, err, "ERP_VALIDATION"); e.Status != 422 {
		t.Fatalf("over AckMax status = %d, want 422", e.Status)
	}
}

// TestOrdersSampleThreeAck decodes Orgasoft sample 3 through the registered
// provider. The literal {{orderNumber}} must be replaced by a seeded order; the
// unreplaced fixture must give not_found.
func TestOrdersSampleThreeAck(t *testing.T) {
	ctx := context.Background()
	raw, err := os.ReadFile("testdata/orgasoft_order_ack_sample3.json")
	if err != nil {
		t.Fatal(err)
	}
	prov, ok := provider.Get(provider.DefaultProvider)
	if !ok {
		t.Fatal("default provider is not registered")
	}

	repo := newFakeRepo()
	repo.addSale(7, "ORD-S3", "pending", time.Now())
	svc := ordersService(repo)
	p := vendorOrdersPrincipal(7)

	replaced := strings.ReplaceAll(string(raw), "{{orderNumber}}", "ORD-S3")
	entries, err := prov.DecodeAck([]byte(replaced))
	if err != nil {
		t.Fatalf("decode replaced sample: %v", err)
	}
	out, err := svc.Ack(ctx, p, "sales", entries)
	if err != nil || len(out) != 1 || out[0].Result != "acknowledged" {
		t.Fatalf("sample 3 ack = %+v, %v; want acknowledged", out, err)
	}
	if out[0].ExistingReference != "" || repo.importReference("ORD-S3") != "ORG-9921" {
		t.Fatalf("sample 3 reference = %q, want ORG-9921", repo.importReference("ORD-S3"))
	}

	entries, err = prov.DecodeAck(raw)
	if err != nil {
		t.Fatalf("decode unreplaced sample: %v", err)
	}
	if len(entries) != 1 || entries[0].OrderNumber != "{{orderNumber}}" {
		t.Fatalf("unreplaced sample decoded to %+v, want the literal placeholder", entries)
	}
	out, err = svc.Ack(ctx, p, "sales", entries)
	if err != nil || out[0].Result != "not_found" {
		t.Fatalf("unreplaced sample ack = %+v, %v; want not_found", out, err)
	}
}

func TestOrdersDateRange(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	base := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	repo.addSale(7, "R-1", "pending", base.Add(-time.Hour))
	repo.addSale(7, "R-2", "pending", base.Add(time.Hour))
	repo.addSale(7, "R-3", "pending", base.Add(48*time.Hour))
	svc := ordersService(repo)

	from, to := base, base.Add(24*time.Hour)
	page, err := svc.ListSales(ctx, vendorOrdersPrincipal(7), OrderFilter{State: "all", From: &from, To: &to})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Rows) != 1 || page.Rows[0].OrderNumber != "R-2" {
		t.Fatalf("range rows = %+v, want only R-2", page.Rows)
	}
}

func TestOrdersCapabilityAndScope(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	repo.addSale(7, "C-1", "pending", time.Now())
	repo.addPurchase(8, "PO-1", "pending", time.Now())
	svc := ordersService(repo)

	_, err := svc.ListSales(ctx, customerOrdersPrincipal(8), OrderFilter{State: "all"})
	wantErpCode(t, err, "ERP_CAPABILITY_REQUIRED")

	_, err = svc.ListSales(ctx, vendorOrdersPrincipal(7, ScopeOrdersAck), OrderFilter{State: "all"})
	e := wantErpCode(t, err, "ERP_SCOPE_MISSING")
	if e.Details["requiredScope"] != ScopeOrdersRead {
		t.Fatalf("scope details = %+v, want requiredScope %s", e.Details, ScopeOrdersRead)
	}

	if _, err := svc.ListPurchase(ctx, customerOrdersPrincipal(8), OrderFilter{State: "all"}); err != nil {
		t.Fatalf("customer purchase read: %v", err)
	}
	if _, err := svc.ListPurchase(ctx, vendorOrdersPrincipal(7), OrderFilter{State: "all"}); err != nil {
		t.Fatalf("vendor purchase read: %v", err)
	}
}

func TestOrdersPagingDefaultsAndLimits(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	for i := 0; i < 3; i++ {
		repo.addSale(7, fmt.Sprintf("P-%d", i), "pending", time.Now())
	}
	svc := ordersService(repo)

	page, err := svc.ListSales(ctx, vendorOrdersPrincipal(7), OrderFilter{State: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Limit != PageDefault || repo.lastSalesFilter.Limit != PageDefault+1 {
		t.Fatalf("default page limit = %d (repository %d), want %d", page.Limit, repo.lastSalesFilter.Limit, PageDefault)
	}

	small, err := svc.ListSales(ctx, vendorOrdersPrincipal(7), OrderFilter{State: "all", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(small.Rows) != 2 || small.NextAfter != small.Rows[1].ShipmentID {
		t.Fatalf("page of 2 = %d rows, next %d; want 2 rows and next the last shipment id", len(small.Rows), small.NextAfter)
	}

	_, err = svc.ListSales(ctx, vendorOrdersPrincipal(7), OrderFilter{State: "all", Limit: PageMax + 1})
	wantErpCode(t, err, "ERP_VALIDATION")
}

// TestOrdersMissingIsIdentical proves another organization's order gives the
// same answer as an order that does not exist (no existence oracle).
func TestOrdersMissingIsIdentical(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	repo.addSale(8, "OTHER-1", "pending", time.Now())
	svc := ordersService(repo)

	_, otherErr := svc.GetSales(ctx, vendorOrdersPrincipal(7), "OTHER-1")
	_, missingErr := svc.GetSales(ctx, vendorOrdersPrincipal(7), "NOPE-1")
	a := wantErpCode(t, otherErr, "ERP_ORDER_NOT_FOUND")
	b := wantErpCode(t, missingErr, "ERP_ORDER_NOT_FOUND")
	if a.Status != 404 || b.Status != 404 || a.Error() != b.Error() {
		t.Fatalf("other-org and missing errors differ: %v (%d) vs %v (%d)", otherErr, a.Status, missingErr, b.Status)
	}
}
