package erp

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// purchaseFixture is a buyer key (organization 7, user 5) with one linked
// supplier (20) selling variant VAR-1 at 62.50 less 12.50 percent (54.69).
func purchaseFixture(t *testing.T) (*Service, *fakeRepo, *fakePlacer) {
	t.Helper()
	repo := newFakeRepo()
	placer := &fakePlacer{repo: repo}
	cfg := Config{Pepper: []byte("purchase-test-pepper-32-bytes-ok"), Tag: "test", DefaultRate: 600, MaxBatchItems: 50000, Provider: "orgasoft"}
	svc := NewService(repo, Ports{Placer: placer}, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	repo.addVariant("VAR-1", 11, 20, money.MustParse("62.50"), money.MustParse("12.50"))
	repo.poLinks[[2]int64{7, 20}] = true
	return svc, repo, placer
}

func buyerPrincipal() Principal {
	user := int64(5)
	return Principal{OrganizationID: 7, IntegrationID: 3, KeyID: 9, Provider: "orgasoft", Scopes: []string{ScopePurchaseCreate},
		BranchID: 41, BuyingUserID: &user, OrgType: "pharmacy"}
}

func purchaseLine(variant string, qty int, unit string) PurchaseLine {
	line := PurchaseLine{VariantID: variant, Quantity: qty}
	if unit != "" {
		price := money.MustParse(unit)
		line.UnitPrice = &price
	}
	return line
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	e, ok := err.(*Error)
	if !ok || e.Code != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
}

func TestPurchaseNeedsScopeReferenceAndBuyingUser(t *testing.T) {
	svc, _, _ := purchaseFixture(t)
	p := buyerPrincipal()
	p.Scopes = nil
	_, err := svc.CreatePurchaseOrder(context.Background(), p, CreatePurchaseRequest{ExternalReference: "R", Lines: []PurchaseLine{purchaseLine("VAR-1", 1, "")}})
	wantCode(t, err, "ERP_SCOPE_MISSING")

	_, err = svc.CreatePurchaseOrder(context.Background(), buyerPrincipal(), CreatePurchaseRequest{Lines: []PurchaseLine{purchaseLine("VAR-1", 1, "")}})
	wantCode(t, err, "ERP_PO_REFERENCE_REQUIRED")

	p = buyerPrincipal()
	p.BuyingUserID = nil
	_, err = svc.CreatePurchaseOrder(context.Background(), p, CreatePurchaseRequest{ExternalReference: "R", Lines: []PurchaseLine{purchaseLine("VAR-1", 1, "")}})
	wantCode(t, err, "ERP_PO_BUYING_USER_MISSING")
}

func TestPurchaseLineRules(t *testing.T) {
	svc, _, _ := purchaseFixture(t)
	for _, qty := range []int{0, -3} {
		_, err := svc.CreatePurchaseOrder(context.Background(), buyerPrincipal(), CreatePurchaseRequest{ExternalReference: "Q", Lines: []PurchaseLine{purchaseLine("VAR-1", qty, "")}})
		wantCode(t, err, "ERP_PO_QUANTITY_INVALID")
	}
	tooMany := make([]PurchaseLine, PurchaseLinesMax+1)
	for i := range tooMany {
		tooMany[i] = purchaseLine("VAR-1", 1, "")
	}
	_, err := svc.CreatePurchaseOrder(context.Background(), buyerPrincipal(), CreatePurchaseRequest{ExternalReference: "L", Lines: tooMany})
	wantCode(t, err, "ERP_VALIDATION")
}

func TestPurchaseOfferRules(t *testing.T) {
	svc, repo, _ := purchaseFixture(t)
	repo.addVariant("OWN", 12, 7, money.MustParse("10.00"), money.Zero)
	repo.addVariant("OFF", 13, 20, money.MustParse("10.00"), money.Zero)
	off := repo.poVariants["OFF"]
	off.Active = false
	repo.poVariants["OFF"] = off
	repo.addVariant("UNLINKED", 14, 30, money.MustParse("10.00"), money.Zero)
	cases := map[string]struct {
		variant, code string
	}{
		"missing":  {"NOPE", "ERP_PO_ITEM_NOT_FOUND"},
		"own":      {"OWN", "ERP_PO_SELF_SUPPLY"},
		"inactive": {"OFF", "ERP_PO_NO_ACTIVE_OFFER"},
		"unlinked": {"UNLINKED", "ERP_PO_SUPPLIER_NOT_LINKED"},
	}
	for name, c := range cases {
		_, err := svc.CreatePurchaseOrder(context.Background(), buyerPrincipal(), CreatePurchaseRequest{ExternalReference: "O-" + name, Lines: []PurchaseLine{purchaseLine(c.variant, 1, "")}})
		wantCode(t, err, c.code)
	}
}

func TestPurchasePriceMismatchNamesTheCorrectPrice(t *testing.T) {
	svc, _, placer := purchaseFixture(t)
	_, err := svc.CreatePurchaseOrder(context.Background(), buyerPrincipal(), CreatePurchaseRequest{ExternalReference: "P", Lines: []PurchaseLine{purchaseLine("VAR-1", 2, "60.00")}})
	wantCode(t, err, "ERP_PO_PRICE_MISMATCH")
	lines := err.(*Error).Details["lines"].([]map[string]any)
	if len(lines) != 1 || lines[0]["line"] != 1 || lines[0]["sentPrice"] != "60.00" || lines[0]["correctPrice"] != "54.69" {
		t.Fatalf("lines = %+v, want line 1 sent 60.00 correct 54.69", lines)
	}
	if placer.calls != 0 {
		t.Fatalf("placer called %d times after a price mismatch", placer.calls)
	}
}

func TestPurchaseReplayReturnsTheStoredOrder(t *testing.T) {
	svc, _, placer := purchaseFixture(t)
	req := CreatePurchaseRequest{ExternalReference: "REPLAY", Notes: "weekly", Lines: []PurchaseLine{purchaseLine("VAR-1", 12, "54.69")}}
	first, err := svc.CreatePurchaseOrder(context.Background(), buyerPrincipal(), req)
	if err != nil || first.Replay {
		t.Fatalf("first: replay=%v err=%v", first.Replay, err)
	}
	if placer.last.BranchID != 41 || placer.last.BuyerOrgType != "pharmacy" || placer.last.BuyingUserID != 5 {
		t.Fatalf("placer request = %+v", placer.last)
	}
	if want := "[ERP:orgasoft:REPLAY]\nweekly"; placer.last.Notes != want {
		t.Fatalf("notes = %q, want %q", placer.last.Notes, want)
	}
	second, err := svc.CreatePurchaseOrder(context.Background(), buyerPrincipal(), req)
	if err != nil || !second.Replay || second.OrderNumber != first.OrderNumber {
		t.Fatalf("second: %+v err=%v", second, err)
	}
	if placer.calls != 1 {
		t.Fatalf("placer calls = %d, want 1", placer.calls)
	}
	if second.Row.ExternalReference != "REPLAY" {
		t.Fatalf("external reference = %q", second.Row.ExternalReference)
	}
}

func TestPurchaseRefusalDeletesTheRowSoTheRetryPlaces(t *testing.T) {
	svc, repo, placer := purchaseFixture(t)
	req := CreatePurchaseRequest{ExternalReference: "REFUSE", Lines: []PurchaseLine{purchaseLine("VAR-1", 1, "")}}
	placer.err = NewError("ERP_PO_REFUSED", 409, map[string]any{"reason": "quota.exhausted"})
	_, err := svc.CreatePurchaseOrder(context.Background(), buyerPrincipal(), req)
	wantCode(t, err, "ERP_PO_REFUSED")
	if _, stored := repo.poIdem["REFUSE"]; stored {
		t.Fatal("a refusal must delete the idempotency row")
	}
	placer.err = nil
	if _, err := svc.CreatePurchaseOrder(context.Background(), buyerPrincipal(), req); err != nil {
		t.Fatalf("retry after refusal: %v", err)
	}
}

func TestPurchaseSameReferenceWithOtherBodyConflicts(t *testing.T) {
	svc, _, _ := purchaseFixture(t)
	if _, err := svc.CreatePurchaseOrder(context.Background(), buyerPrincipal(), CreatePurchaseRequest{ExternalReference: "SAME", Lines: []PurchaseLine{purchaseLine("VAR-1", 1, "")}}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreatePurchaseOrder(context.Background(), buyerPrincipal(), CreatePurchaseRequest{ExternalReference: "SAME", Lines: []PurchaseLine{purchaseLine("VAR-1", 2, "")}})
	wantCode(t, err, "ERP_IDEMPOTENCY_CONFLICT")
}

func TestPurchaseFreshInProgressIsRefused(t *testing.T) {
	svc, repo, _ := purchaseFixture(t)
	req := CreatePurchaseRequest{ExternalReference: "BUSY", Lines: []PurchaseLine{purchaseLine("VAR-1", 1, "")}}
	repo.poIdem["BUSY"] = &fakeIdem{sha: purchaseRequestDigest(req), state: "in_progress", started: time.Now()}
	_, err := svc.CreatePurchaseOrder(context.Background(), buyerPrincipal(), req)
	wantCode(t, err, "ERP_PO_IN_PROGRESS")
}

// TestPurchaseStaleRowIsRepairedThroughTheMarker simulates a crash after the
// order was written: the stale row must return that order and place nothing.
func TestPurchaseStaleRowIsRepairedThroughTheMarker(t *testing.T) {
	svc, repo, placer := purchaseFixture(t)
	req := CreatePurchaseRequest{ExternalReference: "CRASH", Lines: []PurchaseLine{purchaseLine("VAR-1", 3, "")}}
	repo.poIdem["CRASH"] = &fakeIdem{sha: purchaseRequestDigest(req), state: "in_progress", started: time.Now().Add(-5 * time.Minute)}
	repo.addPurchase(7, "90077", "pending", time.Now())
	repo.poMarkers["[ERP:orgasoft:CRASH]"] = fakeMarker{orderID: 77, orderNumber: "90077"}
	got, err := svc.CreatePurchaseOrder(context.Background(), buyerPrincipal(), req)
	if err != nil || !got.Replay || got.OrderNumber != "90077" || got.OrderID != 77 {
		t.Fatalf("repair: %+v err=%v", got, err)
	}
	if placer.calls != 0 {
		t.Fatalf("placer called %d times during repair", placer.calls)
	}
}

func TestPurchaseWritesTheAuditRowAsTheKey(t *testing.T) {
	svc, repo, _ := purchaseFixture(t)
	if _, err := svc.CreatePurchaseOrder(context.Background(), buyerPrincipal(), CreatePurchaseRequest{ExternalReference: "AUD", Lines: []PurchaseLine{purchaseLine("VAR-1", 1, "")}}); err != nil {
		t.Fatal(err)
	}
	if len(repo.poAudits) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(repo.poAudits))
	}
	a := repo.poAudits[0]
	if a.Action != "erp.po.create" || a.ActorKind != "api_key" || a.ActorKeyID == nil || *a.ActorKeyID != 9 {
		t.Fatalf("audit = %+v", a)
	}
}

func TestOffersAreCheapestFirstAndAtMostFifty(t *testing.T) {
	svc, repo, _ := purchaseFixture(t)
	for i := 0; i < 60; i++ {
		repo.poOffers = append(repo.poOffers, Offer{VariantID: "V", PublicPrice: money.FromMajor(int64(100 - i)), DiscountPercent: money.Zero, InStock: true})
	}
	offers, err := svc.Offers(context.Background(), buyerPrincipal(), "6221024000123")
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != OffersMax || repo.poOfferLimit != OffersMax {
		t.Fatalf("offers = %d, limit asked = %d, want %d", len(offers), repo.poOfferLimit, OffersMax)
	}
	for i := 1; i < len(offers); i++ {
		if offers[i-1].UnitPrice.Minor() > offers[i].UnitPrice.Minor() {
			t.Fatalf("offers not cheapest first at %d", i)
		}
	}
}
